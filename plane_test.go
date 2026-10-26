// Copyright 2026 The Recursive Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"strings"
	"testing"
)

func TestLoopPlan(t *testing.T) {
	preparePlans()
	cases := []struct{ from, to byte }{
		{0, 1},
		{0, 32},
		{10, 30},
		{97, 32},
		{112, 10},
		{5, 5},
		{0, 255},
		{255, 0},
		{100, 50},
		{32, 119},
	}
	for _, tc := range cases {
		for d := 1; d <= 3; d++ {
			p := plans[d][tc.from][tc.to]
			if p.raw {
				continue
			}
			z := pt{}
			at := pt{d, 0}
			code := strings.Repeat("+", int(tc.from))
			if d > 0 {
				code = strings.Repeat(">", d) + strings.Repeat("+", int(tc.from))
			}
			got := loopCode(p, at, z)
			if len(got) != int(p.cost) {
				t.Fatalf("from %d to %d dist %d code %d plan %d", tc.from, tc.to, d, len(got), p.cost)
			}
			prog := code + got + "."
			st, err := ExecutePack(prog, "", 100000)
			if err != nil {
				t.Fatalf("from %d to %d dist %d: %v code %s", tc.from, tc.to, d, err, prog)
			}
			if st.Out != string([]byte{tc.to}) {
				t.Fatalf("from %d to %d dist %d out %q code %s", tc.from, tc.to, d, st.Out, prog)
			}
		}
		if rawCode(tc.from, tc.to) != "" || tc.from == tc.to {
			prog := rawCode(tc.from, tc.to)
			if tc.from != 0 {
				prog = strings.Repeat("+", int(tc.from)) + rawCode(tc.from, tc.to)
			}
			prog += "."
			st, err := ExecutePack(prog, "", 10000)
			if err != nil || st.Out != string([]byte{tc.to}) {
				t.Fatalf("raw %d -> %d %q %v", tc.from, tc.to, st.Out, err)
			}
		}
	}
}

func TestScaleCode(t *testing.T) {
	for n := 0; n <= 36; n++ {
		for _, b := range scaleVariants(n) {
			st, err := ExecutePack(b.code+".", "", 100000)
			if err != nil {
				t.Fatalf("n %d code %s: %v", n, b.code, err)
			}
			if st.Ptr != b.at[0] || st.Y != b.at[1] || st.Out != string([]byte{byte(n)}) {
				t.Fatalf("n %d at %+v ptr %d y %d out %q code %s", n, b.at, st.Ptr, st.Y, st.Out, b.code)
			}
			if b.at[1] == 0 && b.at[0] != 0 && st.At(0) != 0 {
				t.Fatalf("n %d scratch %d code %s", n, st.At(0), b.code)
			}
		}
	}
	if len(scaleCode(16, pt{1, 0})) >= 16 {
		t.Fatalf("scale 16 %s", scaleCode(16, pt{1, 0}))
	}
}

func TestPlaneDeposit(t *testing.T) {
	pts := []pt{{1, 0}, {1, 1}}
	code := flatCode(3, []byte{1, 2}, pts, pt{}) + ">.v."
	st, err := ExecutePack(code, "", 10000)
	if err != nil {
		t.Fatal(err)
	}
	if st.Out != string([]byte{3, 6}) {
		t.Fatalf("flat out %q code %s", st.Out, code)
	}

	line := linePts(4, 2)
	nest := nest2D(4, 4, []byte{6, 7, 7, 2}, planeInner, line)
	if nest != "++++[>++++[>++++++>+++++++>+++++++>++<<<<-]<-]" {
		t.Fatalf("nest line %s", nest)
	}
	st, err = ExecutePack(nest+">>.>.>.>.", "", 100000)
	if err != nil {
		t.Fatal(err)
	}
	if st.Out != string([]byte{96, 112, 112, 32}) {
		t.Fatalf("nest out %v", []byte(st.Out))
	}

	box := snakePts(4, true)
	square := nest2D(4, 4, []byte{6, 7, 7, 2}, planeInner, box)
	if !strings.ContainsAny(square, "^v") {
		t.Fatalf("square nest %s", square)
	}
	st, err = ExecutePack(square, "", 100000)
	if err != nil {
		t.Fatal(err)
	}
	if st.Y != 0 || st.Ptr != 0 {
		t.Fatalf("square end x %d y %d", st.Ptr, st.Y)
	}
}

func TestSpaceDeposit(t *testing.T) {
	var a, b pt
	a[2] = 1
	b[3] = 1
	pts := []pt{a, b}
	code := flatCode(3, []byte{1, 2}, pts, pt{})
	if !strings.Contains(code, "}") || !strings.Contains(code, ")") {
		t.Fatalf("code %s", code)
	}
	code += move(pt{}, a) + "." + move(a, b) + "."
	st, err := ExecutePack(code, "", 10000)
	if err != nil {
		t.Fatal(err)
	}
	if st.Out != string([]byte{3, 6}) {
		t.Fatalf("space out %q code %s", st.Out, code)
	}
	if !commandOnly(code) {
		t.Fatalf("code %s", code)
	}
}

func TestNearestZeroPlane(t *testing.T) {
	z, d := nearestZero(pt{}, nil)
	want := pt{}
	want[1] = -1
	if d != 1 || z != want {
		t.Fatalf("nearest %v %d", z, d)
	}
	z, d = nearestZero(pt{}, []pt{want})
	if d != 1 || z != (pt{1}) {
		t.Fatalf("next %v %d", z, d)
	}
}

func TestHigherLayouts(t *testing.T) {
	found := false
	for _, set := range layouts(planeFlat, 4, pt{}) {
		for _, p := range set {
			if p[2] != 0 || p[3] != 0 {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("layouts stayed on the first two axes")
	}
}

func TestPlaneShort(t *testing.T) {
	for _, text := range []string{"a", "ab", "hi"} {
		code, err := Plane(text)
		if err != nil {
			t.Fatal(err)
		}
		if !commandOnly(code) {
			t.Fatalf("%q program %q", text, code)
		}
		st, err := ExecutePack(code, "", 2_000_000)
		if err != nil {
			t.Fatal(err)
		}
		if st.Out != text {
			t.Fatalf("%q printed %q code %s", text, st.Out, code)
		}
		if len(code) >= len(naive(text)) {
			t.Fatalf("%q did not beat naive: %d vs %d", text, len(code), len(naive(text)))
		}
	}
}

func TestPlaneName(t *testing.T) {
	const text = "andrew jordan snodgrass"
	code, err := Plane(text)
	if err != nil {
		t.Fatal(err)
	}
	if !commandOnly(code) {
		t.Fatalf("non-command in %q", code)
	}
	st, err := ExecutePack(code, "", 2_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if st.Out != text {
		t.Fatalf("printed %q", st.Out)
	}
	if len(code) >= len(naive(text)) {
		t.Fatalf("len %d naive %d", len(code), len(naive(text)))
	}
	t.Logf("space %d bytes rank %d %s", len(code), spaceRank(code), code)
}
