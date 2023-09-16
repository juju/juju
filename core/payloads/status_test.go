// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloads_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/payloads"
	"github.com/juju/juju/testing"
)

var (
	okayStates = []string{
		payloads.StateStarting,
		payloads.StateRunning,
		payloads.StateStopping,
		payloads.StateStopped,
	}
)

type statusSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&statusSuite{})

func (s *statusSuite) TestValidateStateOkay(c *gc.C) {
	for _, state := range okayStates {
		c.Logf("checking %q", state)
		err := payloads.ValidateState(state)

		c.Check(err, jc.ErrorIsNil)
	}
}

func (s *statusSuite) TestValidateStateUndefined(c *gc.C) {
	var state string
	err := payloads.ValidateState(state)

	c.Check(err, jc.ErrorIs, errors.NotValid)
}

func (s *statusSuite) TestValidateStateBadState(c *gc.C) {
	state := "some bogus state"
	err := payloads.ValidateState(state)

	c.Check(err, jc.ErrorIs, errors.NotValid)
}
