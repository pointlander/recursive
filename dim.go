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
)

// dimCap is how far the dimension-stack search places an anchor.
// Axes 0..3 already have a one-byte step. A later axis is reached by
// '*' and then '>' or '<'.
const dimCap = 6

// squareOff is the anchor layout of the short rank-2 program, as steps
// away from the scale counter. An arm on a later axis is added to it.
var squareOff = [][dimCap]int{
	{1, 0},
	{1, -1},
	{2, -1},
	{2, 0},
}

var selCache = func() [48]string {
	var c [48]string
	for i := range c {
		c[i] = makeSelect(i)
	}
	return c
}()

// stackGen is one multiply-loop program whose anchors may sit on an axis
// selected with '*'. offs are steps from the counter. For a nested loop
// the counter is the inner cell and the outer counter stays at the origin.
type stackGen struct {
	text    string
	kind    int
	a, b    int
	mults   []byte
	offs    [][dimCap]int
	counter [dimCap]int
	scale   string
	pts     [][dimCap]int
	// lockAssign keeps mults[i] on offs[i]. The sweep already tries every
	// ordered tuple, so a second assignment pass would only repeat work.
	lockAssign bool
	initOnly   bool
}

type stackPick struct {
	cost int
	code string
}

// searchStack plants anchors on axes chosen with the dimension stack and
// keeps a program only when execution shows it is shorter than the
// one-byte-axis program already found.
func (s *planeSearch) searchStack() error {
	preparePlans()
	best := stackPick{cost: math.MaxInt}
	consider := func(g *stackGen) {
		if !g.high() {
			return
		}
		assign, ord, cost := g.best()
		if cost == math.MaxInt {
			return
		}
		if best.cost < math.MaxInt-8 && cost >= best.cost+8 {
			return
		}
		code, n := g.eval(assign, ord, true)
		if n != cost {
			fmt.Fprintf(os.Stderr, "space stack cost %d len %d\n", cost, n)
			return
		}
		code = squeeze(code)
		if len(code) >= best.cost || !strings.Contains(code, "*") || !commandOnly(code) {
			return
		}
		best.cost = len(code)
		best.code = code
	}
	s.stackFromPool(consider)
	s.stackSweep(consider)
	if best.code == "" {
		fmt.Fprintf(os.Stderr, "space stack none\n")
		return nil
	}
	if err := verifyPlane(s.text, best.code); err != nil {
		fmt.Fprintf(os.Stderr, "space stack reject %d: %v\n", len(best.code), err)
		return nil
	}
	fmt.Fprintf(os.Stderr, "space stack %d bytes\n", len(best.code))
	if len(best.code) < s.bestLen {
		s.bestLen = len(best.code)
		s.bestCode = best.code
		fmt.Fprintf(os.Stderr, "space %d bytes\n", len(best.code))
	}
	return nil
}

func (s *planeSearch) stackFromPool(consider func(*stackGen)) {
	seen := make(map[string]bool)
	var try func(kind, a, b int, mults []byte)
	try = func(kind, a, b int, mults []byte) {
		key := fmt.Sprintf("%d:%d:%d:%v", kind, a, b, mults)
		if seen[key] {
			return
		}
		seen[key] = true
		s.starGens(kind, a, b, mults, false, consider)
		if len(mults) == 4 && a > 0 {
			s.armGens(kind, a, b, mults, consider)
		}
	}
	for i := range s.pool {
		c := &s.pool[i]
		try(c.kind, c.a, c.b, append([]byte(nil), c.mult[:c.k]...))
	}
}

func (s *planeSearch) stackSweep(consider func(*stackGen)) {
	if len(s.alpha) == 0 {
		return
	}
	order := []int{16, 12, 24, 32, 8, 18, 20, 10, 15, 14, 28, 36}
	seen := make(map[int]bool, planeMaxScale)
	for _, sc := range order {
		seen[sc] = true
	}
	for sc := 2; sc <= planeMaxScale; sc++ {
		if !seen[sc] {
			order = append(order, sc)
		}
	}
	for _, scale := range order {
		choices := append([]byte(nil), multChoices(scale, s.alpha)...)
		sort.Slice(choices, func(i, j int) bool { return choices[i] < choices[j] })
		if len(choices) == 0 {
			continue
		}
		for k := 2; k <= 4; k++ {
			if powOver(len(choices), k, 700) {
				continue
			}
			eachBounded(choices, k, 48, func(tup []byte) {
				cp := append([]byte(nil), tup...)
				s.starGens(planeFlat, scale, 0, cp, true, consider)
			})
		}
	}
}

func (s *planeSearch) starGens(kind, a, b int, mults []byte, lock bool, consider func(*stackGen)) {
	k := len(mults)
	if k == 0 || k > 5 {
		return
	}
	eachCombo(dimCap, k, func(axes []int) {
		if !axesHigh(axes) {
			return
		}
		offs := make([][dimCap]int, k)
		for i, ax := range axes {
			offs[i][ax] = 1
		}
		if kind == planeFlat {
			for _, bld := range scaleVariants(a) {
				consider(&stackGen{
					text: s.text, kind: kind, a: a, b: b, mults: mults,
					offs: offs, counter: ptDim(bld.at), scale: bld.code, lockAssign: lock,
				})
			}
			return
		}
		for _, ax := range []int{0, 1} {
			var counter [dimCap]int
			counter[ax] = 1
			consider(&stackGen{
				text: s.text, kind: kind, a: a, b: b, mults: mults,
				offs: offs, counter: counter, lockAssign: lock,
			})
		}
	})
}

func (s *planeSearch) armGens(kind, a, b int, mults []byte, consider func(*stackGen)) {
	limit := 255
	scale := a
	if kind == planeNest {
		if b < 2 {
			return
		}
		scale = a * b
	}
	if scale <= 0 {
		return
	}
	limit /= scale
	if limit > 8 {
		limit = 8
	}
	for extra := 1; extra <= limit; extra++ {
		grown := append(append([]byte(nil), mults...), byte(extra))
		for arm := 4; arm < dimCap; arm++ {
			offs := append(append([][dimCap]int(nil), squareOff...), [dimCap]int{})
			offs[len(offs)-1][arm] = 1
			if kind == planeFlat {
				for _, bld := range scaleVariants(a) {
					consider(&stackGen{
						text: s.text, kind: kind, a: a, mults: grown,
						offs: offs, counter: ptDim(bld.at), scale: bld.code,
					})
				}
				continue
			}
			for _, ax := range []int{0, 1} {
				var counter [dimCap]int
				counter[ax] = 1
				consider(&stackGen{
					text: s.text, kind: kind, a: a, b: b, mults: grown,
					offs: offs, counter: counter,
				})
			}
		}
	}
}

func (g *stackGen) high() bool {
	for _, o := range g.offs {
		for ax := 4; ax < dimCap; ax++ {
			if o[ax] != 0 {
				return true
			}
		}
	}
	return false
}

func (g *stackGen) prepare() bool {
	if g.a < 1 || len(g.mults) == 0 || len(g.mults) != len(g.offs) || len(g.mults) > 5 {
		return false
	}
	if g.kind == planeNest && g.b < 2 {
		return false
	}
	scale := g.a
	if g.kind == planeNest {
		scale = g.a * g.b
	}
	for _, m := range g.mults {
		if m == 0 || scale*int(m) > 255 {
			return false
		}
	}
	seen := make(map[[dimCap]int]bool, len(g.offs)+1)
	seen[g.counter] = true
	if g.kind == planeNest {
		var origin [dimCap]int
		if g.counter == origin {
			return false
		}
		seen[origin] = true
	}
	pts := make([][dimCap]int, len(g.offs))
	for i, o := range g.offs {
		p := g.counter
		for ax := range p {
			p[ax] += o[ax]
		}
		if seen[p] {
			return false
		}
		seen[p] = true
		pts[i] = p
	}
	g.pts = pts
	return true
}

func (g *stackGen) best() (assign, ord []int, cost int) {
	if !g.prepare() {
		return nil, nil, math.MaxInt
	}
	// The loop restores the active axis, so the print head does not depend
	// on the visit order. The order only changes how long the setup is.
	k := len(g.mults)
	bestInit := math.MaxInt
	permute(k, func(tour []int) {
		n := g.initLen(tour)
		if n < bestInit {
			bestInit = n
			ord = append([]int(nil), tour...)
		}
	})
	cost = math.MaxInt
	try := func(as []int) {
		_, c := g.eval(as, ord, false)
		if c < cost {
			cost = c
			assign = append([]int(nil), as...)
		}
	}
	if g.lockAssign {
		try(identity(k))
		return assign, ord, cost
	}
	permute(k, try)
	return assign, ord, cost
}

func (g *stackGen) initLen(tour []int) int {
	g.initOnly = true
	_, n := g.eval(identity(len(g.mults)), tour, false)
	g.initOnly = false
	return n
}

func (g *stackGen) eval(assign, tour []int, build bool) (string, int) {
	var b strings.Builder
	m := mover{active: 0}
	if build {
		m.b = &b
	}
	home := g.counter
	if g.kind == planeFlat {
		m.str(g.scale)
		m.pos = g.counter
		m.active = 0
		m.byte('[')
		g.visit(&m, assign, tour, home)
		// Each pass of the loop has to start with '>' on axis 0.
		m.aim(0)
		m.str("-]")
	} else {
		var origin [dimCap]int
		m.str(strings.Repeat("+", g.a))
		m.byte('[')
		m.goTo(g.counter)
		m.str(strings.Repeat("+", g.b))
		entry := m.active
		m.byte('[')
		g.visit(&m, assign, tour, home)
		m.aim(entry)
		m.str("-]")
		m.goTo(origin)
		m.aim(0)
		m.str("-]")
	}
	if g.initOnly {
		return "", m.n
	}
	vals := make([]byte, len(g.mults))
	scale := g.a
	if g.kind == planeNest {
		scale = g.a * g.b
	}
	for i := range vals {
		vals[i] = byte(scale * int(g.mults[assign[i]]))
	}
	for i := 0; i < len(g.text); i++ {
		ch := g.text[i]
		bestJ := 0
		bestC := math.MaxInt
		for j := range g.pts {
			c, _, _ := moveCost(m.pos, m.active, g.pts[j])
			c += len(rawCode(vals[j], ch)) + 1
			if c < bestC {
				bestC = c
				bestJ = j
			}
		}
		m.goTo(g.pts[bestJ])
		m.str(rawCode(vals[bestJ], ch))
		m.byte('.')
		vals[bestJ] = ch
	}
	return b.String(), m.n
}

func (g *stackGen) visit(m *mover, assign, tour []int, home [dimCap]int) {
	for _, i := range tour {
		m.goTo(g.pts[i])
		m.str(strings.Repeat("+", int(g.mults[assign[i]])))
	}
	m.goTo(home)
}

type mover struct {
	b      *strings.Builder
	pos    [dimCap]int
	active int
	n      int
}

func (m *mover) byte(c byte) {
	if m.b != nil {
		m.b.WriteByte(c)
	}
	m.n++
}

func (m *mover) str(s string) {
	if m.b != nil {
		m.b.WriteString(s)
	}
	m.n += len(s)
}

// aim makes '>' and '<' step axis. The head does not move.
func (m *mover) aim(axis int) {
	if m.active == axis {
		return
	}
	m.str(selectCode(axis))
	m.active = axis
}

func (m *mover) goTo(dst [dimCap]int) {
	if m.b == nil {
		c, p, a := moveCost(m.pos, m.active, dst)
		m.n += c
		m.pos = p
		m.active = a
		return
	}
	s, p, a := emitMove(m.pos, m.active, dst)
	m.b.WriteString(s)
	m.n += len(s)
	m.pos = p
	m.active = a
}

func moveCost(pos [dimCap]int, active int, dst [dimCap]int) (int, [dimCap]int, int) {
	n := 0
	for _, ax := range []int{1, 2, 3} {
		d := dst[ax] - pos[ax]
		if d < 0 {
			d = -d
		}
		n += d
		pos[ax] = dst[ax]
	}
	var rest [dimCap]int
	rn := 0
	for ax := 0; ax < dimCap; ax++ {
		if ax == 1 || ax == 2 || ax == 3 || pos[ax] == dst[ax] {
			continue
		}
		rest[rn] = ax
		rn++
	}
	for i := 1; i < rn; i++ {
		if rest[i] == active {
			rest[0], rest[i] = rest[i], rest[0]
			break
		}
	}
	for i := 0; i < rn; i++ {
		ax := rest[i]
		if active != ax {
			n += len(selectCode(ax))
			active = ax
		}
		d := dst[ax] - pos[ax]
		if d < 0 {
			d = -d
		}
		n += d
		pos[ax] = dst[ax]
	}
	return n, pos, active
}

func emitMove(pos [dimCap]int, active int, dst [dimCap]int) (string, [dimCap]int, int) {
	var b strings.Builder
	for _, ax := range []int{1, 2, 3} {
		d := dst[ax] - pos[ax]
		if d == 0 {
			continue
		}
		ch := axisPair[ax][0]
		if d < 0 {
			ch = axisPair[ax][1]
			d = -d
		}
		for i := 0; i < d; i++ {
			b.WriteByte(ch)
		}
		pos[ax] = dst[ax]
	}
	var rest [dimCap]int
	rn := 0
	for ax := 0; ax < dimCap; ax++ {
		if ax == 1 || ax == 2 || ax == 3 || pos[ax] == dst[ax] {
			continue
		}
		rest[rn] = ax
		rn++
	}
	for i := 1; i < rn; i++ {
		if rest[i] == active {
			rest[0], rest[i] = rest[i], rest[0]
			break
		}
	}
	for i := 0; i < rn; i++ {
		ax := rest[i]
		if active != ax {
			b.WriteString(selectCode(ax))
			active = ax
		}
		d := dst[ax] - pos[ax]
		ch := byte('>')
		if d < 0 {
			ch = '<'
			d = -d
		}
		for i := 0; i < d; i++ {
			b.WriteByte(ch)
		}
		pos[ax] = dst[ax]
	}
	return b.String(), pos, active
}

func makeSelect(axis int) string {
	best := "*" + strings.Repeat("+", axis) + "."
	for q := 1; q <= 36; q++ {
		for f := 1; f <= 36; f++ {
			prod := q * f
			if prod > 255 {
				continue
			}
			rem := axis - prod
			if rem > 12 || rem < -12 {
				continue
			}
			var b strings.Builder
			b.WriteByte('*')
			b.WriteString(strings.Repeat("+", q))
			b.WriteString("[>")
			b.WriteString(strings.Repeat("+", f))
			b.WriteString("<-]>")
			if rem > 0 {
				b.WriteString(strings.Repeat("+", rem))
			} else if rem < 0 {
				b.WriteString(strings.Repeat("-", -rem))
			}
			b.WriteByte('.')
			if b.Len() < len(best) {
				best = b.String()
			}
		}
	}
	return best
}

func selectCode(axis int) string {
	if axis >= 0 && axis < len(selCache) {
		return selCache[axis]
	}
	return makeSelect(axis)
}

func axesHigh(axes []int) bool {
	for _, ax := range axes {
		if ax >= 4 {
			return true
		}
	}
	return false
}

func ptDim(p pt) [dimCap]int {
	var h [dimCap]int
	for i := 0; i < planeMaxD && i < dimCap; i++ {
		h[i] = p[i]
	}
	return h
}

func identity(n int) []int {
	p := make([]int, n)
	for i := range p {
		p[i] = i
	}
	return p
}

func eachCombo(n, k int, fn func([]int)) {
	if k == 0 || k > n {
		return
	}
	idx := make([]int, k)
	var rec func(int, int)
	rec = func(d, start int) {
		if d == k {
			cp := append([]int(nil), idx...)
			fn(cp)
			return
		}
		for a := start; a < n; a++ {
			idx[d] = a
			rec(d+1, a+1)
		}
	}
	rec(0, 0)
}

func permute(n int, fn func([]int)) {
	if n == 0 {
		fn(nil)
		return
	}
	p := make([]int, n)
	for i := range p {
		p[i] = i
	}
	var rec func(int)
	rec = func(d int) {
		if d == n {
			fn(p)
			return
		}
		for i := d; i < n; i++ {
			p[d], p[i] = p[i], p[d]
			rec(d + 1)
			p[d], p[i] = p[i], p[d]
		}
	}
	rec(0)
}
