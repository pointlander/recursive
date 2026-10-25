// Copyright 2026 The Recursive Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "fmt"

// The pack machine is brainfuck with a compression command.
//
// ><+-.,[] move, add, read, write, and loop on an 8-bit wrapping tape
// that grows in both directions. ',' reads the next input byte and stores
// 0 at the end of input. '[' and ']' jump the way they do in Execute.
// A byte that is not a command is written straight to the output, so a
// string that contains no command bytes is a program that prints itself.
//
// '~' compresses the bytes that follow it:
//
//	~ 0 X        write the byte X
//	~ N 0 bytes  write the next N bytes as they are (N > 0)
//	~ D L        copy L bytes from D back in the output; the copy may overlap
//
// '`' writes nothing. The rest of the program is a payload in which every
// byte is a literal except '~', so Brainfuck command bytes can be stored
// one for one.
//
// '|' writes nothing. The rest of the program is a bit-packed string:
// a 16-bit length, an alphabet, and symbols packed at ceil(log2(k)) bits
// each, most significant bit first.
//
// Pack returns the shortest of the direct program, the payload program,
// and the bit-packed program. It is the shortest program in this family,
// not a proof of the shortest program the machine can run.

const (
	linkLit byte = iota
	linkMatch
	linkRaw
)

// Pack compresses text into a program for the pack machine.
func Pack(text string) string {
	if text == "" {
		return ""
	}
	best := encode(text, isSpecial)
	payload := encode(text, func(b byte) bool { return b == '~' })
	if len(payload)+1 < len(best) {
		best = "`" + payload
	}
	if packed, ok := bitpack(text); ok && len(packed)+1 < len(best) {
		best = "|" + packed
	}
	return best
}

func encode(text string, special func(byte) bool) string {
	n := len(text)
	const inf = int(^uint(0)>>1) / 4
	cost := make([]int, n+1)
	kind := make([]byte, n+1)
	dist := make([]int, n+1)
	from := make([]int, n+1)
	for i := 1; i <= n; i++ {
		cost[i] = inf
	}
	for i := 0; i < n; i++ {
		c := cost[i] + 1
		if special(text[i]) {
			c = cost[i] + 3
		}
		if c < cost[i+1] {
			cost[i+1] = c
			kind[i+1] = linkLit
			from[i+1] = i
		}
		maxl := n - i
		if maxl > 255 {
			maxl = 255
		}
		for l := 2; l <= maxl; l++ {
			c = cost[i] + 3 + l
			if c < cost[i+l] {
				cost[i+l] = c
				kind[i+l] = linkRaw
				from[i+l] = i
				dist[i+l] = 0
			}
		}
		d, L := longestMatch(text, i)
		for l := 1; l <= L; l++ {
			c = cost[i] + 3
			if c < cost[i+l] {
				cost[i+l] = c
				kind[i+l] = linkMatch
				dist[i+l] = d
				from[i+l] = i
			}
		}
	}

	rev := make([]byte, 0, cost[n])
	for p := n; p > 0; {
		f := from[p]
		switch kind[p] {
		case linkMatch:
			rev = append(rev, byte(p-f), byte(dist[p]), '~')
		case linkRaw:
			for k := p - 1; k >= f; k-- {
				rev = append(rev, text[k])
			}
			rev = append(rev, 0, byte(p-f), '~')
		default:
			b := text[p-1]
			if special(b) {
				rev = append(rev, b, 0, '~')
			} else {
				rev = append(rev, b)
			}
		}
		p = f
	}
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return string(rev)
}

func isSpecial(b byte) bool {
	switch b {
	case '~', '`', '|', '>', '<', '+', '-', '.', ',', '[', ']':
		return true
	default:
		return false
	}
}

// bitpack stores text as a 16-bit length, a first-seen alphabet, and
// packed symbol indexes. It reports ok false when the text is empty, longer
// than 65535 bytes, or uses every byte value.
func bitpack(text string) (string, bool) {
	n := len(text)
	if n == 0 || n > 65535 {
		return "", false
	}
	index := make([]int, 256)
	for i := range index {
		index[i] = -1
	}
	alpha := make([]byte, 0, 16)
	for i := 0; i < n; i++ {
		b := text[i]
		if index[b] >= 0 {
			continue
		}
		if len(alpha) == 255 {
			return "", false
		}
		index[b] = len(alpha)
		alpha = append(alpha, b)
	}
	bits := ceilLog2(len(alpha))
	need := 0
	if bits > 0 {
		need = (n*bits + 7) / 8
	}
	buf := make([]byte, 3+len(alpha)+need)
	buf[0] = byte(n >> 8)
	buf[1] = byte(n)
	buf[2] = byte(len(alpha))
	copy(buf[3:], alpha)
	if bits == 0 {
		return string(buf), true
	}
	data := buf[3+len(alpha):]
	bitpos := 0
	for i := 0; i < n; i++ {
		v := index[text[i]]
		for b := bits - 1; b >= 0; b-- {
			if v&(1<<uint(b)) != 0 {
				data[bitpos/8] |= 1 << uint(7-bitpos%8)
			}
			bitpos++
		}
	}
	return string(buf), true
}

func ceilLog2(k int) int {
	if k <= 1 {
		return 0
	}
	n := 0
	v := 1
	for v < k {
		v *= 2
		n++
	}
	return n
}

func expandBitpack(code string) ([]byte, error) {
	if len(code) < 3 {
		return nil, fmt.Errorf("truncated bitpack")
	}
	n := int(code[0])<<8 | int(code[1])
	k := int(code[2])
	if k == 0 || 3+k > len(code) {
		return nil, fmt.Errorf("truncated bitpack")
	}
	syms := code[3 : 3+k]
	bits := ceilLog2(k)
	data := code[3+k:]
	need := 0
	if bits > 0 {
		need = (n*bits + 7) / 8
	}
	if len(data) != need {
		return nil, fmt.Errorf("truncated bitpack")
	}
	out := make([]byte, 0, n)
	if bits == 0 {
		for i := 0; i < n; i++ {
			out = append(out, syms[0])
		}
		return out, nil
	}
	acc := 0
	for i := 0; i < n; i++ {
		v := 0
		for b := 0; b < bits; b++ {
			bit := (data[acc/8] >> uint(7-acc%8)) & 1
			v = v<<1 | int(bit)
			acc++
		}
		if v >= k {
			return nil, fmt.Errorf("bitpack symbol %d", v)
		}
		out = append(out, syms[v])
	}
	return out, nil
}

func longestMatch(text string, i int) (dist, length int) {
	maxd := i
	if maxd > 255 {
		maxd = 255
	}
	limit := len(text) - i
	if limit > 255 {
		limit = 255
	}
	for d := 1; d <= maxd; d++ {
		n := 0
		for n < limit && text[i+n] == text[i-d+n] {
			n++
		}
		if n > length {
			length = n
			dist = d
		}
	}
	return dist, length
}

// ExecutePack runs code on the pack machine.
// input supplies bytes for ','. Execution fails past maxSteps or on a
// truncated '~' command, an unmatched bracket, or a copy past the start
// of the output.
func ExecutePack(code, input string, maxSteps int) (Stop, error) {
	if maxSteps <= 0 {
		return Stop{}, fmt.Errorf("maxSteps must be positive")
	}
	jump, err := packJumps(code)
	if err != nil {
		return Stop{}, err
	}

	t := tape{cells: []byte{0}}
	ptr := 0
	in := 0
	var out []byte
	steps := 0
	for ip := 0; ip < len(code); ip++ {
		steps++
		if steps > maxSteps {
			return Stop{}, fmt.Errorf("exceeded %d steps", maxSteps)
		}
		switch code[ip] {
		case '>':
			ptr++
		case '<':
			ptr--
		case '+':
			*t.at(ptr)++
		case '-':
			*t.at(ptr)--
		case '.':
			out = append(out, *t.at(ptr))
		case ',':
			if in < len(input) {
				*t.at(ptr) = input[in]
				in++
			} else {
				*t.at(ptr) = 0
			}
		case '[':
			if *t.at(ptr) == 0 {
				ip = jump[ip]
			}
		case ']':
			if *t.at(ptr) != 0 {
				ip = jump[ip]
			}
		case '~':
			var last int
			out, last, err = applyTilde(code, ip, out)
			if err != nil {
				return Stop{}, err
			}
			ip = last
		case '`':
			out, err = expandPayload(code[ip+1:], out)
			if err != nil {
				return Stop{}, err
			}
			ip = len(code)
		case '|':
			var extra []byte
			extra, err = expandBitpack(code[ip+1:])
			if err != nil {
				return Stop{}, err
			}
			out = append(out, extra...)
			ip = len(code)
		default:
			out = append(out, code[ip])
		}
	}
	return Stop{
		Out:   string(out),
		Cells: t.cells,
		Base:  t.base,
		Ptr:   ptr,
		Steps: steps,
	}, nil
}

// applyTilde runs the '~' command at i and returns the index of its last byte.
func applyTilde(code string, i int, out []byte) ([]byte, int, error) {
	last, dist, length, err := readTilde(code, i)
	if err != nil {
		return nil, 0, err
	}
	if dist == 0 {
		out = append(out, byte(length))
	} else if length == 0 {
		out = append(out, code[i+3:i+3+dist]...)
	} else if dist > len(out) {
		return nil, 0, fmt.Errorf("match dist %d exceeds output %d", dist, len(out))
	} else {
		for n := 0; n < length; n++ {
			out = append(out, out[len(out)-dist])
		}
	}
	return out, last, nil
}

func expandPayload(code string, out []byte) ([]byte, error) {
	for i := 0; i < len(code); i++ {
		if code[i] != '~' {
			out = append(out, code[i])
			continue
		}
		var last int
		var err error
		out, last, err = applyTilde(code, i, out)
		if err != nil {
			return nil, err
		}
		i = last
	}
	return out, nil
}

// readTilde parses the '~' command at i and returns the index of its last byte.
func readTilde(code string, i int) (last, dist, length int, err error) {
	if i+2 >= len(code) {
		return 0, 0, 0, fmt.Errorf("truncated ~ at %d", i)
	}
	dist = int(code[i+1])
	length = int(code[i+2])
	last = i + 2
	if dist != 0 && length == 0 {
		if i+3+dist > len(code) {
			return 0, 0, 0, fmt.Errorf("truncated raw block at %d", i)
		}
		last = i + 2 + dist
	}
	return last, dist, length, nil
}

func packJumps(code string) ([]int, error) {
	jump := make([]int, len(code))
	stack := make([]int, 0, 16)
	for i := 0; i < len(code); {
		if code[i] == '`' {
			if err := validatePayload(code[i+1:]); err != nil {
				return nil, err
			}
			break
		}
		if code[i] == '|' {
			if _, err := expandBitpack(code[i+1:]); err != nil {
				return nil, err
			}
			break
		}
		if code[i] == '~' {
			last, _, _, err := readTilde(code, i)
			if err != nil {
				return nil, err
			}
			i = last + 1
			continue
		}
		switch code[i] {
		case '[':
			stack = append(stack, i)
		case ']':
			if len(stack) == 0 {
				return nil, fmt.Errorf("unmatched ] at %d", i)
			}
			open := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			jump[open] = i
			jump[i] = open
		}
		i++
	}
	if len(stack) != 0 {
		return nil, fmt.Errorf("unmatched [ at %d", stack[len(stack)-1])
	}
	return jump, nil
}

func validatePayload(code string) error {
	for i := 0; i < len(code); {
		if code[i] != '~' {
			i++
			continue
		}
		last, _, _, err := readTilde(code, i)
		if err != nil {
			return err
		}
		i = last + 1
	}
	return nil
}
