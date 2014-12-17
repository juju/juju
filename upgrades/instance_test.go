// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

type instanceSuite struct {
	testing.FakeJujuHomeSuite
	ctx upgrades.Context
}

var _ = gc.Suite(&instanceSuite{})

func (s *instanceSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)

	s.ctx = &mockContext{}
}

func (s *instanceSuite) TestAddAvaililityZoneToInstanceData(c *gc.C) {
	var stArg *state.State
	s.PatchValue(upgrades.AddAZToInstData,
		func(st *state.State, azFunc func(*state.State, instance.Id) (string, error)) error {
			stArg = st
			// We can't compare functions for equality so we trust that
			// azFunc is correct.
			return nil
		},
	)

	err := upgrades.AddAvaililityZoneToInstanceData(s.ctx)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(stArg, gc.Equals, s.ctx.State())
}
