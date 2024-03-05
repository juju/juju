// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/worker/uniter"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	config uniter.ManifoldConfig
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = uniter.ManifoldConfig{
		Clock:       testclock.NewClock(time.Now()),
		MachineLock: fakeLock{},
		Logger:      loggo.GetLogger("test"),
		ModelType:   model.IAAS,
	}
}

func (s *ManifoldSuite) TestConfigValidation(c *gc.C) {
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ManifoldSuite) TestConfigValidationMissingClock(c *gc.C) {
	s.config.Clock = nil
	err := s.config.Validate()
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, "missing Clock not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingMachineLock(c *gc.C) {
	s.config.MachineLock = nil
	err := s.config.Validate()
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, "missing MachineLock not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingLogger(c *gc.C) {
	s.config.Logger = nil
	err := s.config.Validate()
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, "missing Logger not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingModelType(c *gc.C) {
	s.config.ModelType = ""
	err := s.config.Validate()
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, "missing model type not valid")
}

type fakeLock struct {
	machinelock.Lock
}
