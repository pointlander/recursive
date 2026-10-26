// Copyright 2026 The Recursive Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"strconv"
	"strings"
)

// axisCount is the number of axes with a one-byte step. The coordinate
// itself is unbounded; '@' steps any later axis.
const axisCount = 4

// axisPair[d] is the positive step and the negative step on axis d.
// > < are axis 0, v ^ are axis 1, } { are axis 2, and ) ( are axis 3.
var axisPair = [axisCount][2]byte{
	{'>', '<'},
	{'v', '^'},
	{'}', '{'},
	{')', '('},
}

// The pack machine is an n-dimensional brainfuck machine with a
// compression command.
//
// The head holds a coordinate that grows as new axes are used. Cells are
// 8-bit and wrap. > and < step the active axis, which starts at 0. v
// increases axis 1 and ^ decreases it. } and { step axis 2. ) and (
// step axis 3. '@' followed by one byte steps any axis: the low 7 bits
// are the axis and the high bit set means negative. +-.,[] add, read,
// write, and loop on the tape. ',' reads the next input byte and stores
// 0 at the end of input. '[' and ']' jump the way they do in Execute.
//
// '*' pushes a zero cell onto a dimension stack and aims ><+-.,[] at
// that stack. On the stack those commands are ordinary brainfuck, and a
// further '*' pushes another zero cell past the cells already used.
// '.' on the stack reads the current cell as an axis index, aims '>' and
// '<' at that axis, and does not write the byte to the output. v^ }{ )(
// keep stepping axes 1, 2, and 3.
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
	if _, _, ok := axisStep(b); ok {
		return true
	}
	switch b {
	case '~', '`', '|', '@', '*', '+', '-', '.', ',', '[', ']':
		return true
	default:
		return false
	}
}

func axisStep(b byte) (dim, delta int, ok bool) {
	for d, p := range axisPair {
		if b == p[0] {
			return d, 1, true
		}
		if b == p[1] {
			return d, -1, true
		}
	}
	return 0, 0, false
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
// truncated '~' or '@' command, an unmatched bracket, or a copy past the
// start of the output.
func ExecutePack(code, input string, maxSteps int) (Stop, error) {
	if maxSteps <= 0 {
		return Stop{}, fmt.Errorf("maxSteps must be positive")
	}
	jump, err := packJumps(code)
	if err != nil {
		return Stop{}, err
	}

	sp := space{row0: tape{cells: []byte{0}}}
	var sel dimSel
	in := 0
	var out []byte
	steps := 0
	for ip := 0; ip < len(code); ip++ {
		steps++
		if steps > maxSteps {
			return Stop{}, fmt.Errorf("exceeded %d steps", maxSteps)
		}
		switch code[ip] {
		case '*':
			sel.push()
		case '+':
			*sel.cell(&sp)++
		case '-':
			*sel.cell(&sp)--
		case '.':
			if sel.on {
				sel.axis = int(*sel.cell(&sp))
				sel.on = false
			} else {
				out = append(out, *sp.at())
			}
		case ',':
			dst := sel.cell(&sp)
			if in < len(input) {
				*dst = input[in]
				in++
			} else {
				*dst = 0
			}
		case '[':
			if *sel.cell(&sp) == 0 {
				ip = jump[ip]
			}
		case ']':
			if *sel.cell(&sp) != 0 {
				ip = jump[ip]
			}
		case '>':
			if sel.on {
				sel.move(1)
			} else {
				sp.step(sel.axis, 1)
			}
		case '<':
			if sel.on {
				sel.move(-1)
			} else {
				sp.step(sel.axis, -1)
			}
		case '@':
			if ip+1 >= len(code) {
				return Stop{}, fmt.Errorf("truncated @ at %d", ip)
			}
			arg := code[ip+1]
			ip++
			delta := 1
			if arg&0x80 != 0 {
				delta = -1
			}
			sp.step(int(arg&0x7f), delta)
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
			if dim, delta, ok := axisStep(code[ip]); ok {
				sp.step(dim, delta)
			} else {
				out = append(out, code[ip])
			}
		}
	}
	x, y, pos := sp.head()
	return Stop{
		Out:   string(out),
		Cells: sp.row0.cells,
		Base:  sp.row0.base,
		Ptr:   x,
		Y:     y,
		Pos:   pos,
		Steps: steps,
	}, nil
}

// dimSel is the stack whose emitted cell chooses the active axis.
// '*' pushes a zero cell and aims the standard commands here. '.' copies
// the current cell into axis and aims those commands back at the tape.
type dimSel struct {
	cells tape
	ptr   int
	hi    int
	on    bool
	used  bool
	axis  int
}

func (d *dimSel) push() {
	if !d.used {
		d.used = true
		d.ptr = 0
		d.hi = 0
		d.on = true
		return
	}
	d.hi++
	d.ptr = d.hi
	*d.cells.at(d.ptr) = 0
	d.on = true
}

func (d *dimSel) move(delta int) {
	d.ptr += delta
	if d.ptr > d.hi {
		d.hi = d.ptr
	}
	_ = d.cells.at(d.ptr)
}

// cell is the stack cell while a dimension is being built, and the tape
// cell otherwise.
func (d *dimSel) cell(sp *space) *byte {
	if d.on {
		return d.cells.at(d.ptr)
	}
	return sp.at()
}

// space is the pack machine's tape. The axis-0 line at the origin of
// every other axis is stored on its own so Stop can report that line the
// way the one-dimensional machine did.
type space struct {
	pos  []int
	row0 tape
	mem  map[string]*byte
}

func (s *space) step(dim, delta int) {
	for len(s.pos) <= dim {
		s.pos = append(s.pos, 0)
	}
	s.pos[dim] += delta
}

func (s *space) head() (x, y int, pos []int) {
	pos = append([]int(nil), s.pos...)
	if len(pos) > 0 {
		x = pos[0]
	}
	if len(pos) > 1 {
		y = pos[1]
	}
	return x, y, pos
}

func (s *space) onRow0() bool {
	for i := 1; i < len(s.pos); i++ {
		if s.pos[i] != 0 {
			return false
		}
	}
	return true
}

func (s *space) at() *byte {
	if s.onRow0() {
		x := 0
		if len(s.pos) > 0 {
			x = s.pos[0]
		}
		return s.row0.at(x)
	}
	key := coordKey(s.pos)
	if s.mem == nil {
		s.mem = make(map[string]*byte)
	}
	p := s.mem[key]
	if p == nil {
		b := byte(0)
		p = &b
		s.mem[key] = p
	}
	return p
}

// coordKey names a cell. Trailing zeros do not change the cell, so a
// visit before and after a higher axis has been used is the same cell.
func coordKey(pos []int) string {
	n := len(pos)
	for n > 0 && pos[n-1] == 0 {
		n--
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Itoa(pos[i]))
	}
	return b.String()
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
		if code[i] == '@' {
			if i+1 >= len(code) {
				return nil, fmt.Errorf("truncated @ at %d", i)
			}
			i += 2
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
