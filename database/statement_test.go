// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"github.com/juju/testing"
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

func (s *statementSuite) TestMapKeysToPlaceHolder(c *gc.C) {
	args := map[string]string{
		"foo": "1",
		"bar": "2",
	}
	binds, vals := MapKeysToPlaceHolder(args)
	c.Check(binds, gc.Equals, "?,?")
	c.Check(vals, gc.DeepEquals, []any{"foo", "bar"})
}

func (s *statementSuite) TestNilMapKeysToPlaceHolder(c *gc.C) {
	binds, vals := MapKeysToPlaceHolder(map[string]string(nil))
	c.Check(binds, gc.Equals, "")
	c.Check(vals, gc.HasLen, 0)
}
