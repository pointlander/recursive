// Copyright 2026 The Recursive Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

// MTF is the move-to-front transform.
//
// The symbol list starts as bytes 0 through 255. Each input byte is replaced
// by its index in that list, and the byte is then moved to the front.
// InverseMTF recovers the original string.
func MTF(s string) string {
	var list [256]byte
	for i := range list {
		list[i] = byte(i)
	}
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		idx := 0
		for list[idx] != c {
			idx++
		}
		out[i] = byte(idx)
		copy(list[1:idx+1], list[:idx])
		list[0] = c
	}
	return string(out)
}

// InverseMTF inverts MTF.
func InverseMTF(s string) string {
	var list [256]byte
	for i := range list {
		list[i] = byte(i)
	}
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		idx := int(s[i])
		c := list[idx]
		out[i] = c
		copy(list[1:idx+1], list[:idx])
		list[0] = c
	}
	return string(out)
}
