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
func AssertEmpty(c *C, s set.StringSet) {
	c.Assert(len(s.Values()), Equals, 0)
}

func AssertValues(c *C, s set.StringSet, expected ...string) {
	values := s.Values()
	sort.Strings(expected)
	sort.Strings(values)
	c.Assert(values, DeepEquals, expected)
}

func AssertSortedValues(c *C, s set.StringSet, expected ...string) {
	values := s.Values()
	sort.Strings(expected)
	c.Assert(values, DeepEquals, expected)
}

// Actual tests start here.
func (stringSetSuite) TestEmpty(c *C) {
	s := set.MakeStringSet()
	AssertEmpty(c, s)
}

func (stringSetSuite) TestInitialValues(c *C) {
	s := set.MakeStringSet("foo", "bar", "baz")
	AssertValues(c, s, "foo", "bar", "baz")
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
	AssertEmpty(c, s)
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
