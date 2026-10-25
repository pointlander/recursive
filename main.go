// Copyright 2026 The Recursive Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
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
	if err := os.WriteFile("snodgrass.bf", []byte(code), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%d bytes\n%s\n", len(code), code)
}
