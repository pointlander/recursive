// Copyright 2026 The Recursive Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "sort"

// BBC is the bijective Burrows-Wheeler transform.
//
// The string is factored into non-increasing Lyndon words. Every rotation
// of those words is sorted in infinite lexicographic order, and BBC returns
// the last byte of each rotation. The result has the same length as s.
// InverseBBC recovers s.
func BBC(s string) string {
	if s == "" {
		return ""
	}
	var rots []string
	for _, f := range lyndonFactors(s) {
		for i := 0; i < len(f); i++ {
			rots = append(rots, f[i:]+f[:i])
		}
	}
	sort.Slice(rots, func(i, j int) bool {
		return lessOmega(rots[i], rots[j])
	})
	out := make([]byte, len(rots))
	for i, r := range rots {
		out[i] = r[len(r)-1]
	}
	return string(out)
}

// InverseBBC inverts BBC.
//
// The standard permutation lists string positions in stable symbol order.
// Each cycle spells a rotation of one Lyndon factor. Those factors are
// rotated to their minimum form, sorted into non-increasing order, and
// concatenated.
func InverseBBC(s string) string {
	n := len(s)
	if n == 0 {
		return ""
	}
	// perm[i] is the string index of the i-th symbol in stable sorted order.
	perm := make([]int, n)
	for i := 0; i < n; i++ {
		perm[i] = i
	}
	sort.SliceStable(perm, func(i, j int) bool {
		return s[perm[i]] < s[perm[j]]
	})
	next := make([]int, n)
	copy(next, perm)
	words := make([]string, 0, 8)
	for i := 0; i < n; i++ {
		if next[i] < 0 {
			continue
		}
		buf := make([]byte, 0, 8)
		j := i
		for next[j] >= 0 {
			k := next[j]
			buf = append(buf, s[k])
			next[j] = -1
			j = k
		}
		if len(buf) > 0 {
			words = append(words, minRotation(string(buf)))
		}
	}
	sort.Slice(words, func(i, j int) bool {
		return words[i] > words[j]
	})
	out := make([]byte, 0, n)
	for _, w := range words {
		out = append(out, w...)
	}
	return string(out)
}

// lyndonFactors is Duval's factorization into non-increasing Lyndon words.
func lyndonFactors(s string) []string {
	n := len(s)
	factors := make([]string, 0, 8)
	for i := 0; i < n; {
		j := i + 1
		k := i
		for j < n && s[k] <= s[j] {
			if s[k] < s[j] {
				k = i
			} else {
				k++
			}
			j++
		}
		for i <= k {
			factors = append(factors, s[i:i+j-k])
			i += j - k
		}
	}
	return factors
}

// lessOmega reports whether a repeated forever precedes b repeated forever.
func lessOmega(a, b string) bool {
	if a == b {
		return false
	}
	ia, ib := 0, 0
	limit := len(a) * len(b)
	for k := 0; k < limit; k++ {
		if a[ia] != b[ib] {
			return a[ia] < b[ib]
		}
		ia++
		if ia == len(a) {
			ia = 0
		}
		ib++
		if ib == len(b) {
			ib = 0
		}
	}
	return false
}

func minRotation(s string) string {
	best := s
	for i := 1; i < len(s); i++ {
		r := s[i:] + s[:i]
		if r < best {
			best = r
		}
	}
	return best
}
