// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package interval_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/interval"
)

type integerIntervalSuite struct{}

var _ = gc.Suite(&integerIntervalSuite{})

func (s *integerIntervalSuite) TestNewIntegerInterval(c *gc.C) {
	i := interval.NewIntegerInterval(1, 5)
	c.Check(i.Lower, gc.Equals, 1)
	c.Check(i.Upper, gc.Equals, 5)

	i = interval.NewIntegerInterval(5, 1)
	c.Check(i.Lower, gc.Equals, 1)
	c.Check(i.Upper, gc.Equals, 5)
}

func (s *integerIntervalSuite) TestIntersects(c *gc.C) {
	i := interval.NewIntegerInterval(1, 5)
	c.Check(i.Intersects(interval.NewIntegerInterval(2, 3)), gc.Equals, true)
	c.Check(i.Intersects(interval.NewIntegerInterval(5, 6)), gc.Equals, true)
	c.Check(i.Intersects(interval.NewIntegerInterval(0, 1)), gc.Equals, true)
	c.Check(i.Intersects(interval.NewIntegerInterval(6, 7)), gc.Equals, false)
	c.Check(i.Intersects(interval.NewIntegerInterval(5, 5)), gc.Equals, true)
	c.Check(i.Intersects(interval.NewIntegerInterval(0, 0)), gc.Equals, false)

	i = interval.NewIntegerInterval(1, 1)
	c.Check(i.Intersects(interval.NewIntegerInterval(0, 0)), gc.Equals, false)
	c.Check(i.Intersects(interval.NewIntegerInterval(0, 1)), gc.Equals, true)
	c.Check(i.Intersects(interval.NewIntegerInterval(1, 1)), gc.Equals, true)
	c.Check(i.Intersects(interval.NewIntegerInterval(2, 2)), gc.Equals, false)
}

func (s *integerIntervalSuite) TestAdjacent(c *gc.C) {
	i := interval.NewIntegerInterval(1, 5)
	c.Check(i.Adjacent(interval.NewIntegerInterval(0, 0)), gc.Equals, true)
	c.Check(i.Adjacent(interval.NewIntegerInterval(6, 6)), gc.Equals, true)
	c.Check(i.Adjacent(interval.NewIntegerInterval(6, 12)), gc.Equals, true)

	c.Check(i.Adjacent(interval.NewIntegerInterval(0, 2)), gc.Equals, false)
	c.Check(i.Adjacent(interval.NewIntegerInterval(4, 6)), gc.Equals, false)
}

func (s *integerIntervalSuite) TestIsSubsetOf(c *gc.C) {
	i := interval.NewIntegerInterval(1, 5)
	c.Check(i.IsSubsetOf(interval.NewIntegerInterval(0, 6)), gc.Equals, true)
	c.Check(i.IsSubsetOf(interval.NewIntegerInterval(1, 5)), gc.Equals, true)
	c.Check(i.IsSubsetOf(interval.NewIntegerInterval(0, 5)), gc.Equals, true)

	c.Check(i.IsSubsetOf(interval.NewIntegerInterval(0, 4)), gc.Equals, false)
	c.Check(i.IsSubsetOf(interval.NewIntegerInterval(2, 6)), gc.Equals, false)
	c.Check(i.IsSubsetOf(interval.NewIntegerInterval(2, 4)), gc.Equals, false)
}

func (s *integerIntervalSuite) TestDifference(c *gc.C) {
	i := interval.NewIntegerInterval(1, 5)

	c.Check(i.Difference(interval.NewIntegerInterval(0, 0)), jc.DeepEquals, interval.IntegerIntervals{i})
	c.Check(i.Difference(interval.NewIntegerInterval(6, 10)), jc.DeepEquals, interval.IntegerIntervals{i})
	c.Check(i.Difference(interval.NewIntegerInterval(0, 1)), jc.DeepEquals, interval.IntegerIntervals{
		{Lower: 2, Upper: 5},
	})
	c.Check(i.Difference(interval.NewIntegerInterval(1, 2)), jc.DeepEquals, interval.IntegerIntervals{
		{Lower: 3, Upper: 5},
	})
	c.Check(i.Difference(interval.NewIntegerInterval(0, 2)), jc.DeepEquals, interval.IntegerIntervals{
		{Lower: 3, Upper: 5},
	})
	c.Check(i.Difference(interval.NewIntegerInterval(2, 3)), jc.DeepEquals, interval.IntegerIntervals{
		{Lower: 1, Upper: 1},
		{Lower: 4, Upper: 5},
	})
	c.Check(i.Difference(interval.NewIntegerInterval(3, 3)), jc.DeepEquals, interval.IntegerIntervals{
		{Lower: 1, Upper: 2},
		{Lower: 4, Upper: 5},
	})

	c.Check(i.Difference(interval.NewIntegerInterval(4, 5)), jc.DeepEquals, interval.IntegerIntervals{
		{Lower: 1, Upper: 3},
	})
	c.Check(i.Difference(interval.NewIntegerInterval(1, 5)), jc.DeepEquals, interval.IntegerIntervals{})
	c.Check(i.Difference(interval.NewIntegerInterval(1, 6)), jc.DeepEquals, interval.IntegerIntervals{})
	c.Check(i.Difference(interval.NewIntegerInterval(0, 6)), jc.DeepEquals, interval.IntegerIntervals{})
}

func (s *integerIntervalSuite) TestNewIntegerIntervals(c *gc.C) {
	iis := interval.NewIntegerIntervals(
		interval.NewIntegerInterval(1, 5),
	)
	c.Check(iis, jc.DeepEquals, interval.IntegerIntervals{
		interval.NewIntegerInterval(1, 5),
	})

	iis = interval.NewIntegerIntervals(
		interval.NewIntegerInterval(1, 5),
		interval.NewIntegerInterval(6, 10),
	)
	c.Check(iis, jc.DeepEquals, interval.IntegerIntervals{
		{Lower: 1, Upper: 10},
	})

	iis = interval.NewIntegerIntervals(
		interval.NewIntegerInterval(6, 10),
		interval.NewIntegerInterval(1, 5),
	)
	c.Check(iis, jc.DeepEquals, interval.IntegerIntervals{
		interval.NewIntegerInterval(1, 10),
	})

	iis = interval.NewIntegerIntervals(
		interval.NewIntegerInterval(1, 5),
		interval.NewIntegerInterval(7, 10),
	)
	c.Check(iis, jc.DeepEquals, interval.IntegerIntervals{
		interval.NewIntegerInterval(1, 5),
		interval.NewIntegerInterval(7, 10),
	})

	iis = interval.NewIntegerIntervals(
		interval.NewIntegerInterval(7, 10),
		interval.NewIntegerInterval(1, 5),
	)
	c.Check(iis, jc.DeepEquals, interval.IntegerIntervals{
		interval.NewIntegerInterval(1, 5),
		interval.NewIntegerInterval(7, 10),
	})

	iis = interval.NewIntegerIntervals()
	c.Check(iis, jc.DeepEquals, interval.IntegerIntervals{})
}

func (s *integerIntervalSuite) TestUnion(c *gc.C) {
	iis := interval.NewIntegerIntervals(
		interval.NewIntegerInterval(1, 5),
	)
	iis = iis.Union(interval.NewIntegerInterval(6, 10))
	c.Check(iis, jc.DeepEquals, interval.IntegerIntervals{
		interval.NewIntegerInterval(1, 10),
	})

	iis = interval.NewIntegerIntervals(
		interval.NewIntegerInterval(1, 5),
	)
	iis = iis.Union(interval.NewIntegerInterval(5, 10))
	c.Check(iis, jc.DeepEquals, interval.IntegerIntervals{
		interval.NewIntegerInterval(1, 10),
	})

	iis = interval.NewIntegerIntervals(
		interval.NewIntegerInterval(1, 5),
	)
	iis = iis.Union(interval.NewIntegerInterval(2, 3))
	c.Check(iis, jc.DeepEquals, interval.IntegerIntervals{
		interval.NewIntegerInterval(1, 5),
	})

	iis = interval.NewIntegerIntervals(
		interval.NewIntegerInterval(1, 5),
	)
	iis = iis.Union(interval.NewIntegerInterval(0, 1))
	c.Check(iis, jc.DeepEquals, interval.IntegerIntervals{
		interval.NewIntegerInterval(0, 5),
	})

	iis = interval.NewIntegerIntervals(
		interval.NewIntegerInterval(1, 5),
	)
	iis = iis.Union(interval.NewIntegerInterval(0, 0))
	c.Check(iis, jc.DeepEquals, interval.IntegerIntervals{
		interval.NewIntegerInterval(0, 5),
	})

	iis = interval.NewIntegerIntervals(
		interval.NewIntegerInterval(1, 5),
	)
	iis = iis.Union(interval.NewIntegerInterval(5, 6))
	c.Check(iis, jc.DeepEquals, interval.IntegerIntervals{
		{Lower: 1, Upper: 6},
	})

	iis = interval.NewIntegerIntervals(
		interval.NewIntegerInterval(1, 5),
	)
	iis = iis.Union(interval.NewIntegerInterval(0, 6))
	c.Check(iis, jc.DeepEquals, interval.IntegerIntervals{
		{Lower: 0, Upper: 6},
	})

	iis = interval.NewIntegerIntervals(
		interval.NewIntegerInterval(1, 5),
		interval.NewIntegerInterval(10, 15),
	)
	iis = iis.Union(interval.NewIntegerInterval(6, 9))
	c.Check(iis, jc.DeepEquals, interval.IntegerIntervals{
		interval.NewIntegerInterval(1, 15),
	})

	iis = interval.NewIntegerIntervals(
		interval.NewIntegerInterval(1, 5),
		interval.NewIntegerInterval(10, 15),
	)
	iis = iis.Union(interval.NewIntegerInterval(3, 12))
	c.Check(iis, jc.DeepEquals, interval.IntegerIntervals{
		interval.NewIntegerInterval(1, 15),
	})

	iis = interval.NewIntegerIntervals(
		interval.NewIntegerInterval(1, 5),
		interval.NewIntegerInterval(10, 15),
		interval.NewIntegerInterval(20, 25),
	)
	iis = iis.Union(interval.NewIntegerInterval(3, 12))
	c.Check(iis, jc.DeepEquals, interval.IntegerIntervals{
		interval.NewIntegerInterval(1, 15),
		interval.NewIntegerInterval(20, 25),
	})

	iis = interval.NewIntegerIntervals(
		interval.NewIntegerInterval(1, 5),
		interval.NewIntegerInterval(10, 15),
		interval.NewIntegerInterval(20, 25),
	)
	iis = iis.Union(interval.NewIntegerInterval(3, 21))
	c.Check(iis, jc.DeepEquals, interval.IntegerIntervals{
		interval.NewIntegerInterval(1, 25),
	})

	iis = interval.NewIntegerIntervals(
		interval.NewIntegerInterval(1, 5),
		interval.NewIntegerInterval(10, 15),
		interval.NewIntegerInterval(20, 25),
	)
	iis = iis.Union(interval.NewIntegerInterval(0, 29))
	c.Check(iis, jc.DeepEquals, interval.IntegerIntervals{
		interval.NewIntegerInterval(0, 29),
	})
}

func (s *integerIntervalSuite) TestIntervalsDifference(c *gc.C) {
	iis := interval.NewIntegerIntervals(
		interval.NewIntegerInterval(1, 5),
		interval.NewIntegerInterval(10, 15),
	)
	iis = iis.Difference(interval.NewIntegerInterval(3, 7))
	c.Check(iis, jc.DeepEquals, interval.IntegerIntervals{
		interval.NewIntegerInterval(1, 2),
		interval.NewIntegerInterval(10, 15),
	})

	iis = interval.NewIntegerIntervals(
		interval.NewIntegerInterval(1, 5),
		interval.NewIntegerInterval(10, 15),
	)
	iis = iis.Difference(interval.NewIntegerInterval(3, 12))
	c.Check(iis, jc.DeepEquals, interval.IntegerIntervals{
		interval.NewIntegerInterval(1, 2),
		interval.NewIntegerInterval(13, 15),
	})

	iis = interval.NewIntegerIntervals(
		interval.NewIntegerInterval(1, 5),
		interval.NewIntegerInterval(10, 15),
	)
	iis = iis.Difference(interval.NewIntegerInterval(6, 9))
	c.Check(iis, jc.DeepEquals, interval.IntegerIntervals{
		interval.NewIntegerInterval(1, 5),
		interval.NewIntegerInterval(10, 15),
	})

	iis = interval.NewIntegerIntervals(
		interval.NewIntegerInterval(1, 5),
		interval.NewIntegerInterval(10, 15),
	)
	iis = iis.Difference(interval.NewIntegerInterval(1, 9))
	c.Check(iis, jc.DeepEquals, interval.IntegerIntervals{
		interval.NewIntegerInterval(10, 15),
	})

	iis = interval.NewIntegerIntervals(
		interval.NewIntegerInterval(1, 5),
		interval.NewIntegerInterval(10, 15),
	)
	iis = iis.Difference(interval.NewIntegerInterval(1, 20))
	c.Check(iis, jc.DeepEquals, interval.IntegerIntervals{})
}
