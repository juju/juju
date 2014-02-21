// Copied with small adaptations from the reflect package in the
// Go source tree. We use testing rather than gocheck to preserve
// as much source equivalence as possible.

// TODO tests for error messages

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package checkers_test

import (
	"regexp"
	"testing"

	"launchpad.net/juju-core/testing/checkers"
)

func deepEqual(a1, a2 interface{}) bool {
	ok, _ := checkers.DeepEqual(a1, a2)
	return ok
}

type Basic struct {
	x int
	y float32
}

type NotBasic Basic

type DeepEqualTest struct {
	a, b interface{}
	eq   bool
	msg  string
}

// Simple functions for DeepEqual tests.
var (
	fn1 func()             // nil.
	fn2 func()             // nil.
	fn3 = func() { fn1() } // Not nil.
)

var deepEqualTests = []DeepEqualTest{
	// Equalities
	{nil, nil, true, ""},
	{1, 1, true, ""},
	{int32(1), int32(1), true, ""},
	{0.5, 0.5, true, ""},
	{float32(0.5), float32(0.5), true, ""},
	{"hello", "hello", true, ""},
	{make([]int, 10), make([]int, 10), true, ""},
	{&[3]int{1, 2, 3}, &[3]int{1, 2, 3}, true, ""},
	{Basic{1, 0.5}, Basic{1, 0.5}, true, ""},
	{error(nil), error(nil), true, ""},
	{map[int]string{1: "one", 2: "two"}, map[int]string{2: "two", 1: "one"}, true, ""},
	{fn1, fn2, true, ""},

	// Inequalities
	{1, 2, false, `mismatch at top level: unequal; obtained 1; expected 2`},
	{int32(1), int32(2), false, `mismatch at top level: unequal; obtained 1; expected 2`},
	{0.5, 0.6, false, `mismatch at top level: unequal; obtained 0\.5; expected 0\.6`},
	{float32(0.5), float32(0.6), false, `mismatch at top level: unequal; obtained 0\.5; expected 0\.6`},
	{"hello", "hey", false, `mismatch at top level: unequal; obtained "hello"; expected "hey"`},
	{make([]int, 10), make([]int, 11), false, `mismatch at top level: length mismatch, 10 vs 11; obtained \[\]int\{0, 0, 0, 0, 0, 0, 0, 0, 0, 0\}; expected \[\]int\{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0\}`},
	{&[3]int{1, 2, 3}, &[3]int{1, 2, 4}, false, `mismatch at \(\*\)\[2\]: unequal; obtained 3; expected 4`},
	{Basic{1, 0.5}, Basic{1, 0.6}, false, `mismatch at \.y: unequal; obtained 0\.5; expected 0\.6`},
	{Basic{1, 0}, Basic{2, 0}, false, `mismatch at \.x: unequal; obtained 1; expected 2`},
	{map[int]string{1: "one", 3: "two"}, map[int]string{2: "two", 1: "one"}, false, `mismatch at \[3\]: validity mismatch; obtained "two"; expected <nil>`},
	{map[int]string{1: "one", 2: "txo"}, map[int]string{2: "two", 1: "one"}, false, `mismatch at \[2\]: unequal; obtained "txo"; expected "two"`},
	{map[int]string{1: "one"}, map[int]string{2: "two", 1: "one"}, false, `mismatch at top level: length mismatch, 1 vs 2; obtained map\[int\]string\{1:"one"\}; expected map\[int\]string\{2:"two", 1:"one"\}`},
	{map[int]string{2: "two", 1: "one"}, map[int]string{1: "one"}, false, `mismatch at top level: length mismatch, 2 vs 1; obtained map\[int\]string\{2:"two", 1:"one"\}; expected map\[int\]string\{1:"one"\}`},
	{nil, 1, false, `mismatch at top level: nil vs non-nil mismatch; obtained <nil>; expected 1`},
	{1, nil, false, `mismatch at top level: nil vs non-nil mismatch; obtained 1; expected <nil>`},
	{fn1, fn3, false, `mismatch at top level: non-nil functions; obtained \(func\(\)\)\(nil\); expected \(func\(\)\)\(0x[0-9a-f]+\)`},
	{fn3, fn3, false, `mismatch at top level: non-nil functions; obtained \(func\(\)\)\(0x[0-9a-f]+\); expected \(func\(\)\)\(0x[0-9a-f]+\)`},

	// Nil vs empty: they're the same (difference from normal DeepEqual)
	{[]int{}, []int(nil), true, ""},
	{[]int{}, []int{}, true, ""},
	{[]int(nil), []int(nil), true, ""},

	// Mismatched types
	{1, 1.0, false, `mismatch at top level: type mismatch int vs float64; obtained 1; expected 1`},
	{int32(1), int64(1), false, `mismatch at top level: type mismatch int32 vs int64; obtained 1; expected 1`},
	{0.5, "hello", false, `mismatch at top level: type mismatch float64 vs string; obtained 0\.5; expected "hello"`},
	{[]int{1, 2, 3}, [3]int{1, 2, 3}, false, `mismatch at top level: type mismatch \[\]int vs \[3\]int; obtained \[\]int\{1, 2, 3\}; expected \[3\]int\{1, 2, 3\}`},
	{&[3]interface{}{1, 2, 4}, &[3]interface{}{1, 2, "s"}, false, `mismatch at \(\*\)\[2\]: type mismatch int vs string; obtained 4; expected "s"`},
	{Basic{1, 0.5}, NotBasic{1, 0.5}, false, `mismatch at top level: type mismatch checkers_test\.Basic vs checkers_test\.NotBasic; obtained checkers_test\.Basic\{x:1, y:0\.5\}; expected checkers_test\.NotBasic\{x:1, y:0\.5\}`},
	{map[uint]string{1: "one", 2: "two"}, map[int]string{2: "two", 1: "one"}, false, `mismatch at top level: type mismatch map\[uint\]string vs map\[int\]string; obtained map\[uint\]string\{0x1:"one", 0x2:"two"\}; expected map\[int\]string\{2:"two", 1:"one"\}`},
}

func TestDeepEqual(t *testing.T) {
	for _, test := range deepEqualTests {
		r, err := checkers.DeepEqual(test.a, test.b)
		if r != test.eq {
			t.Errorf("deepEqual(%v, %v) = %v, want %v", test.a, test.b, r, test.eq)
		}
		if test.eq {
			if err != nil {
				t.Errorf("deepEqual(%v, %v): unexpected error message %q when equal", test.a, test.b, err)
			}
		} else {
			if ok, _ := regexp.MatchString(test.msg, err.Error()); !ok {
				t.Errorf("deepEqual(%v, %v); unexpected error %q, want %q", test.a, test.b, err.Error(), test.msg)
			}
		}
	}
}

type Recursive struct {
	x int
	r *Recursive
}

func TestDeepEqualRecursiveStruct(t *testing.T) {
	a, b := new(Recursive), new(Recursive)
	*a = Recursive{12, a}
	*b = Recursive{12, b}
	if !deepEqual(a, b) {
		t.Error("deepEqual(recursive same) = false, want true")
	}
}

type _Complex struct {
	a int
	b [3]*_Complex
	c *string
	d map[float64]float64
}

func TestDeepEqualComplexStruct(t *testing.T) {
	m := make(map[float64]float64)
	stra, strb := "hello", "hello"
	a, b := new(_Complex), new(_Complex)
	*a = _Complex{5, [3]*_Complex{a, b, a}, &stra, m}
	*b = _Complex{5, [3]*_Complex{b, a, a}, &strb, m}
	if !deepEqual(a, b) {
		t.Error("deepEqual(complex same) = false, want true")
	}
}

func TestDeepEqualComplexStructInequality(t *testing.T) {
	m := make(map[float64]float64)
	stra, strb := "hello", "helloo" // Difference is here
	a, b := new(_Complex), new(_Complex)
	*a = _Complex{5, [3]*_Complex{a, b, a}, &stra, m}
	*b = _Complex{5, [3]*_Complex{b, a, a}, &strb, m}
	if deepEqual(a, b) {
		t.Error("deepEqual(complex different) = true, want false")
	}
}

type UnexpT struct {
	m map[int]int
}

func TestDeepEqualUnexportedMap(t *testing.T) {
	// Check that DeepEqual can look at unexported fields.
	x1 := UnexpT{map[int]int{1: 2}}
	x2 := UnexpT{map[int]int{1: 2}}
	if !deepEqual(&x1, &x2) {
		t.Error("deepEqual(x1, x2) = false, want true")
	}

	y1 := UnexpT{map[int]int{2: 3}}
	if deepEqual(&x1, &y1) {
		t.Error("deepEqual(x1, y1) = true, want false")
	}
}
