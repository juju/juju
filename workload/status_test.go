// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workload_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/workload"
)

var (
	okayStates = []string{
		workload.StateStarting,
		workload.StateRunning,
		workload.StateStopping,
		workload.StateStopped,
	}
)

type statusSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&statusSuite{})

func (s *statusSuite) TestValidateStateOkay(c *gc.C) {
	for _, state := range okayStates {
		c.Logf("checking %q", state)
		err := workload.ValidateState(state)

		c.Check(err, jc.ErrorIsNil)
	}
}

func (s *statusSuite) TestValidateStateUndefined(c *gc.C) {
	var state string
	err := workload.ValidateState(state)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *statusSuite) TestValidateStateBadState(c *gc.C) {
	state := "some bogus state"
	err := workload.ValidateState(state)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}
