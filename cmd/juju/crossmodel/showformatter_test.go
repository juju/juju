// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/crossmodel"
	"github.com/juju/juju/testing"
)

type funcSuite struct {
	testing.BaseSuite
}

func (s *funcSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

var _ = gc.Suite(&funcSuite{})

func (s *funcSuite) TestMaxFirstBigger(c *gc.C) {
	c.Assert(crossmodel.Max(3, 1), gc.DeepEquals, 3)
}

func (s *funcSuite) TestMaxLastBigger(c *gc.C) {
	c.Assert(crossmodel.Max(1, 3), gc.DeepEquals, 3)
}

func (s *funcSuite) TestMaxEquals(c *gc.C) {
	c.Assert(crossmodel.Max(3, 3), gc.DeepEquals, 3)
}

func (s *funcSuite) TestAtInRange(c *gc.C) {
	desc := []string{"one", "two"}
	c.Assert(crossmodel.DescAt(desc, 0), gc.DeepEquals, desc[0])
	c.Assert(crossmodel.DescAt(desc, 1), gc.DeepEquals, desc[1])
}

func (s *funcSuite) TestAtOutRange(c *gc.C) {
	desc := []string{"one", "two"}
	c.Assert(crossmodel.DescAt(desc, 2), gc.DeepEquals, "")
	c.Assert(crossmodel.DescAt(desc, 10), gc.DeepEquals, "")
}

func (s *funcSuite) TestBreakLinesEmpty(c *gc.C) {
	empty := ""
	c.Assert(crossmodel.BreakLines(empty), gc.DeepEquals, []string{empty})
}

func (s *funcSuite) TestBreakLinesOneWord(c *gc.C) {
	aWord := "aWord"
	c.Assert(crossmodel.BreakLines(aWord), gc.DeepEquals, []string{aWord})
}

func (s *funcSuite) TestBreakLinesManyWordsOneLine(c *gc.C) {
	aWord := "aWord aWord aWord aWord aWord"
	c.Assert(crossmodel.BreakLines(aWord), gc.DeepEquals, []string{aWord})
}

func (s *funcSuite) TestBreakLinesManyWordsManyLines(c *gc.C) {
	aWord := "aWord aWord aWord aWord aWord aWord aWord aWord aWord aWord"
	c.Assert(crossmodel.BreakLines(aWord), gc.DeepEquals,
		[]string{
			"aWord aWord aWord aWord aWord aWord aWord",
			"aWord aWord aWord",
		})
}

func (s *funcSuite) TestBreakOneWord(c *gc.C) {
	aWord := "aWord"
	c.Assert(crossmodel.BreakOneWord(aWord), gc.DeepEquals, []string{aWord})
}

func (s *funcSuite) TestBreakOneLongWord(c *gc.C) {
	aWord := "aVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryaWordaWordaWordaWordaWordaWord"
	c.Assert(crossmodel.BreakOneWord(aWord), gc.DeepEquals,
		[]string{
			aWord[0:crossmodel.ColumnWidth],
			aWord[crossmodel.ColumnWidth : crossmodel.ColumnWidth*2],
			aWord[crossmodel.ColumnWidth*2:],
		})
}
