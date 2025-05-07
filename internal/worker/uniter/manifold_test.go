// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/model"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/uniter"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	config uniter.ManifoldConfig
}

var _ = tc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = uniter.ManifoldConfig{
		Clock:       testclock.NewClock(time.Now()),
		MachineLock: fakeLock{},
		Logger:      loggertesting.WrapCheckLog(c),
		ModelType:   model.IAAS,
	}
}

func (s *ManifoldSuite) TestConfigValidation(c *tc.C) {
	err := s.config.Validate()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ManifoldSuite) TestConfigValidationMissingClock(c *tc.C) {
	s.config.Clock = nil
	err := s.config.Validate()
	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(err, tc.ErrorMatches, "missing Clock not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingMachineLock(c *tc.C) {
	s.config.MachineLock = nil
	err := s.config.Validate()
	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(err, tc.ErrorMatches, "missing MachineLock not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingLogger(c *tc.C) {
	s.config.Logger = nil
	err := s.config.Validate()
	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(err, tc.ErrorMatches, "missing Logger not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingModelType(c *tc.C) {
	s.config.ModelType = ""
	err := s.config.Validate()
	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(err, tc.ErrorMatches, "missing model type not valid")
}

type fakeLock struct {
	machinelock.Lock
}
