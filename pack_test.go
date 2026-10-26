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

func TestPlane(t *testing.T) {
	st, err := ExecutePack("v+.", "", 20)
	if err != nil || st.Out != "\x01" || st.Ptr != 0 || st.Y != 1 {
		t.Fatalf("down %q x %d y %d %v", st.Out, st.Ptr, st.Y, err)
	}
	st, err = ExecutePack("+v++.^-.", "", 30)
	if err != nil || st.Out != "\x02\x00" || st.Y != 0 || st.Ptr != 0 || st.At(0) != 0 {
		t.Fatalf("two rows %q cell %d y %d %v", st.Out, st.At(0), st.Y, err)
	}
	st, err = ExecutePack(">v<.", "", 20)
	if err != nil || st.Ptr != 0 || st.Y != 1 || st.Out != "\x00" {
		t.Fatalf("corner %q x %d y %d %v", st.Out, st.Ptr, st.Y, err)
	}

	const name = "andrew jordan snodgrass"
	prog := Pack(name)
	if prog != name {
		t.Fatalf("Pack(%q) = %q", name, prog)
	}
	st, err = ExecutePack(prog, "", len(prog)+10)
	if err != nil || st.Out != name || st.Y != 0 {
		t.Fatalf("name %q y %d %v", st.Out, st.Y, err)
	}
}

func TestSpace(t *testing.T) {
	st, err := ExecutePack("}+.", "", 20)
	if err != nil || st.Out != "\x01" || st.Ptr != 0 || st.Y != 0 || len(st.Pos) < 3 || st.Pos[2] != 1 || st.At(0) != 0 {
		t.Fatalf("axis2 out %q pos %v cell %d %v", st.Out, st.Pos, st.At(0), err)
	}
	st, err = ExecutePack(")+.", "", 20)
	if err != nil || st.Out != "\x01" || len(st.Pos) < 4 || st.Pos[3] != 1 || st.Ptr != 0 || st.Y != 0 {
		t.Fatalf("axis3 out %q pos %v %v", st.Out, st.Pos, err)
	}
	st, err = ExecutePack("@\x04+.", "", 20)
	if err != nil || st.Out != "\x01" || len(st.Pos) < 5 || st.Pos[4] != 1 || st.Ptr != 0 || st.Y != 0 || st.At(0) != 0 {
		t.Fatalf("axis4 out %q pos %v %v", st.Out, st.Pos, err)
	}
	st, err = ExecutePack("@\x84-.", "", 20)
	if err != nil || st.Out != "\xff" || len(st.Pos) < 5 || st.Pos[4] != -1 {
		t.Fatalf("neg out %q pos %v %v", st.Out, st.Pos, err)
	}
	// The axis-2 cell is the same cell before and after axis 3 is used.
	st, err = ExecutePack("}+{)(}.", "", 40)
	if err != nil || st.Out != "\x01" || st.Ptr != 0 || st.Y != 0 {
		t.Fatalf("same cell out %q pos %v %v", st.Out, st.Pos, err)
	}
	if _, err := ExecutePack("@", "", 10); err == nil {
		t.Fatal("expected truncated @")
	}
	// The byte after '@' is an operand, including when it looks like '['.
	st, err = ExecutePack("@[+.", "", 20)
	if err != nil || st.Out != "\x01" {
		t.Fatalf("operand bracket out %q %v", st.Out, err)
	}
	if commandOnly("@\x04") || !commandOnly("}+.{") {
		t.Fatal("command set")
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
		"@",
		"}{",
		"()",
		"a[b]",
	}
	for _, text := range cases {
		prog := Pack(text)
		if text != "" && !isSpecial(text[0]) && !strings.ContainsAny(text, "~`|><^v{}()@+-.,[]") && len(prog) > len(text) {
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
