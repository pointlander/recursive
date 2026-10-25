// Copyright 2026 The Recursive Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "testing"

func TestExecute(t *testing.T) {
	cases := []struct {
		code string
		out  string
		ptr  int
		cell int
		val  byte
	}{
		{"+++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++.", "A", 0, 0, 'A'},
		{"++++[->++++++++<]>.", " ", 1, 1, 32},
		{"++[>++[>++<-]<-]>>.", string([]byte{8}), 2, 2, 8},
		{"<+.", string([]byte{1}), -1, -1, 1},
		{"+x+.", string([]byte{2}), 0, 0, 2},
	}
	for _, tc := range cases {
		st, err := Execute(tc.code, 100000)
		if err != nil {
			t.Fatalf("%s: %v", tc.code, err)
		}
		if st.Out != tc.out || st.Ptr != tc.ptr || st.At(tc.cell) != tc.val {
			t.Fatalf("%s: out %q ptr %d cell %d", tc.code, st.Out, st.Ptr, st.At(tc.cell))
		}
	}
	if _, err := Execute("[", 10); err == nil {
		t.Fatal("expected unmatched bracket")
	}
	if _, err := Execute("+[.+]", 50); err == nil {
		t.Fatal("expected step limit")
	}
	st, err := Execute(stringsRepeat256Plus(), 10000)
	if err != nil {
		t.Fatal(err)
	}
	if st.At(0) != 0 || st.Out != "\x00" {
		t.Fatalf("wrap got %d %q", st.At(0), st.Out)
	}
}

func stringsRepeat256Plus() string {
	b := make([]byte, 257)
	for i := 0; i < 256; i++ {
		b[i] = '+'
	}
	b[256] = '.'
	return string(b)
}

func TestPatterns(t *testing.T) {
	ensurePats()
	if got := pats[1][0][1].code(1); got != "+" {
		t.Fatalf("0->1 %q", got)
	}
	if got := pats[1][0][255].code(1); got != "-" {
		t.Fatalf("0->255 %q", got)
	}
	if got := pats[1][4][0].code(1); got != "[-]" {
		t.Fatalf("4->0 %q", got)
	}
	if got := pats[1][0][0].code(1); got != "" {
		t.Fatalf("empty %q", got)
	}
	samples := []byte{0, 1, 2, 4, 10, 32, 97, 100, 110, 119, 128, 200, 255}
	for dist := 1; dist <= 3; dist++ {
		for _, from := range samples {
			for _, to := range samples {
				p := pats[dist][from][to]
				code := p.code(dist)
				if len(code) != int(p.length) || leadingLT(code) != int(p.lead) {
					t.Fatalf("measure drift")
				}
				if addedCost(0, dist, len(code), leadingLT(code)) != len(join(0, dist, code)) {
					t.Fatalf("join drift dist %d %q", dist, code)
				}
				setup := make([]byte, 0, dist+int(from))
				for i := 0; i < dist; i++ {
					setup = append(setup, '>')
				}
				for i := 0; i < int(from); i++ {
					setup = append(setup, '+')
				}
				st, err := Execute(string(setup)+code, 100000)
				if err != nil {
					t.Fatalf("run %v", err)
				}
				if st.Ptr != dist || st.At(dist) != to || st.At(0) != 0 {
					t.Fatalf("dist %d %d->%d code %q ptr %d cell %d scratch %d", dist, from, to, code, st.Ptr, st.At(dist), st.At(0))
				}
				for c := 1; c < dist; c++ {
					if st.At(c) != 0 {
						t.Fatalf("clobber cell %d with %q", c, code)
					}
				}
			}
		}
	}
}

func TestInit(t *testing.T) {
	targets := []byte{32, 100, 110, 119}
	for scale := 1; scale <= 40; scale++ {
		n, ptr := snappedCost(scale, targets)
		code, p := snappedCode(scale, targets)
		if n != len(code) || ptr != p {
			t.Fatalf("scale %d cost %d/%d ptr %d/%d", scale, n, len(code), ptr, p)
		}
		st, err := Execute(code, 1_000_000)
		if err != nil {
			t.Fatal(err)
		}
		if st.Ptr != ptr || st.At(0) != 0 {
			t.Fatalf("scale %d ptr %d scratch %d", scale, st.Ptr, st.At(0))
		}
		for i, want := range targets {
			if st.At(i+1) != want {
				t.Fatalf("scale %d cell %d = %d want %d code %s", scale, i+1, st.At(i+1), want, code)
			}
		}
	}
	code := multCode(10, []byte{3, 10, 11})
	if multCost(10, []byte{3, 10, 11}) != len(code) {
		t.Fatal("mult cost")
	}
	st, err := Execute(code, 100000)
	if err != nil {
		t.Fatal(err)
	}
	if st.Ptr != 0 || st.At(0) != 0 || st.At(1) != 30 || st.At(2) != 100 || st.At(3) != 110 {
		t.Fatalf("mult tape %d %d %d %d ptr %d", st.At(0), st.At(1), st.At(2), st.At(3), st.Ptr)
	}

	nest := nestCode(4, 4, []byte{6, 7, 7, 2})
	if nestCost(4, 4, []byte{6, 7, 7, 2}) != len(nest) {
		t.Fatalf("nest cost %d %d", nestCost(4, 4, []byte{6, 7, 7, 2}), len(nest))
	}
	st, err = Execute(nest, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if st.Ptr != 0 || st.At(0) != 0 || st.At(1) != 0 || st.At(2) != 96 || st.At(3) != 112 || st.At(4) != 112 || st.At(5) != 32 {
		t.Fatalf("nest ptr %d tape %d %d %d %d %d %d code %s", st.Ptr, st.At(0), st.At(1), st.At(2), st.At(3), st.At(4), st.At(5), nest)
	}
	if squeeze("><+-><") != "" || squeeze("++><--") != "" || squeeze("+>-<") != "+>-<" || squeeze("[->+<]") != "[->+<]" {
		t.Fatalf("squeeze %q %q %q", squeeze("++><--"), squeeze("+>-<"), squeeze("[->+<]"))
	}
}

func TestSearchShort(t *testing.T) {
	code, err := Search("Ab")
	if err != nil {
		t.Fatal(err)
	}
	st, err := Execute(code, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if st.Out != "Ab" {
		t.Fatalf("out %q code %s", st.Out, code)
	}
	if len(code) >= len(naive("Ab")) {
		t.Fatalf("search did not beat naive: %d %s", len(code), code)
	}
}
