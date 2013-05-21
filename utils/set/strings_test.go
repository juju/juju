// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package set_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/utils/set"
	"sort"
	"testing"
)

func Test(t *testing.T) {
	TestingT(t)
}

type stringSetSuite struct{}

var _ = Suite(stringSetSuite{})

// Helper methods for the tests.
func AssertValues(c *C, s set.Strings, expected ...string) {
	values := s.Values()
	// Expect an empty slice, not a nil slice for values.
	if expected == nil {
		expected = []string{}
	}
	sort.Strings(expected)
	sort.Strings(values)
	c.Assert(values, DeepEquals, expected)
	c.Assert(s.Size(), Equals, len(expected))
	// Check the sorted values too.
	sorted := s.SortedValues()
	c.Assert(sorted, DeepEquals, expected)
}

// Actual tests start here.

func (stringSetSuite) TestEmpty(c *C) {
	s := set.NewStrings()
	AssertValues(c, s)
}

func (stringSetSuite) TestInitialValues(c *C) {
	values := []string{"foo", "bar", "baz"}
	s := set.NewStrings(values...)
	AssertValues(c, s, values...)
}

func (stringSetSuite) TestSize(c *C) {
	// Empty sets are empty.
	s := set.NewStrings()
	c.Assert(s.Size(), Equals, 0)

	// Size returns number of unique values.
	s = set.NewStrings("foo", "foo", "bar")
	c.Assert(s.Size(), Equals, 2)
}

func (stringSetSuite) TestIsEmpty(c *C) {
	// Empty sets are empty.
	s := set.NewStrings()
	c.Assert(s.IsEmpty(), Equals, true)

	// Non-empty sets are not empty.
	s = set.NewStrings("foo")
	c.Assert(s.IsEmpty(), Equals, false)
	// Newly empty sets work too.
	s.Remove("foo")
	c.Assert(s.IsEmpty(), Equals, true)
}

func (stringSetSuite) TestAdd(c *C) {
	s := set.NewStrings()
	s.Add("foo")
	s.Add("foo")
	s.Add("bar")
	AssertValues(c, s, "foo", "bar")
}

func (stringSetSuite) TestRemove(c *C) {
	s := set.NewStrings("foo", "bar")
	s.Remove("foo")
	AssertValues(c, s, "bar")
}

func (stringSetSuite) TestContains(c *C) {
	s := set.NewStrings("foo", "bar")
	c.Assert(s.Contains("foo"), Equals, true)
	c.Assert(s.Contains("bar"), Equals, true)
	c.Assert(s.Contains("baz"), Equals, false)
}

func (stringSetSuite) TestRemoveNonExistent(c *C) {
	s := set.NewStrings()
	s.Remove("foo")
	AssertValues(c, s)
}

func (stringSetSuite) TestUnion(c *C) {
	s1 := set.NewStrings("foo", "bar")
	s2 := set.NewStrings("foo", "baz", "bang")
	union1 := s1.Union(s2)
	union2 := s2.Union(s1)

	AssertValues(c, union1, "foo", "bar", "baz", "bang")
	AssertValues(c, union2, "foo", "bar", "baz", "bang")
}

func (stringSetSuite) TestIntersection(c *C) {
	s1 := set.NewStrings("foo", "bar")
	s2 := set.NewStrings("foo", "baz", "bang")
	int1 := s1.Intersection(s2)
	int2 := s2.Intersection(s1)

	AssertValues(c, int1, "foo")
	AssertValues(c, int2, "foo")
}

func (stringSetSuite) TestDifference(c *C) {
	s1 := set.NewStrings("foo", "bar")
	s2 := set.NewStrings("foo", "baz", "bang")
	diff1 := s1.Difference(s2)
	diff2 := s2.Difference(s1)

	AssertValues(c, diff1, "bar")
	AssertValues(c, diff2, "baz", "bang")
}

func (stringSetSuite) TestUninitialized(c *C) {
	var uninitialized set.Strings
	c.Assert(uninitialized.Size(), Equals, 0)
	c.Assert(uninitialized.IsEmpty(), Equals, true)
	// You can get values and sorted values from an unitialized set.
	AssertValues(c, uninitialized)
	// All contains checks are false
	c.Assert(uninitialized.Contains("foo"), Equals, false)
	// Remove works on an uninitialized Strings
	uninitialized.Remove("foo")

	var other set.Strings
	// Union returns a new set that is empty but initialized.
	c.Assert(uninitialized.Union(other), DeepEquals, set.NewStrings())
	c.Assert(uninitialized.Intersection(other), DeepEquals, set.NewStrings())
	c.Assert(uninitialized.Difference(other), DeepEquals, set.NewStrings())

	other = set.NewStrings("foo", "bar")
	c.Assert(uninitialized.Union(other), DeepEquals, other)
	c.Assert(uninitialized.Intersection(other), DeepEquals, set.NewStrings())
	c.Assert(uninitialized.Difference(other), DeepEquals, set.NewStrings())
	c.Assert(other.Union(uninitialized), DeepEquals, other)
	c.Assert(other.Intersection(uninitialized), DeepEquals, set.NewStrings())
	c.Assert(other.Difference(uninitialized), DeepEquals, other)

	// Once something is added, the set becomes initialized.
	uninitialized.Add("foo")
	AssertValues(c, uninitialized, "foo")
}
