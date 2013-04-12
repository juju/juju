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
func AssertValues(c *C, s set.StringSet, expected ...string) {
	values := s.Values()
	// Expect an empty slice, not a nil slice for values.
	if expected == nil {
		expected = []string{}
	}
	sort.Strings(expected)
	sort.Strings(values)
	c.Assert(values, DeepEquals, expected)
	c.Assert(s.Size(), Equals, len(expected))
}

func AssertSortedValues(c *C, s set.StringSet, expected ...string) {
	// Expect an empty slice, not a nil slice for values.
	if expected == nil {
		expected = []string{}
	}
	values := s.SortedValues()
	sort.Strings(expected)
	c.Assert(values, DeepEquals, expected)
	c.Assert(s.Size(), Equals, len(expected))
}

// Actual tests start here.

func (stringSetSuite) TestEmpty(c *C) {
	s := set.MakeStringSet()
	AssertValues(c, s)
	AssertSortedValues(c, s)
}

func (stringSetSuite) TestInitialValues(c *C) {
	values := []string{"foo", "bar", "baz"}
	s := set.MakeStringSet(values...)
	AssertValues(c, s, values...)
	AssertSortedValues(c, s, values...)
}

func (stringSetSuite) TestSize(c *C) {
	// Empty sets are empty.
	s := set.MakeStringSet()
	c.Assert(s.Size(), Equals, 0)

	// Size returns number of unique values.
	s = set.MakeStringSet("foo", "foo", "bar")
	c.Assert(s.Size(), Equals, 2)
}

func (stringSetSuite) TestAdd(c *C) {
	s := set.MakeStringSet()
	s.Add("foo")
	s.Add("foo")
	s.Add("bar")
	AssertValues(c, s, "foo", "bar")
}

func (stringSetSuite) TestRemove(c *C) {
	s := set.MakeStringSet("foo", "bar")
	s.Remove("foo")
	AssertValues(c, s, "bar")
}

func (stringSetSuite) TestContains(c *C) {
	s := set.MakeStringSet("foo", "bar")
	c.Assert(s.Contains("foo"), Equals, true)
	c.Assert(s.Contains("bar"), Equals, true)
	c.Assert(s.Contains("baz"), Equals, false)
}

func (stringSetSuite) TestRemoveNonExistant(c *C) {
	s := set.MakeStringSet()
	s.Remove("foo")
	AssertValues(c, s)
}

func (stringSetSuite) TestUnion(c *C) {
	s1 := set.MakeStringSet("foo", "bar")
	s2 := set.MakeStringSet("foo", "baz", "bang")
	union1 := s1.Union(s2)
	union2 := s2.Union(s1)

	AssertValues(c, union1, "foo", "bar", "baz", "bang")
	AssertValues(c, union2, "foo", "bar", "baz", "bang")
}

func (stringSetSuite) TestIntersection(c *C) {
	s1 := set.MakeStringSet("foo", "bar")
	s2 := set.MakeStringSet("foo", "baz", "bang")
	int1 := s1.Intersection(s2)
	int2 := s2.Intersection(s1)

	AssertValues(c, int1, "foo")
	AssertValues(c, int2, "foo")
}

func (stringSetSuite) TestDifference(c *C) {
	s1 := set.MakeStringSet("foo", "bar")
	s2 := set.MakeStringSet("foo", "baz", "bang")
	diff1 := s1.Difference(s2)
	diff2 := s2.Difference(s1)

	AssertValues(c, diff1, "bar")
	AssertValues(c, diff2, "baz", "bang")
}

func (stringSetSuite) TestUninitialized(c *C) {
	var uninitialized set.StringSet
	c.Assert(uninitialized.Size(), Equals, 0)
	// You can get values and sorted values from an unitialized set.
	AssertValues(c, uninitialized)
	AssertSortedValues(c, uninitialized)
	// All contains checks are false
	c.Assert(uninitialized.Contains("foo"), Equals, false)
	// Remove works on an uninitialized StringSet
	uninitialized.Remove("foo")

	var other set.StringSet
	// Union returns a new set that is empty but initialized.
	c.Assert(uninitialized.Union(other), DeepEquals, set.MakeStringSet())
	c.Assert(uninitialized.Intersection(other), DeepEquals, set.MakeStringSet())
	c.Assert(uninitialized.Difference(other), DeepEquals, set.MakeStringSet())

	other = set.MakeStringSet("foo", "bar")
	c.Assert(uninitialized.Union(other), DeepEquals, other)
	c.Assert(uninitialized.Intersection(other), DeepEquals, set.MakeStringSet())
	c.Assert(uninitialized.Difference(other), DeepEquals, set.MakeStringSet())
	c.Assert(other.Union(uninitialized), DeepEquals, other)
	c.Assert(other.Intersection(uninitialized), DeepEquals, set.MakeStringSet())
	c.Assert(other.Difference(uninitialized), DeepEquals, other)

	// Once something is added, the set becomes initialized.
	uninitialized.Add("foo")
	AssertValues(c, uninitialized, "foo")
}
