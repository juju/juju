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
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/worker/multiwatcher"
)

type WorkerSuite struct {
	statetesting.StateSuite
	logger loggo.Logger
	config multiwatcher.Config
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	s.logger = loggo.GetLogger("test")
	s.logger.SetLogLevel(loggo.TRACE)

	allWatcherBacking, err := state.NewAllWatcherBacking(s.StatePool)
	c.Assert(err, jc.ErrorIsNil)
	s.config = multiwatcher.Config{
		Clock:                clock.WallClock,
		Logger:               s.logger,
		Backing:              allWatcherBacking,
		PrometheusRegisterer: noopRegisterer{},
	}
}

func (s *WorkerSuite) TestConfigMissingClock(c *gc.C) {
	s.config.Clock = nil
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "missing Clock not valid")
}

func (s *WorkerSuite) TestConfigMissingLogger(c *gc.C) {
	s.config.Logger = nil
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "missing Logger not valid")
}

func (s *WorkerSuite) TestConfigMissingBacking(c *gc.C) {
	s.config.Backing = nil
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "missing Backing not valid")
}

func (s *WorkerSuite) TestConfigMissingRegisterer(c *gc.C) {
	s.config.PrometheusRegisterer = nil
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "missing PrometheusRegisterer not valid")
}
