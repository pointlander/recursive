// Copyright 2026 The Recursive Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "testing"

func TestSelectCode(t *testing.T) {
	for _, axis := range []int{0, 1, 2, 4, 5, 16, 20, 40} {
		code := selectCode(axis) + ">+."
		st, err := ExecutePack(code, "", 200000)
		if err != nil {
			t.Fatalf("axis %d %s: %v", axis, code, err)
		}
		if st.Out != "\x01" {
			t.Fatalf("axis %d out %q code %s", axis, st.Out, code)
		}
		got := 0
		if axis < len(st.Pos) {
			got = st.Pos[axis]
		}
		if axis == 0 {
			got = st.Ptr
		}
		if got != 1 {
			t.Fatalf("axis %d pos %v ptr %d code %s", axis, st.Pos, st.Ptr, code)
		}
		if !commandOnly(selectCode(axis)) {
			t.Fatalf("axis %d select %q", axis, selectCode(axis))
		}
	}
}

func TestStackDeposit(t *testing.T) {
	preparePlans()
	var counter [dimCap]int
	offs := [][dimCap]int{{}}
	offs[0][4] = 1
	g := stackGen{
		text:    "\x03",
		kind:    planeFlat,
		a:       3,
		mults:   []byte{1},
		offs:    offs,
		counter: counter,
		scale:   "+++",
	}
	assign, ord, cost := g.best()
	code, n := g.eval(assign, ord, true)
	if n != cost {
		t.Fatalf("cost %d len %d code %s", cost, n, code)
	}
	if code != "+++[*++++.>+<*.-]*++++.>." {
		t.Fatalf("code %s", code)
	}
	st, err := ExecutePack(code, "", 100000)
	if err != nil {
		t.Fatal(err)
	}
	if st.Out != "\x03" {
		t.Fatalf("out %q", st.Out)
	}
}

func TestStackNestDeposit(t *testing.T) {
	preparePlans()
	var counter [dimCap]int
	counter[0] = 1
	offs := [][dimCap]int{{}}
	offs[0][4] = 1
	g := stackGen{
		text:    "\x04",
		kind:    planeNest,
		a:       2,
		b:       2,
		mults:   []byte{1},
		offs:    offs,
		counter: counter,
	}
	assign, ord, cost := g.best()
	if cost == 0 || assign == nil {
		t.Fatal(cost)
	}
	code, n := g.eval(assign, ord, true)
	if n != cost {
		t.Fatalf("cost %d len %d code %s", cost, n, code)
	}
	st, err := ExecutePack(squeeze(code), "", 200000)
	if err != nil {
		t.Fatalf("%v code %s", err, code)
	}
	if st.Out != "\x04" {
		t.Fatalf("out %q code %s", st.Out, code)
	}
}

func TestStackScaleStar(t *testing.T) {
	preparePlans()
	builds := scaleVariants(16)
	if len(builds) == 0 {
		t.Fatal("no scale")
	}
	for _, bld := range builds {
		offs := [][dimCap]int{{}, {}}
		offs[0][0] = 1
		offs[1][4] = 1
		runStackGen(t, stackGen{
			text:       string([]byte{112, 32}),
			kind:       planeFlat,
			a:          16,
			mults:      []byte{7, 2},
			offs:       offs,
			counter:    ptDim(bld.at),
			scale:      bld.code,
			lockAssign: true,
		})
	}
}

func TestStackSquareArm(t *testing.T) {
	preparePlans()
	var bld scaleBuild
	for _, b := range scaleVariants(16) {
		if b.at == (pt{0, 1}) {
			bld = b
		}
	}
	if bld.code == "" {
		t.Fatal("no counter at (0,1)")
	}
	offs := append(append([][dimCap]int(nil), squareOff...), [dimCap]int{})
	offs[len(offs)-1][4] = 1
	mults := []byte{6, 2, 7, 7, 3}
	vals := make([]byte, len(mults))
	for i, m := range mults {
		vals[i] = byte(16 * int(m))
	}
	runStackGen(t, stackGen{
		text:    string(vals),
		kind:    planeFlat,
		a:       16,
		mults:   mults,
		offs:    offs,
		counter: ptDim(bld.at),
		scale:   bld.code,
	})
}

func runStackGen(t *testing.T, g stackGen) {
	t.Helper()
	assign, ord, cost := g.best()
	code, n := g.eval(assign, ord, true)
	if n != cost {
		t.Fatalf("cost %d len %d code %s", cost, n, code)
	}
	checkOut(t, code, g.text)
	sq := squeeze(code)
	if sq != code {
		checkOut(t, sq, g.text)
	}
}

func checkOut(t *testing.T, code, want string) {
	t.Helper()
	st, err := ExecutePack(code, "", 2_000_000)
	if err != nil {
		t.Fatalf("%v code %s", err, code)
	}
	if st.Out != want {
		t.Fatalf("out %q want %q code %s", st.Out, want, code)
	}
}

func TestStackTwoAxes(t *testing.T) {
	preparePlans()
	var counter [dimCap]int
	offs := [][dimCap]int{{}, {}}
	offs[0][4] = 1
	offs[1][5] = 1
	g := stackGen{
		text:       "\x02\x02",
		kind:       planeFlat,
		a:          2,
		mults:      []byte{1, 1},
		offs:       offs,
		counter:    counter,
		scale:      "++",
		lockAssign: true,
	}
	assign, ord, cost := g.best()
	code, n := g.eval(assign, ord, true)
	if n != cost {
		t.Fatalf("cost %d len %d code %s", cost, n, code)
	}
	st, err := ExecutePack(code, "", 200000)
	if err != nil {
		t.Fatalf("%v code %s", err, code)
	}
	if st.Out != "\x02\x02" {
		t.Fatalf("out %q code %s", st.Out, code)
	}
}
