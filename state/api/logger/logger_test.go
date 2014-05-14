// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger_test

import (
	gc "launchpad.net/gocheck"

	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/logger"
	"launchpad.net/juju-core/state/testing"
)

type loggerSuite struct {
	jujutesting.JujuConnSuite

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine
	rawCharm   *state.Charm
	rawService *state.Service
	rawUnit    *state.Unit

	logger *logger.State
}

var _ = gc.Suite(&loggerSuite{})

func (s *loggerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	var stateAPI *api.State
	stateAPI, s.rawMachine = s.OpenAPIAsNewMachine(c)
	// Create the logger facade.
	s.logger = stateAPI.Logger()
	c.Assert(s.logger, gc.NotNil)
}

func (s *loggerSuite) TestLoggingConfigWrongMachine(c *gc.C) {
	config, err := s.logger.LoggingConfig("42")
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(config, gc.Equals, "")
}

func (s *loggerSuite) TestLoggingConfig(c *gc.C) {
	config, err := s.logger.LoggingConfig(s.rawMachine.Tag())
	c.Assert(err, gc.IsNil)
	c.Assert(config, gc.Not(gc.Equals), "")
}

func (s *loggerSuite) setLoggingConfig(c *gc.C, loggingConfig string) {
	err := s.BackingState.UpdateEnvironConfig(map[string]interface{}{"logging-config": loggingConfig}, nil, nil)
	c.Assert(err, gc.IsNil)
}

func (s *loggerSuite) TestWatchLoggingConfig(c *gc.C) {
	watcher, err := s.logger.WatchLoggingConfig(s.rawMachine.Tag())
	c.Assert(err, gc.IsNil)
	defer testing.AssertStop(c, watcher)
	wc := testing.NewNotifyWatcherC(c, s.BackingState, watcher)
	// Initial event
	wc.AssertOneChange()

	loggingConfig := "<root>=WARN;juju.log.test=DEBUG"
	s.setLoggingConfig(c, loggingConfig)
	// One change noticing the new version
	wc.AssertOneChange()
	// Setting the version to the same value doesn't trigger a change
	s.setLoggingConfig(c, loggingConfig)
	wc.AssertNoChange()

	loggingConfig = loggingConfig + ";wibble=DEBUG"
	s.setLoggingConfig(c, loggingConfig)
	wc.AssertOneChange()
	testing.AssertStop(c, watcher)
	wc.AssertClosed()
}
