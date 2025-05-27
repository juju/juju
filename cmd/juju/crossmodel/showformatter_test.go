// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	gc "gopkg.in/check.v1"

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
	c.Assert(max(3, 1), gc.DeepEquals, 3)
}

func (s *funcSuite) TestMaxLastBigger(c *gc.C) {
	c.Assert(max(1, 3), gc.DeepEquals, 3)
}

func (s *funcSuite) TestMaxEquals(c *gc.C) {
	c.Assert(max(3, 3), gc.DeepEquals, 3)
}

func (s *funcSuite) TestAtInRange(c *gc.C) {
	desc := []string{"one", "two"}
	c.Assert(descAt(desc, 0), gc.DeepEquals, desc[0])
	c.Assert(descAt(desc, 1), gc.DeepEquals, desc[1])
}

func (s *funcSuite) TestAtOutRange(c *gc.C) {
	desc := []string{"one", "two"}
	c.Assert(descAt(desc, 2), gc.DeepEquals, "")
	c.Assert(descAt(desc, 10), gc.DeepEquals, "")
}

func (s *funcSuite) TestBreakLinesEmpty(c *gc.C) {
	empty := ""
	c.Assert(breakLines(empty), gc.DeepEquals, []string{empty})
}

func (s *funcSuite) TestBreakLinesOneWord(c *gc.C) {
	aWord := "aWord"
	c.Assert(breakLines(aWord), gc.DeepEquals, []string{aWord})
}

func (s *funcSuite) TestBreakLinesManyWordsOneLine(c *gc.C) {
	aWord := "aWord aWord aWord aWord aWord"
	c.Assert(breakLines(aWord), gc.DeepEquals, []string{aWord})
}

func (s *funcSuite) TestBreakLinesManyWordsManyLines(c *gc.C) {
	aWord := "aWord aWord aWord aWord aWord aWord aWord aWord aWord aWord"
	c.Assert(breakLines(aWord), gc.DeepEquals,
		[]string{
			"aWord aWord aWord aWord aWord aWord aWord",
			"aWord aWord aWord",
		})
}

func (s *funcSuite) TestBreakLinesManyWordsManyLinesOverflow(c *gc.C) {
	// This causes a panic, because the last word is too long and it doesn't fit
	// in the last line. So, we need to grow the lines by one to accommodate
	// the last word.
	aWord := "aWord aWord aWord aWord aWord aWord aWord aWord aWord aWord aWord aWord aWord aWord aWord aWord aWord aWord aWord aWord aWord panicme"
	c.Assert(breakLines(aWord), gc.DeepEquals,
		[]string{
			"aWord aWord aWord aWord aWord aWord aWord",
			"aWord aWord aWord aWord aWord aWord aWord",
			"aWord aWord aWord aWord aWord aWord aWord",
			"panicme",
		})
}

func (s *funcSuite) TestBreakOneWord(c *gc.C) {
	aWord := "aWord"
	c.Assert(breakOneWord(aWord), gc.DeepEquals, []string{aWord})
}

func (s *funcSuite) TestBreakOneLongWord(c *gc.C) {
	aWord := "aVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryaWordaWordaWordaWordaWordaWord"
	c.Assert(breakOneWord(aWord), gc.DeepEquals,
		[]string{
			aWord[0:columnWidth],
			aWord[columnWidth : columnWidth*2],
			aWord[columnWidth*2:],
		})
}
