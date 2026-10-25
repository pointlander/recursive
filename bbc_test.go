// Copyright 2026 The Recursive Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"math/rand"
	"testing"
)

func TestBBCKnown(t *testing.T) {
	cases := []struct{ in, out string }{
		{"", ""},
		{"a", "a"},
		{"abc", "cab"},
		{"banana", "annbaa"},
		{"abaababaabaab", "bbbaababaaaaa"},
	}
	for _, tc := range cases {
		if got := BBC(tc.in); got != tc.out {
			t.Fatalf("BBC(%q) = %q want %q", tc.in, got, tc.out)
		}
		if got := InverseBBC(tc.out); got != tc.in {
			t.Fatalf("InverseBBC(%q) = %q want %q", tc.out, got, tc.in)
		}
	}
	factors := lyndonFactors("banana")
	if len(factors) != 4 || factors[0] != "b" || factors[1] != "an" || factors[2] != "an" || factors[3] != "a" {
		t.Fatalf("factors %q", factors)
	}
}

func TestBBCRoundTrip(t *testing.T) {
	alpha := []byte{'a', 'b', 'c'}
	var rec func(prefix string, n int)
	rec = func(prefix string, n int) {
		if n == 0 {
			if got := InverseBBC(BBC(prefix)); got != prefix {
				t.Fatalf("%q -> %q -> %q", prefix, BBC(prefix), got)
			}
			return
		}
		for _, c := range alpha {
			rec(prefix+string(c), n-1)
		}
	}
	for n := 0; n <= 5; n++ {
		rec("", n)
	}

	r := rand.New(rand.NewSource(1))
	for n := 0; n < 60; n++ {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(r.Intn(256))
		}
		text := string(b)
		if got := InverseBBC(BBC(text)); got != text {
			t.Fatalf("n %d mismatch", n)
		}
	}

	const name = "andrew jordan snodgrass"
	if len(BBC(name)) != len(name) {
		t.Fatalf("len %d", len(BBC(name)))
	}
	if InverseBBC(BBC(name)) != name {
		t.Fatalf("name %q", BBC(name))
	}
}
