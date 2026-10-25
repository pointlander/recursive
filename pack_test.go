// Copyright 2026 The Recursive Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"math/rand"
	"os"
	"strings"
	"testing"
)

func TestExecutePack(t *testing.T) {
	code := "++[>++[>++<-]<-]>>."
	want, err := Execute(code, 100000)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ExecutePack(code, "", 100000)
	if err != nil {
		t.Fatal(err)
	}
	if got.Out != want.Out || got.Ptr != want.Ptr || got.At(2) != want.At(2) {
		t.Fatalf("pack bf out %q ptr %d cell %d", got.Out, got.Ptr, got.At(2))
	}

	st, err := ExecutePack(",.", "Q", 100)
	if err != nil || st.Out != "Q" || st.At(0) != 'Q' {
		t.Fatalf("comma %q %v", st.Out, err)
	}
	st, err = ExecutePack(",.", "", 100)
	if err != nil || st.Out != "\x00" {
		t.Fatalf("comma eof %q %v", st.Out, err)
	}
	st, err = ExecutePack("a+.", "", 100)
	if err != nil || st.Out != "a\x01" || st.At(0) != 1 {
		t.Fatalf("literal mix %q cell %d", st.Out, st.At(0))
	}
	st, err = ExecutePack("<+.", "", 100)
	if err != nil || st.Ptr != -1 || st.Out != "\x01" {
		t.Fatalf("left %d %q", st.Ptr, st.Out)
	}
	if _, err := ExecutePack("[", "", 10); err == nil {
		t.Fatal("expected unmatched bracket")
	}
	if _, err := ExecutePack("+[.+]", "", 50); err == nil {
		t.Fatal("expected step limit")
	}
	if _, err := ExecutePack("~", "", 10); err == nil {
		t.Fatal("expected truncated ~")
	}
	if _, err := ExecutePack("~\x02\x00X", "", 10); err == nil {
		t.Fatal("expected truncated raw block")
	}
	if _, err := ExecutePack("~\x01\x01", "", 10); err == nil {
		t.Fatal("expected bad distance")
	}
	// Brackets inside a compression command are data, not jumps.
	st, err = ExecutePack("~\x00[~\x03\x00[+]", "", 100)
	if err != nil || st.Out != "[[+]" {
		t.Fatalf("data brackets %q %v", st.Out, err)
	}
	st, err = ExecutePack("`+.", "", 100)
	if err != nil || st.Out != "+." || st.At(0) != 0 {
		t.Fatalf("payload %q cell %d %v", st.Out, st.At(0), err)
	}
	st, err = ExecutePack("`[", "", 100)
	if err != nil || st.Out != "[" {
		t.Fatalf("payload bracket %q %v", st.Out, err)
	}
	if _, err := ExecutePack("|", "", 10); err == nil {
		t.Fatal("expected truncated bitpack")
	}
}

func TestPackRoundTrip(t *testing.T) {
	cases := []string{
		"",
		"a",
		"\x00",
		"~",
		"+",
		"+-",
		"++++",
		"++++++",
		"hi",
		"andrew jordan snodgrass",
		"abcabcabc",
		strings.Repeat("a", 6),
		strings.Repeat("a", 300),
		strings.Repeat("ab", 200),
		strings.Repeat("andrew jordan snodgrass", 12),
		"~><+-.,[]",
		"a[b]",
	}
	for _, text := range cases {
		prog := Pack(text)
		if text != "" && !isSpecial(text[0]) && !strings.ContainsAny(text, "~><+-.,[]") && len(prog) > len(text) {
			t.Fatalf("%q grew %d -> %d", text, len(text), len(prog))
		}
		st, err := ExecutePack(prog, "", len(prog)+10)
		if err != nil {
			t.Fatalf("%q: %v", text, err)
		}
		if st.Out != text {
			t.Fatalf("%q printed %q (program %q)", text, st.Out, prog)
		}
	}

	exact := []struct{ text, prog string }{
		{"hi", "hi"},
		{"andrew jordan snodgrass", "andrew jordan snodgrass"},
		{"+", "`+"},
		{"+-", "`+-"},
		{"++++", "`+~\x01\x03"},
		{"abcabcabc", "abc~\x03\x06"},
		{"aaaaaa", "a~\x01\x05"},
	}
	for _, tc := range exact {
		if got := Pack(tc.text); got != tc.prog {
			t.Fatalf("Pack(%q) = %q want %q", tc.text, got, tc.prog)
		}
	}

	plain := "andrew jordan snodgrass"
	if len(Pack(plain)) != len(plain) {
		t.Fatalf("name pack %d", len(Pack(plain)))
	}
	repeated := strings.Repeat(plain, 12)
	if len(Pack(repeated)) >= len(repeated) {
		t.Fatalf("repeated name did not shrink: %d -> %d", len(repeated), len(Pack(repeated)))
	}
	run := strings.Repeat("+", 40)
	if len(Pack(run)) >= len(run) {
		t.Fatalf("run did not shrink: %d -> %d", len(run), len(Pack(run)))
	}
}

func TestPackRandom(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for n := 0; n < 80; n++ {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(r.Intn(256))
		}
		text := string(b)
		prog := Pack(text)
		st, err := ExecutePack(prog, "", len(prog)+10)
		if err != nil {
			t.Fatalf("n %d: %v", n, err)
		}
		if st.Out != text {
			t.Fatalf("n %d mismatch", n)
		}
	}
}

func TestBitpack(t *testing.T) {
	packed, ok := bitpack("abc")
	if !ok {
		t.Fatal("bitpack")
	}
	// n=3, alphabet abc, 2 bits each: 00 01 10 -> 0x18
	want := "\x00\x03\x03abc\x18"
	if packed != want {
		t.Fatalf("bitpack %q", packed)
	}
	out, err := expandBitpack(packed)
	if err != nil || string(out) != "abc" {
		t.Fatalf("expand %q %v", out, err)
	}
	st, err := ExecutePack("|"+packed, "", 100)
	if err != nil || st.Out != "abc" {
		t.Fatalf("run %q %v", st.Out, err)
	}

	var all [256]byte
	for i := range all {
		all[i] = byte(i)
	}
	text := string(all[:])
	prog := Pack(text)
	st, err = ExecutePack(prog, "", len(prog)+10)
	if err != nil || st.Out != text {
		t.Fatalf("all bytes %v len %d", err, len(prog))
	}
}

func TestPackSnodgrass(t *testing.T) {
	raw, err := os.ReadFile("snodgrass.bf")
	if err != nil {
		t.Fatal(err)
	}
	prog := Pack(string(raw))
	st, err := ExecutePack(prog, "", len(prog)+10)
	if err != nil {
		t.Fatal(err)
	}
	if st.Out != string(raw) {
		t.Fatal("snodgrass pack mismatch")
	}
	if len(prog) >= len(raw) {
		t.Fatalf("snodgrass.bf did not shrink: %d -> %d", len(raw), len(prog))
	}
	if prog[0] != '|' {
		t.Fatalf("snodgrass pack %q", prog)
	}
	t.Logf("snodgrass.bf %d bytes, pack %d bytes", len(raw), len(prog))
}
