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
	binds := SliceToPlaceholder(args)
	c.Check(binds, gc.Equals, "?,?,?,?")
}
