// Copyright 2026 The Recursive Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"compress/flate"
	"fmt"
	"io"
	"os"
)

func main() {
	const text = "andrew jordan snodgrass"
	code, err := Search(text)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	st, err := Execute(code, 2_000_000)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if st.Out != text {
		fmt.Fprintf(os.Stderr, "machine printed %q\n", st.Out)
		os.Exit(1)
	}
	prog := []byte(code)
	if err := os.WriteFile("snodgrass.bf", prog, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%d bytes\n%s\n", len(prog), code)

	bbc := BBC(text)
	mtf := MTF(bbc)
	if InverseBBC(InverseMTF(mtf)) != text {
		fmt.Fprintf(os.Stderr, "bbc mtf inverse %q\n", InverseBBC(InverseMTF(mtf)))
		os.Exit(1)
	}
	fmt.Printf("bbc %q\n", bbc)
	fmt.Printf("mtf %q\n", mtf)

	namePacked, err := flateCompress([]byte(mtf))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	progPacked, err := flateCompress(prog)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("name bbc mtf %d bytes, flate %d bytes\n", len(mtf), len(namePacked))
	fmt.Printf("program %d bytes, flate %d bytes\n", len(prog), len(progPacked))

	namePack := Pack(mtf)
	st, err = ExecutePack(namePack, "", 2_000_000)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if st.Out != mtf || InverseBBC(InverseMTF(st.Out)) != text {
		fmt.Fprintf(os.Stderr, "pack printed %q\n", st.Out)
		os.Exit(1)
	}
	progPack := Pack(code)
	st, err = ExecutePack(progPack, "", 2_000_000)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if st.Out != code {
		fmt.Fprintf(os.Stderr, "pack printed %q\n", st.Out)
		os.Exit(1)
	}
	fmt.Printf("name bbc mtf pack %d bytes\n", len(namePack))
	fmt.Printf("program pack %d bytes\n", len(progPack))

	namePlane, err := Plane(text)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(code) < len(namePlane) && commandOnly(code) {
		namePlane = code
	}
	st, err = ExecutePack(namePlane, "", 2_000_000)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if st.Out != text || !commandOnly(namePlane) {
		fmt.Fprintf(os.Stderr, "plane printed %q\n", st.Out)
		os.Exit(1)
	}
	fmt.Printf("name plane %d bytes\n%s\n", len(namePlane), namePlane)
}

// flateCompress returns data compressed with compress/flate at the best level.
// The result is raw DEFLATE, and inflating it yields data again.
func flateCompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	out := buf.Bytes()
	got, err := flateDecompress(out)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(got, data) {
		return nil, fmt.Errorf("flate round trip mismatch: got %d bytes, want %d", len(got), len(data))
	}
	return out, nil
}

func flateDecompress(data []byte) ([]byte, error) {
	r := flate.NewReader(bytes.NewReader(data))
	defer r.Close()
	return io.ReadAll(r)
}
