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
	condition, args := MakeQueryCondition(nil)
	c.Assert(condition, gc.Equals, "")
	c.Assert(args, gc.HasLen, 0)
}

func (s *statementSuite) TestMakeQueryConditionSingle(c *gc.C) {
	condition, args := MakeQueryCondition(map[string]any{
		"t1.col": "",
		"t2.col": "foo",
	})
	c.Assert(condition, gc.Equals, "t2.col = $M.t2_col")
	c.Assert(args, jc.DeepEquals, sqlair.M{"t2_col": "foo"})
}

func (s *statementSuite) TestMakeQueryConditionMultiple(c *gc.C) {
	condition, args := MakeQueryCondition(map[string]any{
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
