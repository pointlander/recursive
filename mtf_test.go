// Copyright 2026 The Recursive Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"math/rand"
	"testing"
)

func TestMTFKnown(t *testing.T) {
	if MTF("") != "" || InverseMTF("") != "" {
		t.Fatal("empty")
	}
	if got := MTF("aaa"); got != "\x61\x00\x00" {
		t.Fatalf("aaa %q", got)
	}
	if got := MTF("aba"); got != "\x61\x62\x01" {
		t.Fatalf("aba %q", got)
	}
	if InverseMTF(MTF("aba")) != "aba" {
		t.Fatal("aba inverse")
	}
}

func TestMTFRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for n := 0; n < 80; n++ {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(r.Intn(256))
		}
		text := string(b)
		if got := InverseMTF(MTF(text)); got != text {
			t.Fatalf("n %d mismatch", n)
		}
	}

	const name = "andrew jordan snodgrass"
	coded := MTF(BBC(name))
	if len(coded) != len(name) {
		t.Fatalf("len %d", len(coded))
	}
	if InverseBBC(InverseMTF(coded)) != name {
		t.Fatalf("name %q", coded)
	}
}
