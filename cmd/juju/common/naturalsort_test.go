// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"sort"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type naturalSortSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&naturalSortSuite{})

func (s *naturalSortSuite) TestNaturallyEmpty(c *gc.C) {
	s.assertNaturallySort(
		c,
		[]string{},
		[]string{},
	)
}

func (s *naturalSortSuite) TestNaturallyAlpha(c *gc.C) {
	s.assertNaturallySort(
		c,
		[]string{"bac", "cba", "abc"},
		[]string{"abc", "bac", "cba"},
	)
}

func (s *naturalSortSuite) TestNaturallyAlphaNumeric(c *gc.C) {
	s.assertNaturallySort(
		c,
		[]string{"a1", "a10", "a100", "a11"},
		[]string{"a1", "a10", "a11", "a100"},
	)
}

func (s *naturalSortSuite) TestNaturallySpecial(c *gc.C) {
	s.assertNaturallySort(
		c,
		[]string{"a1", "a10", "a100", "a1/1", "1a"},
		[]string{"1a", "a1", "a1/1", "a10", "a100"},
	)
}

func (s *naturalSortSuite) TestNaturallyTagLike(c *gc.C) {
	s.assertNaturallySort(
		c,
		[]string{"a1/1", "a1/11", "a1/2", "a1/7", "a1/100"},
		[]string{"a1/1", "a1/2", "a1/7", "a1/11", "a1/100"},
	)
}

func (s *naturalSortSuite) TestNaturallySeveralNumericParts(c *gc.C) {
	s.assertNaturallySort(
		c,
		[]string{"x2-y08", "x2-g8", "x8-y8", "x2-y7"},
		[]string{"x2-g8", "x2-y7", "x2-y08", "x8-y8"},
	)
}

func (s *naturalSortSuite) TestNaturallyFoo(c *gc.C) {
	s.assertNaturallySort(
		c,
		[]string{"foo2", "foo01"},
		[]string{"foo01", "foo2"},
	)
}

func (s *naturalSortSuite) TestNaturallyIPs(c *gc.C) {
	s.assertNaturallySort(
		c,
		[]string{"100.001.010.123", "001.001.010.123", "001.002.010.123"},
		[]string{"001.001.010.123", "001.002.010.123", "100.001.010.123"},
	)
}

func (s *naturalSortSuite) TestNaturallyJuju(c *gc.C) {
	s.assertNaturallySort(
		c,
		[]string{
			"ubuntu/0",
			"ubuntu/1",
			"ubuntu/10",
			"ubuntu/100",
			"ubuntu/101",
			"ubuntu/102",
			"ubuntu/103",
			"ubuntu/104",
			"ubuntu/11"},
		[]string{
			"ubuntu/0",
			"ubuntu/1",
			"ubuntu/10",
			"ubuntu/11",
			"ubuntu/100",
			"ubuntu/101",
			"ubuntu/102",
			"ubuntu/103",
			"ubuntu/104"},
	)
}

func (s *naturalSortSuite) assertNaturallySort(c *gc.C, sample, expected []string) {
	sort.Sort(naturally(sample))
	c.Assert(sample, gc.DeepEquals, expected)
}
