// Copyright 2026 The Recursive Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "fmt"

// Stop is the machine state after a program halts.
type Stop struct {
	Out   string
	Cells []byte // Cells[i] is absolute tape cell Base+i
	Base  int
	Ptr   int   // absolute tape index; axis 0 on the pack machine
	Y     int   // axis 1 on the pack machine; Execute leaves this 0
	Pos   []int // pack-machine coordinate; nil after Execute
	Steps int
}

// At returns the value of an absolute tape cell.
func (s Stop) At(abs int) byte {
	i := abs - s.Base
	if i < 0 || i >= len(s.Cells) {
		return 0
	}
	return s.Cells[i]
}

// Execute runs code on an 8-bit wrapping tape.
// Commands are ><+-.,[] . Every other byte is ignored.
// The tape grows in both directions. ',' at end of input stores 0.
// Brackets must match. Execution fails if it exceeds maxSteps.
func Execute(code string, maxSteps int) (Stop, error) {
	if maxSteps <= 0 {
		return Stop{}, fmt.Errorf("maxSteps must be positive")
	}
	jump, err := brackets(code)
	if err != nil {
		return Stop{}, err
	}

	t := tape{cells: []byte{0}}
	ptr := 0
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
			*t.at(ptr) = 0
		case '[':
			if *t.at(ptr) == 0 {
				ip = jump[ip]
			}
		case ']':
			if *t.at(ptr) != 0 {
				ip = jump[ip]
			}
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

// tape stores cells so that cells[i] is absolute cell base+i.
type tape struct {
	cells []byte
	base  int
}

func (t *tape) at(abs int) *byte {
	i := abs - t.base
	if i < 0 {
		extra := -i + 32
		n := make([]byte, len(t.cells)+extra)
		copy(n[extra:], t.cells)
		t.cells = n
		t.base -= extra
		i += extra
	}
	if i >= len(t.cells) {
		n := make([]byte, i+32)
		copy(n, t.cells)
		t.cells = n
	}
	return &t.cells[i]
}

func brackets(code string) ([]int, error) {
	jump := make([]int, len(code))
	stack := make([]int, 0, 16)
	for i := 0; i < len(code); i++ {
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
	}
	if len(stack) != 0 {
		return nil, fmt.Errorf("unmatched [ at %d", stack[len(stack)-1])
	}
	return jump, nil
}
