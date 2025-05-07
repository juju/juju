// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitterminationworker_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/caasunitterminationworker"
)

type ManifoldSuite struct {
	config caasunitterminationworker.ManifoldConfig
}

var _ = tc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.config = caasunitterminationworker.ManifoldConfig{
		Clock:  testclock.NewClock(time.Now()),
		Logger: loggertesting.WrapCheckLog(c),
	}
}

func (s *ManifoldSuite) TestConfigValidation(c *tc.C) {
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ManifoldSuite) TestConfigValidationMissingClock(c *tc.C) {
	s.config.Clock = nil
	err := s.config.Validate()
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, tc.ErrorMatches, "missing Clock not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingLogger(c *tc.C) {
	s.config.Logger = nil
	err := s.config.Validate()
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, tc.ErrorMatches, "missing Logger not valid")
}
