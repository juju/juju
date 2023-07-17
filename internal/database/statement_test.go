// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"github.com/canonical/sqlair"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type statementSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&statementSuite{})

func (s *statementSuite) TestSliceToPlaceholder(c *gc.C) {
	args := []string{"won", "too", "free", "for"}
	binds, vals := SliceToPlaceholder(args)
	c.Check(binds, gc.Equals, "?,?,?,?")
	c.Check(vals, gc.DeepEquals, []any{"won", "too", "free", "for"})
}

func (s *statementSuite) TestNilSliceToPlaceholder(c *gc.C) {
	binds, vals := SliceToPlaceholder[any](nil)
	c.Assert(binds, gc.Equals, "")
	c.Assert(vals, gc.NotNil)
	c.Assert(len(vals), gc.Equals, 0)
}

func (s *statementSuite) TestSliceToPlaceholderTransform(c *gc.C) {
	args := []string{"won", "too", "free", "for"}
	count := 0
	binds, vals := SliceToPlaceholderTransform(args, func(s string) any {
		count++
		return s
	})
	c.Check(binds, gc.Equals, "?,?,?,?")
	c.Check(vals, gc.DeepEquals, []any{"won", "too", "free", "for"})
	c.Check(count, gc.Equals, 4)
}

func (s *statementSuite) TestNilSliceToPlaceholderTransform(c *gc.C) {
	count := 0
	binds, vals := SliceToPlaceholderTransform(nil, func(s string) any {
		count++
		return s
	})
	c.Check(binds, gc.Equals, "")
	c.Check(vals, gc.NotNil)
	c.Check(len(vals), gc.Equals, 0)
	c.Check(count, gc.Equals, 0)
}

func (s *statementSuite) TestMakeBindArgs(c *gc.C) {
	binds := MakeBindArgs(2, 3)
	c.Assert(binds, gc.Equals, "(?, ?), (?, ?), (?, ?)")
}

func (s *statementSuite) TestEmptyMakeQueryCondition(c *gc.C) {
	condition, args := SqlairClauseAnd(nil)
	c.Assert(condition, gc.Equals, "")
	c.Assert(args, gc.HasLen, 0)
}

func (s *statementSuite) TestMakeQueryConditionSingle(c *gc.C) {
	condition, args := SqlairClauseAnd(map[string]any{
		"t1.col": "",
		"t2.col": "foo",
	})
	c.Assert(condition, gc.Equals, "t2.col = $M.t2_col")
	c.Assert(args, jc.DeepEquals, sqlair.M{"t2_col": "foo"})
}

func (s *statementSuite) TestMakeQueryConditionMultiple(c *gc.C) {
	condition, args := SqlairClauseAnd(map[string]any{
		"t1.col": "",
		"t2.col": "foo",
		"t3.col": 123,
	})
	if condition != "t2.col = $M.t2_col AND t3.col = $M.t3_col" &&
		condition != "t3.col = $M.t3_col AND t2.col = $M.t2_col" {
		c.Fatalf("unexpected condition: %q", condition)
	}
	c.Assert(args, jc.DeepEquals, sqlair.M{"t2_col": "foo", "t3_col": 123})
}

func (s *statementSuite) TestMapToMultiPlaceholderNil(c *gc.C) {
	var nilMap map[string]string
	bind, vals := MapToMultiPlaceholder(nilMap)
	c.Assert(bind, gc.Equals, "")
	c.Assert(len(vals), gc.Equals, 0)
}

func (s *statementSuite) TestMapToMultiPlaceholder(c *gc.C) {
	m := map[string]string{
		"one":   "two",
		"three": "four",
		"five":  "six",
	}
	bind, vals := MapToMultiPlaceholder(m)
	c.Assert(bind, gc.Equals, "(?, ?),(?, ?),(?, ?)")
	c.Assert(len(vals), gc.Equals, 6)
	count := 0
	for i := 0; i < len(vals); i += 2 {
		v := vals[i]
		switch v {
		case "one":
			count += 1
			c.Assert(vals[i+1], gc.Equals, "two")
		case "three":
			count += 3
			c.Assert(vals[i+1], gc.Equals, "four")
		case "five":
			count += 5
			c.Assert(vals[i+1], gc.Equals, "six")
		default:
			c.Fatalf("unexpected vals key %s", v)
		}
	}
	c.Assert(count, gc.Equals, 9)
}
