// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package checkers_test

import (
	"testing"

	gc "launchpad.net/gocheck"

	jc "launchpad.net/juju-core/testing/checkers"
)

func Test(t *testing.T) { gc.TestingT(t) }

type CheckerSuite struct{}

var _ = gc.Suite(&CheckerSuite{})

func (s *CheckerSuite) TestHasPrefix(c *gc.C) {
	c.Assert("foo bar", jc.HasPrefix, "foo")
	c.Assert("foo bar", gc.Not(jc.HasPrefix), "omg")
}

func (s *CheckerSuite) TestHasSuffix(c *gc.C) {
	c.Assert("foo bar", jc.HasSuffix, "bar")
	c.Assert("foo bar", gc.Not(jc.HasSuffix), "omg")
}

func (s *CheckerSuite) TestContains(c *gc.C) {
	c.Assert("foo bar baz", jc.Contains, "foo")
	c.Assert("foo bar baz", jc.Contains, "bar")
	c.Assert("foo bar baz", jc.Contains, "baz")
	c.Assert("foo bar baz", gc.Not(jc.Contains), "omg")
}

type test struct {
	s string
	i int
}

func (s *CheckerSuite) TestSameSlice(c *gc.C) {
	//// positive cases ////

	// same
	c.Check(
		[]int{1, 2, 3}, jc.SameSlice,
		[]int{1, 2, 3})

	// empty
	c.Check(
		[]int{}, jc.SameSlice,
		[]int{})

	// single
	c.Check(
		[]int{1}, jc.SameSlice,
		[]int{1})

	// different order
	c.Check(
		[]int{1, 2, 3}, jc.SameSlice,
		[]int{3, 2, 1})

	// multiple copies of same
	c.Check(
		[]int{1, 1, 2}, jc.SameSlice,
		[]int{2, 1, 1})

	// test structs
	c.Check(
		[]test{{"a", 1}, {"b", 2}}, jc.SameSlice,
		[]test{{"b", 2}, {"a", 1}})

	//// negative cases ////

	// different contents
	c.Check(
		[]int{1, 2, 3}, gc.Not(jc.SameSlice),
		[]int{1, 2, 4})

	// different size
	c.Check(
		[]int{1, 2, 3}, gc.Not(jc.SameSlice),
		[]int{1, 2})

	// different type
	c.Check(
		[]int{1, 2, 3}, gc.Not(jc.SameSlice),
		[]string{"1", "2", "3"})

	// different counts of same items
	c.Check(
		[]int{1, 1, 2}, gc.Not(jc.SameSlice),
		[]int{1, 2, 2})

	// not a slice
	c.Check(
		"test", gc.Not(jc.SameSlice),
		[]int{1})

	// not a slice
	c.Check(
		[]int{1}, gc.Not(jc.SameSlice),
		"test")
}
