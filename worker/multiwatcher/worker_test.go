// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package multiwatcher_test

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/multiwatcher"
)

type WorkerSuite struct {
	testing.BaseSuite
	logger loggo.Logger
	config multiwatcher.Config
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.logger = loggo.GetLogger("test")
	s.logger.SetLogLevel(loggo.TRACE)

	s.config = multiwatcher.Config{
		Clock:                clock.WallClock,
		Logger:               s.logger,
		Backing:              noopWatcherBacking{},
		PrometheusRegisterer: noopRegisterer{},
	}
}

func (s *WorkerSuite) TestConfigMissingClock(c *gc.C) {
	s.config.Clock = nil
	err := s.config.Validate()
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, "missing Clock not valid")
}

func (s *WorkerSuite) TestConfigMissingLogger(c *gc.C) {
	s.config.Logger = nil
	err := s.config.Validate()
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, "missing Logger not valid")
}

func (s *WorkerSuite) TestConfigMissingBacking(c *gc.C) {
	s.config.Backing = nil
	err := s.config.Validate()
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, "missing Backing not valid")
}

func (s *WorkerSuite) TestConfigMissingRegisterer(c *gc.C) {
	s.config.PrometheusRegisterer = nil
	err := s.config.Validate()
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, "missing PrometheusRegisterer not valid")
}

type noopWatcherBacking struct {
	state.AllWatcherBacking
}
