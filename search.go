// Copyright 2026 The Recursive Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
)

// Search finds a short brainfuck program that prints text.
//
// It is the shortest program inside the family the search covers, not a
// proof of the globally shortest brainfuck program. The family is:
//   - a flat or nested multiply loop that plants a few anchor cells, and
//   - a print path that only moves, adds, subtracts, and prints.
//
// Add and subtract fragments are searched over loop factors. The print
// path is a greedy walk, then a shortest-path search over which anchor
// prints each character.
func Search(text string) (string, error) {
	if len(text) > 200 {
		return "", fmt.Errorf("text longer than 200 bytes")
	}
	ensurePats()
	if text == "" {
		return "", nil
	}

	base := naive(text)
	p := &pool{
		text:  text,
		best:  len(base),
		naive: len(base),
		limit: 18,
	}
	p.add(cand{kind: kindNaive, cost: len(base)})

	alpha := alphabet(text)
	fmt.Fprintf(os.Stderr, "alphabet %d bytes, naive %d\n", len(alpha), len(base))

	for k := 1; k <= maxAnchors; k++ {
		n := 0
		eachTuple(alpha, k, func(tup []byte) {
			for scale := 1; scale <= maxScale; scale++ {
				n++
				initN, ptr := snappedCost(scale, tup)
				if initN+len(text) >= p.bar() {
					continue
				}
				var vals [8]byte
				copy(vals[1:], tup)
				total := initN + greedyCost(text, &vals, k, ptr)
				if total >= p.bar() {
					continue
				}
				c := cand{kind: kindSnap, cost: total, scale: scale, ptr: ptr, k: k, vals: vals}
				p.add(c)
			}
		})
		fmt.Fprintf(os.Stderr, "snap K=%d tuples*scales=%d best=%d\n", k, n, p.best)
	}

	for scale := 2; scale <= 36; scale++ {
		choices := multChoices(scale, alpha)
		if len(choices) == 0 {
			continue
		}
		limitK := 5
		if limitK > maxAnchors {
			limitK = maxAnchors
		}
		for k := 1; k <= limitK; k++ {
			eachTuple(choices, k, func(tup []byte) {
				for i := 0; i < k; i++ {
					if int(tup[i])*scale > 255 {
						return
					}
				}
				initN := multCost(scale, tup)
				if initN+len(text) >= p.bar() {
					return
				}
				var vals [8]byte
				var mult [8]byte
				for i := 0; i < k; i++ {
					mult[i] = tup[i]
					vals[i+1] = byte(int(tup[i]) * scale)
				}
				total := initN + greedyCost(text, &vals, k, 0)
				if total >= p.bar() {
					return
				}
				p.add(cand{kind: kindMult, cost: total, scale: scale, k: k, vals: vals, mult: mult})
			})
		}
	}
	fmt.Fprintf(os.Stderr, "after multiples best=%d\n", p.best)

	for prod := 2; prod <= 48; prod++ {
		a, b := bestFactors(prod)
		if b == 1 {
			continue
		}
		choices := multChoices(prod, alpha)
		if len(choices) == 0 {
			continue
		}
		for k := 1; k <= 4; k++ {
			eachTuple(choices, k, func(tup []byte) {
				initN := nestCost(a, b, tup)
				if initN+len(text) >= p.bar() {
					return
				}
				vals, highest := nestVals(a, b, tup)
				total := initN + greedyCost(text, &vals, highest, 0)
				if total >= p.bar() {
					return
				}
				var mult [8]byte
				copy(mult[:], tup)
				p.add(cand{
					kind: kindNest, cost: total, scale: a, ptr: b, k: k,
					vals: vals, mult: mult,
				})
			})
		}
	}
	fmt.Fprintf(os.Stderr, "after nested best=%d\n", p.best)

	for k := 1; k <= maxAnchors; k++ {
		var vals [8]byte
		total := greedyCost(text, &vals, k, 0)
		if total < p.bar() {
			p.add(cand{kind: kindZero, cost: total, k: k})
		}
	}
	fmt.Fprintf(os.Stderr, "after zero-fill best=%d pool=%d\n", p.best, len(p.top))

	bestCode := base
	bestN := len(base)
	for i := range p.top {
		code, n, err := realize(text, p.top[i])
		if err != nil {
			fmt.Fprintf(os.Stderr, "reject %v\n", err)
			continue
		}
		if n < bestN {
			bestN = n
			bestCode = code
		}
		ak := p.top[i].anchors()
		if ak > 5 || p.top[i].kind == kindNaive {
			continue
		}
		init, vals, ptr := materialize(p.top[i])
		room := bestN - len(init)
		own := n - len(init)
		if room > own {
			room = own
		}
		if room <= len(text) {
			continue
		}
		printed, pc, ok := optimizePrint(text, vals, ak, ptr, room)
		if !ok || len(init)+pc >= bestN {
			continue
		}
		candCode := squeeze(init + printed)
		if err := checkProgram(text, candCode); err != nil {
			fmt.Fprintf(os.Stderr, "dp reject %v\n", err)
			continue
		}
		fmt.Fprintf(os.Stderr, "dp %d -> %d\n", n, len(candCode))
		if len(candCode) < bestN {
			bestN = len(candCode)
			bestCode = candCode
		}
	}
	bestCode = squeeze(bestCode)
	if err := checkProgram(text, bestCode); err != nil {
		return "", err
	}
	return bestCode, nil
}

const (
	maxAnchors = 6
	maxScale   = 48
)

const (
	kindNaive = iota
	kindSnap
	kindMult
	kindZero
	kindNest
)

const (
	patNop = iota
	patPlus
	patMinus
	patAdd
	patSub
)

type pat struct {
	clear  bool
	k      byte
	q      uint16
	f      uint16
	r      int16
	length uint16
	lead   uint8
}

var (
	patOnce sync.Once
	pats    [7][256][256]pat
)

func ensurePats() {
	patOnce.Do(func() {
		for dist := 1; dist <= maxAnchors; dist++ {
			for from := 0; from < 256; from++ {
				for to := 0; to < 256; to++ {
					p := searchPat(byte(from), byte(to), dist)
					s := p.code(dist)
					p.length = uint16(len(s))
					p.lead = uint8(leadingLT(s))
					pats[dist][from][to] = p
				}
			}
		}
	})
}

func searchPat(from, to byte, dist int) pat {
	best, bestN := basePat(from, to, dist)
	if from != to && from != 0 {
		z, zn := basePat(0, to, dist)
		if 3+zn < bestN {
			z.clear = true
			best = z
		}
	}
	return best
}

func basePat(from, to byte, dist int) (pat, int) {
	if from == to {
		return pat{}, 0
	}
	add := (int(to) - int(from) + 256) % 256
	sub := 256 - add
	best := pat{k: patPlus, q: uint16(add)}
	bestN := add
	if sub < bestN {
		best = pat{k: patMinus, q: uint16(sub)}
		bestN = sub
	}
	try := func(kind byte, q, f, r int) {
		if q <= 0 || f <= 0 {
			return
		}
		n := q + f + abs(r) + 4*dist + 3
		if n < bestN {
			bestN = n
			best = pat{k: kind, q: uint16(q), f: uint16(f), r: int16(r)}
		}
	}
	searchAmount := func(amount int, kind byte, extraSign int) {
		for f := 1; f < bestN && f <= amount; f++ {
			q := amount / f
			rem := amount - q*f
			if q == 0 {
				continue
			}
			try(kind, q, f, extraSign*rem)
			if rem != 0 {
				try(kind, q+1, f, -extraSign*(f-rem))
			}
		}
	}
	if add != 0 {
		searchAmount(add, patAdd, +1)
	}
	if sub != 0 && sub != add {
		searchAmount(sub, patSub, -1)
	} else if sub != 0 && sub == add {
		searchAmount(sub, patSub, -1)
	}
	return best, bestN
}

func (p pat) code(dist int) string {
	b := make([]byte, 0, 48)
	if p.clear {
		b = append(b, '[', '-', ']')
	}
	switch p.k {
	case patPlus:
		b = appendCount(b, '+', int(p.q))
	case patMinus:
		b = appendCount(b, '-', int(p.q))
	case patAdd, patSub:
		b = appendCount(b, '<', dist)
		b = appendCount(b, '+', int(p.q))
		b = append(b, '[', '-')
		b = appendCount(b, '>', dist)
		body := byte('+')
		if p.k == patSub {
			body = '-'
		}
		b = appendCount(b, body, int(p.f))
		b = appendCount(b, '<', dist)
		b = append(b, ']')
		b = appendCount(b, '>', dist)
		if p.r > 0 {
			b = appendCount(b, '+', int(p.r))
		} else if p.r < 0 {
			b = appendCount(b, '-', int(-p.r))
		}
	}
	return string(b)
}

func appendCount(b []byte, c byte, n int) []byte {
	for i := 0; i < n; i++ {
		b = append(b, c)
	}
	return b
}

func leadingLT(s string) int {
	i := 0
	for i < len(s) && s[i] == '<' {
		i++
	}
	return i
}

func addedCost(pos, dist, length, lead int) int {
	if dist == pos {
		return length
	}
	if dist > pos {
		mv := dist - pos
		cancel := lead
		if cancel > mv {
			cancel = mv
		}
		return mv + length - 2*cancel
	}
	return pos - dist + length
}

func join(pos, dist int, frag string) string {
	if dist == pos {
		return frag
	}
	var mv []byte
	if dist > pos {
		mv = appendCount(nil, '>', dist-pos)
		i := 0
		for i < len(mv) && i < len(frag) && frag[i] == '<' {
			i++
		}
		out := make([]byte, 0, len(mv)+len(frag)-2*i)
		out = append(out, mv[:len(mv)-i]...)
		out = append(out, frag[i:]...)
		return string(out)
	}
	mv = appendCount(nil, '<', pos-dist)
	out := make([]byte, 0, len(mv)+len(frag))
	out = append(out, mv...)
	out = append(out, frag...)
	return string(out)
}

func bestMult(scale, target int) (m, r int) {
	if scale <= 0 {
		return 0, target
	}
	m = target / scale
	r = target - m*scale
	m2 := m + 1
	r2 := target - m2*scale
	if m2+abs(r2) < m+abs(r) {
		return m2, r2
	}
	return m, r
}

func snappedCost(scale int, targets []byte) (n, ptr int) {
	k := len(targets)
	var rems [8]int
	n = scale + 1
	for i := 0; i < k; i++ {
		m, r := bestMult(scale, int(targets[i]))
		rems[i] = r
		n += 1 + m
	}
	n += k + 2
	last := -1
	for i := 0; i < k; i++ {
		if rems[i] != 0 {
			last = i
		}
	}
	if last >= 0 {
		for i := 0; i <= last; i++ {
			n += 1 + abs(rems[i])
		}
		ptr = last + 1
	}
	return n, ptr
}

func snappedCode(scale int, targets []byte) (string, int) {
	k := len(targets)
	mults := make([]int, k)
	rems := make([]int, k)
	for i, t := range targets {
		mults[i], rems[i] = bestMult(scale, int(t))
	}
	var b strings.Builder
	b.Grow(scale + 64)
	b.WriteString(strings.Repeat("+", scale))
	b.WriteByte('[')
	for _, m := range mults {
		b.WriteByte('>')
		b.WriteString(strings.Repeat("+", m))
	}
	b.WriteString(strings.Repeat("<", k))
	b.WriteString("-]")
	last := -1
	for i, r := range rems {
		if r != 0 {
			last = i
		}
	}
	ptr := 0
	if last >= 0 {
		for i := 0; i <= last; i++ {
			b.WriteByte('>')
			if rems[i] > 0 {
				b.WriteString(strings.Repeat("+", rems[i]))
			} else if rems[i] < 0 {
				b.WriteString(strings.Repeat("-", -rems[i]))
			}
		}
		ptr = last + 1
	}
	return b.String(), ptr
}

func multChoices(scale int, alpha []byte) []byte {
	seen := make([]bool, 256)
	var out []byte
	for _, c := range alpha {
		m := int(c) / scale
		for _, mm := range []int{m, m + 1} {
			if mm < 0 || mm > 255 || mm*scale > 255 || seen[mm] {
				continue
			}
			seen[mm] = true
			out = append(out, byte(mm))
		}
	}
	return out
}

func multCost(scale int, mults []byte) int {
	n := scale + 1
	for _, m := range mults {
		n += 1 + int(m)
	}
	n += len(mults) + 2
	return n
}

func multCode(scale int, mults []byte) string {
	var b strings.Builder
	b.WriteString(strings.Repeat("+", scale))
	b.WriteByte('[')
	for _, m := range mults {
		b.WriteByte('>')
		b.WriteString(strings.Repeat("+", int(m)))
	}
	b.WriteString(strings.Repeat("<", len(mults)))
	b.WriteString("-]")
	return b.String()
}

func bestFactors(prod int) (a, b int) {
	a, b = 1, prod
	best := a + b
	for x := 1; x*x <= prod; x++ {
		if prod%x != 0 {
			continue
		}
		y := prod / x
		if x+y < best {
			best = x + y
			a, b = x, y
		}
	}
	return a, b
}

func nestCost(a, b int, mults []byte) int {
	n := a + 2 + b + 1
	for _, m := range mults {
		n += 1 + int(m)
	}
	n += len(mults) + 2 + 3
	return n
}

func nestCode(a, b int, mults []byte) string {
	var buf strings.Builder
	buf.WriteString(strings.Repeat("+", a))
	buf.WriteString("[>")
	buf.WriteString(strings.Repeat("+", b))
	buf.WriteByte('[')
	for _, m := range mults {
		buf.WriteByte('>')
		buf.WriteString(strings.Repeat("+", int(m)))
	}
	buf.WriteString(strings.Repeat("<", len(mults)))
	buf.WriteString("-]<-]")
	return buf.String()
}

func nestVals(a, b int, mults []byte) (vals [8]byte, highest int) {
	highest = len(mults) + 1
	for i, m := range mults {
		vals[i+2] = byte(a * b * int(m))
	}
	return vals, highest
}

func greedyCost(text string, vals *[8]byte, k, ptr int) int {
	var cur [8]byte
	cur = *vals
	total := 0
	for i := 0; i < len(text); i++ {
		ch := text[i]
		bestC := math.MaxInt
		bestJ := 1
		for j := 1; j <= k; j++ {
			p := &pats[j][cur[j]][ch]
			c := addedCost(ptr, j, int(p.length), int(p.lead)) + 1
			if c < bestC {
				bestC = c
				bestJ = j
			}
		}
		total += bestC
		cur[bestJ] = ch
		ptr = bestJ
	}
	return total
}

func greedyCode(text string, vals []byte, ptr int) string {
	cur := append([]byte(nil), vals...)
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		ch := text[i]
		bestC := math.MaxInt
		bestJ := 1
		for j := 1; j < len(cur); j++ {
			p := &pats[j][cur[j]][ch]
			c := addedCost(ptr, j, int(p.length), int(p.lead)) + 1
			if c < bestC {
				bestC = c
				bestJ = j
			}
		}
		frag := pats[bestJ][cur[bestJ]][ch].code(bestJ)
		b.WriteString(join(ptr, bestJ, frag))
		b.WriteByte('.')
		cur[bestJ] = ch
		ptr = bestJ
	}
	return b.String()
}

func naive(text string) string {
	var b strings.Builder
	cur := 0
	for i := 0; i < len(text); i++ {
		t := int(text[i])
		d := (t - cur + 256) % 256
		if d > 128 {
			b.WriteString(strings.Repeat("-", 256-d))
		} else {
			b.WriteString(strings.Repeat("+", d))
		}
		b.WriteByte('.')
		cur = t
	}
	return b.String()
}

type cand struct {
	kind  int
	cost  int
	scale int
	ptr   int
	k     int
	vals  [8]byte
	mult  [8]byte
}

func (c cand) anchors() int {
	if c.kind == kindNest {
		return c.k + 1
	}
	return c.k
}

type pool struct {
	text  string
	best  int
	naive int
	limit int
	top   []cand
}

func (p *pool) bar() int {
	if len(p.top) < p.limit {
		return p.naive
	}
	return p.top[len(p.top)-1].cost
}

func (p *pool) add(c cand) {
	if len(p.top) == p.limit && c.cost >= p.top[len(p.top)-1].cost {
		return
	}
	if c.cost < p.best {
		p.best = c.cost
		fmt.Fprintf(os.Stderr, "best %d kind=%d K=%d scale=%d\n", c.cost, c.kind, c.k, c.scale)
	}
	p.top = append(p.top, c)
	sort.Slice(p.top, func(i, j int) bool { return p.top[i].cost < p.top[j].cost })
	if len(p.top) > p.limit {
		p.top = p.top[:p.limit]
	}
}

func materialize(c cand) (init string, vals [8]byte, ptr int) {
	switch c.kind {
	case kindSnap:
		init, ptr = snappedCode(c.scale, c.vals[1:c.k+1])
		vals = c.vals
	case kindMult:
		init = multCode(c.scale, c.mult[:c.k])
		for i := 0; i < c.k; i++ {
			vals[i+1] = byte(int(c.mult[i]) * c.scale)
		}
	case kindZero:
		// vals stay 0, pointer stays 0
	case kindNest:
		init = nestCode(c.scale, c.ptr, c.mult[:c.k])
		vals, _ = nestVals(c.scale, c.ptr, c.mult[:c.k])
	default:
		ptr = 0
	}
	return init, vals, ptr
}

func realize(text string, c cand) (string, int, error) {
	if c.kind == kindNaive {
		code := naive(text)
		if err := checkProgram(text, code); err != nil {
			return "", 0, err
		}
		return code, len(code), nil
	}
	init, vals, ptr := materialize(c)
	if c.kind == kindSnap {
		n, p := snappedCost(c.scale, c.vals[1:c.k+1])
		if n != len(init) || p != ptr {
			return "", 0, fmt.Errorf("init length drift %d vs %d ptr %d vs %d", n, len(init), p, ptr)
		}
	}
	if c.kind == kindMult && multCost(c.scale, c.mult[:c.k]) != len(init) {
		return "", 0, fmt.Errorf("mult length drift")
	}
	if c.kind == kindNest && nestCost(c.scale, c.ptr, c.mult[:c.k]) != len(init) {
		return "", 0, fmt.Errorf("nest length drift")
	}
	ak := c.anchors()
	body := greedyCode(text, vals[:ak+1], ptr)
	g := greedyCost(text, &vals, ak, ptr)
	if g != len(body) {
		return "", 0, fmt.Errorf("greedy length drift %d vs %d", g, len(body))
	}
	code := squeeze(init + body)
	if err := checkProgram(text, code); err != nil {
		return "", 0, err
	}
	return code, len(code), nil
}

func checkProgram(text, code string) error {
	st, err := Execute(code, 2_000_000)
	if err != nil {
		return err
	}
	if st.Out != text {
		return fmt.Errorf("output %q", st.Out)
	}
	return nil
}

type dkey struct {
	i, pos uint8
	v      [6]byte
}

type dnode struct {
	cost int
	i    int
	pos  int
	v    [6]byte
	prev int
	aj   int
}

// optimizePrint searches the shortest move/adjust/print sequence over k anchors.
// incumbent is the best known print-section length. The returned code is shorter
// than that, or ok is false.
func optimizePrint(text string, vals [8]byte, k, ptr, incumbent int) (string, int, bool) {
	if k <= 0 || k > 5 || incumbent <= 1 || len(text) > 255 {
		return "", 0, false
	}
	nodes := make([]dnode, 0, 4096)
	start := dnode{cost: 0, i: 0, pos: ptr, prev: -1, aj: -1}
	for a := 0; a < k; a++ {
		start.v[a] = vals[a+1]
	}
	nodes = append(nodes, start)

	best := make(map[dkey]int, 8192)
	sk := dkey{i: 0, pos: uint8(ptr)}
	copy(sk.v[:k], start.v[:k])
	best[sk] = 0

	buckets := make([][]int, incumbent)
	buckets[0] = []int{0}
	goal := -1
	for cost := 0; cost < incumbent && goal < 0; cost++ {
		layer := buckets[cost]
		for _, id := range layer {
			n := nodes[id]
			if n.cost != cost {
				continue
			}
			key := dkey{i: uint8(n.i), pos: uint8(n.pos)}
			copy(key.v[:k], n.v[:k])
			if known, ok := best[key]; !ok || known != cost {
				continue
			}
			if n.i == len(text) {
				goal = id
				break
			}
			if cost+len(text)-n.i >= incumbent {
				continue
			}
			ch := text[n.i]
			for a := 0; a < k; a++ {
				dist := a + 1
				fp := pats[dist][n.v[a]][ch]
				add := addedCost(n.pos, dist, int(fp.length), int(fp.lead)) + 1
				nc := cost + add
				left := len(text) - (n.i + 1)
				if nc >= incumbent || nc+left >= incumbent || nc >= len(buckets) {
					continue
				}
				var nv [6]byte
				copy(nv[:k], n.v[:k])
				nv[a] = ch
				nk := dkey{i: uint8(n.i + 1), pos: uint8(dist)}
				copy(nk.v[:k], nv[:k])
				if old, ok := best[nk]; ok && nc >= old {
					continue
				}
				best[nk] = nc
				nodes = append(nodes, dnode{cost: nc, i: n.i + 1, pos: dist, v: nv, prev: id, aj: a})
				buckets[nc] = append(buckets[nc], len(nodes)-1)
				if len(nodes) > 2_500_000 {
					fmt.Fprintf(os.Stderr, "dp cap at %d nodes\n", len(nodes))
					return "", 0, false
				}
			}
		}
	}
	if goal < 0 {
		return "", 0, false
	}
	steps := make([]int, 0, len(text))
	for id := goal; nodes[id].prev >= 0; id = nodes[id].prev {
		steps = append(steps, nodes[id].aj)
	}
	if len(steps) != len(text) {
		return "", 0, false
	}
	for i, j := 0, len(steps)-1; i < j; i, j = i+1, j-1 {
		steps[i], steps[j] = steps[j], steps[i]
	}
	cur := vals
	pos := ptr
	var b strings.Builder
	for i, a := range steps {
		dist := a + 1
		ch := text[i]
		frag := pats[dist][cur[dist]][ch].code(dist)
		b.WriteString(join(pos, dist, frag))
		b.WriteByte('.')
		cur[dist] = ch
		pos = dist
	}
	if b.Len() != nodes[goal].cost {
		return "", 0, false
	}
	return b.String(), b.Len(), true
}

func alphabet(s string) []byte {
	var seen [256]bool
	var a []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !seen[c] {
			seen[c] = true
			a = append(a, c)
		}
	}
	sort.Slice(a, func(i, j int) bool { return a[i] < a[j] })
	return a
}

func eachTuple(alpha []byte, k int, fn func([]byte)) {
	tup := make([]byte, k)
	var rec func(int)
	rec = func(d int) {
		if d == k {
			fn(tup)
			return
		}
		for _, c := range alpha {
			tup[d] = c
			rec(d + 1)
		}
	}
	rec(0)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func squeeze(code string) string {
	b := []byte(code)
	for {
		out := make([]byte, 0, len(b))
		changed := false
		for i := 0; i < len(b); i++ {
			if i+1 < len(b) && cancels(b[i], b[i+1]) {
				i++
				changed = true
				continue
			}
			out = append(out, b[i])
		}
		b = out
		if !changed {
			return string(b)
		}
	}
}

func cancels(a, b byte) bool {
	if (a == '+' && b == '-') || (a == '-' && b == '+') {
		return true
	}
	for _, p := range axisPair {
		if (a == p[0] && b == p[1]) || (a == p[1] && b == p[0]) {
			return true
		}
	}
	return false
}
