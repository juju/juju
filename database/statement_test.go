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

func (s *statementSuite) TestMapToMultiPlaceholder(c *gc.C) {
	binds, vals := MapToMultiPlaceholder(map[string]string{
		"a": "b",
		"c": "d",
	})
	c.Assert(binds, gc.Equals, "(?, ?),(?, ?)")
	c.Assert(len(vals), gc.Equals, 4)
}
