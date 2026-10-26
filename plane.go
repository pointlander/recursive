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

// Plane searches for a short program that prints text on the pack machine.
//
// Every byte of the program is a one-byte axis step, '*', or one of +-.[].
// The search does not emit literal bytes, '~' escapes, raw blocks, a
// bit-packed payload, or an '@' operand. It plants a few anchor cells
// with a flat or nested multiply loop, lays those cells out across the
// axes the machine can step in one byte, and walks a print path of moves,
// adds, and dots. A second pass plants anchors on later axes, selected
// with the dimension stack. The result is the shortest program found in
// this family, not a proof of the shortest program the machine can run.

const (
	planeNest = iota
	planeFlat
)

const (
	planeMaxK     = 5
	planeMaxScale = 36
	planeMaxProd  = 48
	planePool     = 24
	planePowCap   = 80000
	planeSlack    = 12
	planeMaxD     = axisCount
)

// planeInner is the counter for a nested multiply loop. The outer counter
// stays at the origin. Data cells are placed on neither.
var planeInner = pt{1, 0}

// pt is a coordinate on the one-byte axes. Axis 0 is >< and axis 1 is v^.
// A zero axis is unused. The pack machine can address further axes with
// '@'; this search stays on the one-byte axes, where a step costs one byte.
type pt [planeMaxD]int

type loopPlan struct {
	cost uint8
	raw  bool
	neg  bool
	q, f uint8
	r    int16
}

var (
	planOnce sync.Once
	plans    [9][256][256]loopPlan
)

type planeCand struct {
	est  int
	kind int
	a, b int
	k    int
	at   pt
	mult [6]byte
	pts  [6]pt
}

// scaleBuild stores scale in at and leaves every other cell 0.
type scaleBuild struct {
	code string
	at   pt
}

type layoutKey struct {
	kind int
	k    int
	at   pt
}

type planeSearch struct {
	text     string
	alpha    []byte
	bestLen  int
	bestCode string
	bestSeen int
	pool     []planeCand
	layouts  map[layoutKey][][]pt
}

// Plane returns a command-only pack-machine program that prints text.
func Plane(text string) (string, error) {
	if text == "" {
		return "", nil
	}
	if len(text) > 80 {
		return "", fmt.Errorf("space: text longer than 80 bytes")
	}
	preparePlans()
	s := &planeSearch{text: text, alpha: alphabet(text)}
	base := naive(text)
	if err := verifyPlane(text, base); err != nil {
		return "", err
	}
	s.bestCode = base
	s.bestLen = len(base)
	s.bestSeen = len(base)
	s.seed()
	s.searchNest()
	s.searchFlat()
	s.finish()
	if err := s.searchStack(); err != nil {
		return "", err
	}
	if err := verifyPlane(text, s.bestCode); err != nil {
		return "", err
	}
	return s.bestCode, nil
}

func (s *planeSearch) seed() {
	s.eval(planeNest, 4, 4, []byte{6, 7, 7, 2})
}

func (s *planeSearch) searchNest() {
	for prod := 2; prod <= planeMaxProd; prod++ {
		a, b := bestFactors(prod)
		if b == 1 {
			continue
		}
		choices := append([]byte(nil), multChoices(prod, s.alpha)...)
		sort.Slice(choices, func(i, j int) bool { return choices[i] < choices[j] })
		for k := 1; k <= planeMaxK; k++ {
			if powOver(len(choices), k, planePowCap) {
				continue
			}
			maxSum := s.bestSeen + planeSlack - len(s.text) - a - b - k
			eachBounded(choices, k, maxSum, func(tup []byte) {
				s.eval(planeNest, a, b, tup)
			})
		}
	}
	fmt.Fprintf(os.Stderr, "space nest best=%d pool=%d\n", s.bestSeen, len(s.pool))
}

func (s *planeSearch) searchFlat() {
	for scale := 2; scale <= planeMaxScale; scale++ {
		choices := append([]byte(nil), multChoices(scale, s.alpha)...)
		sort.Slice(choices, func(i, j int) bool { return choices[i] < choices[j] })
		for k := 1; k <= planeMaxK; k++ {
			if powOver(len(choices), k, planePowCap) {
				continue
			}
			maxSum := s.bestSeen + planeSlack - len(s.text) - scale - k
			eachBounded(choices, k, maxSum, func(tup []byte) {
				s.eval(planeFlat, scale, 0, tup)
			})
		}
	}
	fmt.Fprintf(os.Stderr, "space flat best=%d pool=%d\n", s.bestSeen, len(s.pool))
}

func (s *planeSearch) eval(kind, a, b int, mults []byte) {
	k := len(mults)
	if k == 0 || k > planeMaxK {
		return
	}
	copied := append([]byte(nil), mults...)
	builds := []scaleBuild{{at: pt{}}}
	if kind == planeFlat {
		builds = scaleVariants(a)
	}
	for _, built := range builds {
		for _, pts := range s.layoutSet(kind, k, built.at) {
			s.consider(kind, a, b, copied, pts, built.at)
		}
	}
}

func (s *planeSearch) layoutSet(kind, k int, counter pt) [][]pt {
	if s.layouts == nil {
		s.layouts = make(map[layoutKey][][]pt)
	}
	key := layoutKey{kind: kind, k: k, at: counter}
	if pts, ok := s.layouts[key]; ok {
		return pts
	}
	pts := layouts(kind, k, counter)
	s.layouts[key] = pts
	return pts
}

func (s *planeSearch) consider(kind, a, b int, mults []byte, pts []pt, at pt) {
	sum := 0
	scale := a
	if kind == planeNest {
		scale = a * b
	}
	for _, m := range mults {
		if scale*int(m) > 255 {
			return
		}
		sum += int(m)
	}
	prefix := a
	if kind == planeFlat {
		prefix = len(scaleCode(a, at))
	}
	lb := sum + prefix + len(mults)
	if kind == planeNest {
		lb = sum + a + b + len(mults)
	}
	if lb+len(s.text) >= s.bestSeen+planeSlack {
		return
	}
	init := initCode(kind, a, b, mults, pts, at)
	vals := anchorVals(kind, a, b, mults)
	est := len(init) + printCost(s.text, vals, pts, at)
	if est >= s.bestSeen+planeSlack {
		return
	}
	c := planeCand{est: est, kind: kind, a: a, b: b, k: len(mults), at: at}
	copy(c.mult[:], mults)
	copy(c.pts[:], pts)
	s.add(c)
}

func (s *planeSearch) add(c planeCand) {
	if c.est < s.bestSeen {
		s.bestSeen = c.est
		fmt.Fprintf(os.Stderr, "space est %d kind=%d K=%d a=%d b=%d\n", c.est, c.kind, c.k, c.a, c.b)
	}
	s.pool = append(s.pool, c)
	sort.SliceStable(s.pool, func(i, j int) bool { return s.pool[i].est < s.pool[j].est })
	cut := s.bestSeen + planeSlack
	n := 0
	for n < len(s.pool) && s.pool[n].est < cut {
		n++
	}
	s.pool = s.pool[:n]
	if len(s.pool) > planePool {
		s.pool = s.pool[:planePool]
	}
}

func (s *planeSearch) finish() {
	sort.SliceStable(s.pool, func(i, j int) bool { return s.pool[i].est < s.pool[j].est })
	for i := range s.pool {
		c := s.pool[i]
		pts := climb(c, s.text)
		code := s.program(c, pts, c.k <= 4)
		if code == "" {
			continue
		}
		if len(code) < s.bestLen || (len(code) == s.bestLen && spaceRank(code) > spaceRank(s.bestCode)) {
			s.bestLen = len(code)
			s.bestCode = code
			fmt.Fprintf(os.Stderr, "space %d bytes\n", len(code))
		}
	}
}

func (s *planeSearch) program(c planeCand, pts []pt, useDP bool) string {
	mults := append([]byte(nil), c.mult[:c.k]...)
	init := initCode(c.kind, c.a, c.b, mults, pts, c.at)
	vals := anchorVals(c.kind, c.a, c.b, mults)
	body := greedyProgram(s.text, vals, pts, c.at)
	if useDP {
		if alt, ok := optimizePlane(s.text, vals, pts, c.at, len(body)); ok && len(alt) < len(body) {
			body = alt
		}
	}
	code := squeeze(init + body)
	if err := verifyPlane(s.text, code); err != nil {
		fmt.Fprintf(os.Stderr, "space reject %v\n", err)
		return ""
	}
	return code
}

func verifyPlane(text, code string) error {
	if !commandOnly(code) {
		return fmt.Errorf("program contains a non-command byte")
	}
	st, err := ExecutePack(code, "", 2_000_000)
	if err != nil {
		return err
	}
	if st.Out != text {
		return fmt.Errorf("output %q", st.Out)
	}
	return nil
}

func commandOnly(code string) bool {
	for i := 0; i < len(code); i++ {
		if _, _, ok := axisStep(code[i]); ok {
			continue
		}
		switch code[i] {
		case '*', '+', '-', '.', ',', '[', ']':
		default:
			return false
		}
	}
	return true
}

// spaceRank is one more than the highest axis the program steps on.
// A program that never moves has rank 0. Equal-length programs prefer
// a higher rank so an n-dimensional layout wins a tie with a flat one.
func spaceRank(code string) int {
	rank := 0
	for i := 0; i < len(code); i++ {
		dim, _, ok := axisStep(code[i])
		if ok && dim+1 > rank {
			rank = dim + 1
		}
	}
	return rank
}

func layouts(kind, k int, counter pt) [][]pt {
	avoid := []pt{counter}
	if kind == planeNest {
		avoid = append(avoid, planeInner)
	}
	raw := [][]pt{
		lineAvoid(k, avoid),
		snakeAvoid(k, avoid),
	}
	// The line and the snake cover the first two axes. Each higher
	// dimensionality adds a compact grid and a star.
	for d := 3; d <= planeMaxD; d++ {
		raw = append(raw, gridFill(k, d, avoid), starFill(k, d, avoid))
	}
	out := make([][]pt, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	for _, pts := range raw {
		if len(pts) != k {
			continue
		}
		key := fmt.Sprint(pts)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, pts)
	}
	return out
}

func lineAvoid(k int, avoid []pt) []pt {
	pts := make([]pt, 0, k)
	for x := 1; len(pts) < k; x++ {
		p := pt{x, 0}
		if occupied(avoid, p) {
			continue
		}
		pts = append(pts, p)
	}
	return pts
}

func snakeAvoid(k int, avoid []pt) []pt {
	order := []pt{
		{1, 0}, {2, 0}, {2, 1}, {1, 1},
		{0, 1}, {0, 2}, {1, 2}, {2, 2}, {3, 1}, {3, 0},
		{3, 2}, {4, 0}, {4, 1}, {3, -1}, {2, -1}, {1, -1},
		{4, 2}, {5, 0}, {5, 1}, {-1, 0}, {-1, 1},
	}
	pts := make([]pt, 0, k)
	for _, p := range order {
		if occupied(avoid, p) {
			continue
		}
		pts = append(pts, p)
		if len(pts) == k {
			return pts
		}
	}
	return pts
}

func linePts(k, startX int) []pt {
	pts := make([]pt, k)
	for i := 0; i < k; i++ {
		pts[i] = pt{startX + i, 0}
	}
	return pts
}

func snakePts(k int, blockInner bool) []pt {
	var avoid []pt
	if blockInner {
		avoid = []pt{planeInner}
	}
	return snakeAvoid(k, avoid)
}

// gridFill takes the k free cells closest to the origin inside the first
// dims axes. Coordinates run from -1 to 2.
func gridFill(k, dims int, avoid []pt) []pt {
	if dims > planeMaxD {
		dims = planeMaxD
	}
	type cand struct {
		p pt
		m int
	}
	var cs []cand
	var rec func(d int, cur pt, man int)
	rec = func(d int, cur pt, man int) {
		if d == dims {
			cs = append(cs, cand{cur, man})
			return
		}
		for v := -1; v <= 2; v++ {
			cur[d] = v
			rec(d+1, cur, man+abs(v))
		}
	}
	rec(0, pt{}, 0)
	sort.Slice(cs, func(i, j int) bool {
		if cs[i].m != cs[j].m {
			return cs[i].m < cs[j].m
		}
		for t := 0; t < planeMaxD; t++ {
			if cs[i].p[t] != cs[j].p[t] {
				return cs[i].p[t] < cs[j].p[t]
			}
		}
		return false
	})
	pts := make([]pt, 0, k)
	for _, c := range cs {
		if occupied(avoid, c.p) || occupied(pts, c.p) {
			continue
		}
		pts = append(pts, c.p)
		if len(pts) == k {
			break
		}
	}
	return pts
}

// starFill puts anchors on distinct axes, one step out, then further
// steps, then fills the rest from the grid.
func starFill(k, dims int, avoid []pt) []pt {
	if dims > planeMaxD {
		dims = planeMaxD
	}
	pts := make([]pt, 0, k)
	for step := 1; step <= 3 && len(pts) < k; step++ {
		for axis := 0; axis < dims && len(pts) < k; axis++ {
			for _, sign := range []int{1, -1} {
				var p pt
				p[axis] = sign * step
				if occupied(avoid, p) || occupied(pts, p) {
					continue
				}
				pts = append(pts, p)
				break
			}
		}
	}
	if len(pts) < k {
		more := gridFill(k-len(pts), dims, append(append([]pt{}, avoid...), pts...))
		pts = append(pts, more...)
	}
	if len(pts) > k {
		pts = pts[:k]
	}
	return pts
}

func anchorVals(kind, a, b int, mults []byte) (v [6]byte) {
	scale := a
	if kind == planeNest {
		scale = a * b
	}
	for i, m := range mults {
		v[i] = byte(scale * int(m))
	}
	return v
}

func initCode(kind, a, b int, mults []byte, pts []pt, at pt) string {
	if kind == planeFlat {
		return flatCode(a, mults, pts, at)
	}
	return nest2D(a, b, mults, planeInner, pts)
}

func flatCode(scale int, mults []byte, pts []pt, at pt) string {
	ord := bestTour(at, pts)
	var b strings.Builder
	b.WriteString(scaleCode(scale, at))
	b.WriteByte('[')
	cur := at
	for _, i := range ord {
		b.WriteString(move(cur, pts[i]))
		b.WriteString(strings.Repeat("+", int(mults[i])))
		cur = pts[i]
	}
	b.WriteString(move(cur, at))
	b.WriteString("-]")
	return b.String()
}

// scaleVariants returns the shortest ways to store n in a cell.
// A multiply loop can plant a large scale in fewer bytes than n pluses,
// and then the loop counter sits on that cell.
func scaleVariants(n int) []scaleBuild {
	best := n
	if n == 0 {
		return []scaleBuild{{at: pt{}}}
	}
	var out []scaleBuild
	add := func(code string, at pt) {
		if len(code) > best {
			return
		}
		if len(code) < best {
			best = len(code)
			out = out[:0]
		}
		for _, e := range out {
			if e.at == at {
				return
			}
		}
		out = append(out, scaleBuild{code: code, at: at})
	}
	add(strings.Repeat("+", n), pt{})
	type dir struct {
		at    pt
		forth string
		back  string
		after string
	}
	// The scale counter sits on axis 0 or axis 1. Both loops have the same
	// length, and a later axis does not make the counter shorter.
	dirs := []dir{
		{pt{1, 0}, ">", "<", ">"},
		{pt{0, 1}, "v", "^", "v"},
	}
	for q := 1; q < n; q++ {
		base := n / q
		for _, f := range []int{base, base + 1} {
			if f <= 0 {
				continue
			}
			rem := n - q*f
			if rem > f || rem < -f {
				continue
			}
			for _, d := range dirs {
				code := strings.Repeat("+", q) + "[" + d.forth + strings.Repeat("+", f) + d.back + "-]" + d.after
				if rem > 0 {
					code += strings.Repeat("+", rem)
				} else if rem < 0 {
					code += strings.Repeat("-", -rem)
				}
				add(code, d.at)
			}
		}
	}
	return out
}

func scaleCode(n int, at pt) string {
	for _, b := range scaleVariants(n) {
		if b.at == at {
			return b.code
		}
	}
	return strings.Repeat("+", n)
}

func nest2D(a, b int, mults []byte, inner pt, pts []pt) string {
	ord := bestTour(inner, pts)
	var body strings.Builder
	cur := inner
	for _, i := range ord {
		body.WriteString(move(cur, pts[i]))
		body.WriteString(strings.Repeat("+", int(mults[i])))
		cur = pts[i]
	}
	body.WriteString(move(cur, inner))
	body.WriteByte('-')

	var s strings.Builder
	s.WriteString(strings.Repeat("+", a))
	s.WriteByte('[')
	s.WriteString(move(pt{}, inner))
	s.WriteString(strings.Repeat("+", b))
	s.WriteByte('[')
	s.WriteString(body.String())
	s.WriteByte(']')
	s.WriteString(move(inner, pt{}))
	s.WriteString("-]")
	return s.String()
}

func bestTour(start pt, pts []pt) []int {
	k := len(pts)
	if k == 0 {
		return nil
	}
	idx := make([]int, k)
	for i := range idx {
		idx[i] = i
	}
	best := math.MaxInt
	bestOrd := make([]int, k)
	var rec func(int)
	rec = func(d int) {
		if d == k {
			m := 0
			cur := start
			for _, i := range idx {
				m += manh(cur, pts[i])
				cur = pts[i]
			}
			m += manh(cur, start)
			if m < best {
				best = m
				copy(bestOrd, idx)
			}
			return
		}
		for i := d; i < k; i++ {
			idx[d], idx[i] = idx[i], idx[d]
			rec(d + 1)
			idx[d], idx[i] = idx[i], idx[d]
		}
	}
	rec(0)
	return bestOrd
}

func move(from, to pt) string {
	var b strings.Builder
	for d := 0; d < planeMaxD; d++ {
		delta := to[d] - from[d]
		if delta == 0 {
			continue
		}
		ch := axisPair[d][0]
		if delta < 0 {
			ch = axisPair[d][1]
			delta = -delta
		}
		for i := 0; i < delta; i++ {
			b.WriteByte(ch)
		}
	}
	return b.String()
}

func manh(a, b pt) int {
	n := 0
	for d := 0; d < planeMaxD; d++ {
		n += abs(a[d] - b[d])
	}
	return n
}

func printCost(text string, vals [6]byte, pts []pt, start pt) int {
	dist := anchorDist(pts)
	cur := vals
	pos := start
	total := 0
	for i := 0; i < len(text); i++ {
		j, c := pickAnchor(pos, cur, text[i], pts, dist)
		total += c
		cur[j] = text[i]
		pos = pts[j]
	}
	return total
}

func greedyProgram(text string, vals [6]byte, pts []pt, start pt) string {
	dist := anchorDist(pts)
	cur := vals
	pos := start
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		ch := text[i]
		j, _ := pickAnchor(pos, cur, ch, pts, dist)
		b.WriteString(move(pos, pts[j]))
		b.WriteString(emitAdjust(cur[j], ch, pts[j], pts))
		b.WriteByte('.')
		cur[j] = ch
		pos = pts[j]
	}
	return b.String()
}

func anchorDist(pts []pt) []int {
	dist := make([]int, len(pts))
	for i := range pts {
		_, dist[i] = nearestZero(pts[i], pts)
		if dist[i] > 8 {
			dist[i] = 8
		}
	}
	return dist
}

func pickAnchor(pos pt, cur [6]byte, ch byte, pts []pt, dist []int) (int, int) {
	best := math.MaxInt
	bestJ := 0
	for j := range pts {
		c := manh(pos, pts[j]) + int(plans[dist[j]][cur[j]][ch].cost) + 1
		if c < best {
			best = c
			bestJ = j
		}
	}
	return bestJ, best
}

func emitAdjust(from, to byte, at pt, anchors []pt) string {
	if from == to {
		return ""
	}
	best := plans[0][from][to]
	z, d := nearestZero(at, anchors)
	if d > 0 && d < 9 {
		p := plans[d][from][to]
		if !p.raw && p.cost < best.cost {
			return loopCode(p, at, z)
		}
	}
	return rawCode(from, to)
}

func rawCode(from, to byte) string {
	p := plans[0][from][to]
	if p.cost == 0 {
		return ""
	}
	ch := "+"
	if p.neg {
		ch = "-"
	}
	return strings.Repeat(ch, int(p.cost))
}

func loopCode(p loopPlan, at, z pt) string {
	var b strings.Builder
	b.WriteString(move(at, z))
	b.WriteString(strings.Repeat("+", int(p.q)))
	b.WriteString("[-")
	b.WriteString(move(z, at))
	body := "+"
	if p.neg {
		body = "-"
	}
	b.WriteString(strings.Repeat(body, int(p.f)))
	b.WriteString(move(at, z))
	b.WriteByte(']')
	b.WriteString(move(z, at))
	if p.r > 0 {
		b.WriteString(strings.Repeat("+", int(p.r)))
	} else if p.r < 0 {
		b.WriteString(strings.Repeat("-", int(-p.r)))
	}
	return b.String()
}

func nearestZero(at pt, anchors []pt) (pt, int) {
	for rad := 1; rad <= 8; rad++ {
		if z, ok := firstAtRadius(at, rad, anchors); ok {
			return z, rad
		}
	}
	z := at
	z[0]++
	return z, 1
}

// firstAtRadius returns the first free cell at L1 distance rad.
// Axis 1 is scanned from negative to positive, and higher axes stay at
// 0 until that plane is exhausted, so a free neighbor on the first two
// axes is chosen before a neighbor on a later axis.
func firstAtRadius(at pt, rad int, anchors []pt) (pt, bool) {
	cur := at
	var rec func(dim, left int) (pt, bool)
	rec = func(dim, left int) (pt, bool) {
		if dim == 0 {
			if left == 0 {
				if !occupied(anchors, cur) {
					return cur, true
				}
				return pt{}, false
			}
			cur[0] = at[0] + left
			if !occupied(anchors, cur) {
				return cur, true
			}
			cur[0] = at[0] - left
			if !occupied(anchors, cur) {
				return cur, true
			}
			cur[0] = at[0]
			return pt{}, false
		}
		for _, v := range axisOffsets(dim, left) {
			cur[dim] = at[dim] + v
			if z, ok := rec(nextAxis(dim), left-abs(v)); ok {
				return z, true
			}
		}
		cur[dim] = at[dim]
		return pt{}, false
	}
	return rec(1, rad)
}

func nextAxis(dim int) int {
	if dim == 1 {
		if planeMaxD > 2 {
			return 2
		}
		return 0
	}
	if dim+1 < planeMaxD {
		return dim + 1
	}
	return 0
}

func axisOffsets(dim, left int) []int {
	if dim == 1 {
		out := make([]int, 0, left*2+1)
		for v := -left; v <= left; v++ {
			out = append(out, v)
		}
		return out
	}
	out := make([]int, 0, left*2+1)
	out = append(out, 0)
	for v := 1; v <= left; v++ {
		out = append(out, v, -v)
	}
	return out
}

func occupied(pts []pt, p pt) bool {
	for _, q := range pts {
		if q == p {
			return true
		}
	}
	return false
}

func occupiedOther(pts []pt, skip int, p pt) bool {
	for i, q := range pts {
		if i != skip && q == p {
			return true
		}
	}
	return false
}

func climb(c planeCand, text string) []pt {
	pts := append([]pt(nil), c.pts[:c.k]...)
	mults := append([]byte(nil), c.mult[:c.k]...)
	blocked := []pt{c.at}
	if c.kind == planeNest {
		blocked = append(blocked, pt{}, planeInner)
	}
	score := func(p []pt) int {
		return len(initCode(c.kind, c.a, c.b, mults, p, c.at)) + printCost(text, anchorVals(c.kind, c.a, c.b, mults), p, c.at)
	}
	bestS := score(pts)
	dirs := climbDirs()
	for sweep := 0; sweep < 4; sweep++ {
		improved := false
		for i := range pts {
			base := pts[i]
			bestPt := base
			local := bestS
			for _, d := range dirs {
				np := base
				for a := range d {
					np[a] += d[a]
				}
				if !inClimbBox(np) {
					continue
				}
				if occupied(blocked, np) || occupiedOther(pts, i, np) {
					continue
				}
				pts[i] = np
				if s := score(pts); s < local {
					local = s
					bestPt = np
				}
			}
			pts[i] = bestPt
			if local < bestS {
				bestS = local
				improved = true
			}
		}
		if !improved {
			break
		}
	}
	return pts
}

func climbDirs() []pt {
	dirs := []pt{
		{1, 0}, {-1, 0}, {0, 1}, {0, -1},
		{2, 0}, {-2, 0}, {0, 2}, {0, -2},
		{1, 1}, {-1, 1}, {1, -1}, {-1, -1},
	}
	for d := 2; d < planeMaxD; d++ {
		for _, s := range []int{1, -1, 2, -2} {
			var step pt
			step[d] = s
			dirs = append(dirs, step)
		}
	}
	return dirs
}

func inClimbBox(p pt) bool {
	if p[0] < -2 || p[0] > 6 || p[1] < -2 || p[1] > 3 {
		return false
	}
	for d := 2; d < planeMaxD; d++ {
		if p[d] < -2 || p[d] > 2 {
			return false
		}
	}
	return true
}

func optimizePlane(text string, vals [6]byte, pts []pt, start pt, incumbent int) (string, bool) {
	k := len(pts)
	if k == 0 || k > 4 || incumbent <= len(text) {
		return "", false
	}
	dist := anchorDist(pts)
	type key struct {
		i, at uint8
		v     [6]byte
	}
	type node struct {
		cost     int
		i, at    int
		v        [6]byte
		prev, aj int
	}
	nodes := make([]node, 0, 4096)
	root := node{cost: 0, i: 0, at: k, prev: -1, aj: -1, v: vals}
	nodes = append(nodes, root)
	best := make(map[key]int, 8192)
	sk := key{i: 0, at: uint8(k), v: vals}
	best[sk] = 0
	buckets := make([][]int, incumbent)
	buckets[0] = []int{0}
	goal := -1
	for cost := 0; cost < incumbent && goal < 0; cost++ {
		for _, id := range buckets[cost] {
			n := nodes[id]
			if n.cost != cost {
				continue
			}
			kk := key{i: uint8(n.i), at: uint8(n.at), v: n.v}
			if known, ok := best[kk]; !ok || known != cost {
				continue
			}
			if n.i == len(text) {
				goal = id
				break
			}
			if cost+len(text)-n.i >= incumbent {
				continue
			}
			cur := start
			if n.at < k {
				cur = pts[n.at]
			}
			ch := text[n.i]
			for a := 0; a < k; a++ {
				add := manh(cur, pts[a]) + int(plans[dist[a]][n.v[a]][ch].cost) + 1
				nc := cost + add
				if nc >= incumbent || nc+len(text)-(n.i+1) >= incumbent || nc >= len(buckets) {
					continue
				}
				nv := n.v
				nv[a] = ch
				nk := key{i: uint8(n.i + 1), at: uint8(a), v: nv}
				if old, ok := best[nk]; ok && nc >= old {
					continue
				}
				best[nk] = nc
				nodes = append(nodes, node{cost: nc, i: n.i + 1, at: a, v: nv, prev: id, aj: a})
				buckets[nc] = append(buckets[nc], len(nodes)-1)
				if len(nodes) > 1_500_000 {
					fmt.Fprintf(os.Stderr, "space dp cap\n")
					return "", false
				}
			}
		}
	}
	if goal < 0 {
		return "", false
	}
	seq := make([]int, 0, len(text))
	for id := goal; nodes[id].prev >= 0; id = nodes[id].prev {
		seq = append(seq, nodes[id].aj)
	}
	if len(seq) != len(text) {
		return "", false
	}
	for i, j := 0, len(seq)-1; i < j; i, j = i+1, j-1 {
		seq[i], seq[j] = seq[j], seq[i]
	}
	cur := vals
	pos := start
	var b strings.Builder
	for i, a := range seq {
		ch := text[i]
		b.WriteString(move(pos, pts[a]))
		b.WriteString(emitAdjust(cur[a], ch, pts[a], pts))
		b.WriteByte('.')
		cur[a] = ch
		pos = pts[a]
	}
	if b.Len() != nodes[goal].cost {
		return "", false
	}
	return b.String(), true
}

func preparePlans() {
	planOnce.Do(buildPlans)
}

func buildPlans() {
	try := func(q, f, r, dist int, sub bool, best *loopPlan) {
		if q <= 0 || f <= 0 || q > 255 || f > 255 {
			return
		}
		ar := r
		if ar < 0 {
			ar = -ar
		}
		n := q + f + ar + 4*dist + 3
		if n < int(best.cost) && n < 256 {
			*best = loopPlan{cost: uint8(n), neg: sub, q: uint8(q), f: uint8(f), r: int16(r)}
		}
	}
	consider := func(amount, dist int, sub bool, best *loopPlan) {
		sign := 1
		if sub {
			sign = -1
		}
		for f := 1; f <= amount && f < int(best.cost); f++ {
			q := amount / f
			rem := amount - q*f
			try(q, f, sign*rem, dist, sub, best)
			if rem != 0 {
				try(q+1, f, -sign*(f-rem), dist, sub, best)
			}
		}
	}
	for from := 0; from < 256; from++ {
		for to := 0; to < 256; to++ {
			add := (to - from + 256) % 256
			sub := (from - to + 256) % 256
			raw := add
			rawNeg := false
			if sub < raw {
				raw = sub
				rawNeg = true
			}
			base := loopPlan{cost: uint8(raw), raw: true, neg: rawNeg}
			plans[0][from][to] = base
			for d := 1; d < 9; d++ {
				best := base
				if from != to {
					consider(add, d, false, &best)
					if sub != add {
						consider(sub, d, true, &best)
					}
				}
				plans[d][from][to] = best
			}
		}
	}
}

func eachBounded(choices []byte, k, maxSum int, fn func([]byte)) {
	if k <= 0 || len(choices) == 0 || maxSum < 0 {
		return
	}
	tup := make([]byte, k)
	smallest := int(choices[0])
	var rec func(int, int)
	rec = func(d, sum int) {
		if d == k {
			fn(tup)
			return
		}
		left := k - d - 1
		for _, c := range choices {
			s := sum + int(c)
			if s+smallest*left > maxSum {
				break
			}
			tup[d] = c
			rec(d+1, s)
		}
	}
	rec(0, 0)
}

func powOver(base, exp, cap int) bool {
	if exp <= 0 || base <= 1 {
		return false
	}
	n := 1
	for i := 0; i < exp; i++ {
		if n > cap/base {
			return true
		}
		n *= base
	}
	return n > cap
}
