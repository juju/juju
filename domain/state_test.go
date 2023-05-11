// Copyright 2033 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/database/testing"
)

type dbFactorySuite struct {
	testing.ControllerSuite
}

var _ = gc.Suite(&dbFactorySuite{})

func (s *dbFactorySuite) TestTrackedDBFactory(c *gc.C) {
	factory := TrackedDBFactory(s.TrackedDB())

	state := NewStateBase(factory)
	db, err := state.DB()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(db, gc.Equals, s.TrackedDB())
}
