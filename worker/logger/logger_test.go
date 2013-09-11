// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger_test

import (
	"time"

	gc "launchpad.net/gocheck"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	apilogger "launchpad.net/juju-core/state/api/logger"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/logger"
)

// worstCase is used for timeouts when timing out
// will fail the test. Raising this value should
// not affect the overall running time of the tests
// unless they fail.
const worstCase = 5 * time.Second

type LoggerSuite struct {
	testing.JujuConnSuite

	apiRoot   *api.State
	loggerApi *apilogger.State
	machine   *state.Machine
}

var _ = gc.Suite(&LoggerSuite{})

func (s *LoggerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.apiRoot, s.machine = s.OpenAPIAsNewMachine(c)

	// Create the machiner API facade.
	s.loggerApi = s.apiRoot.Logger()
	c.Assert(s.loggerApi, gc.NotNil)
}

func (s *LoggerSuite) TearDownTest(c *gc.C) {
	err := s.apiRoot.Close()
	c.Assert(err, gc.IsNil)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *LoggerSuite) waitLoggingInfo(c *gc.C, expected string) {
	timeout := time.After(worstCase)
	for {
		select {
		case <-timeout:
			c.Fatalf("timeout while waiting for logging info to change")
		case <-time.After(10 * time.Millisecond):
			loggerInfo := loggo.LoggerInfo()
			if loggerInfo != expected {
				c.Logf("logging is %q, still waiting", loggerInfo)
				continue
			}
			return
		}
	}
}

type mockConfig struct {
	agent.Config
	tag string
}

func (mock *mockConfig) Tag() string {
	return mock.tag
}

func agentConfig(tag string) agent.Config {
	return &mockConfig{tag: tag}
}

func (s *LoggerSuite) makeLogger() worker.Worker {
	return logger.NewLogger(s.loggerApi, agentConfig(s.machine.Tag()))
}

func (s *LoggerSuite) TestRunStop(c *gc.C) {
	loggingWorker := s.makeLogger()
	c.Assert(worker.Stop(loggingWorker), gc.IsNil)
}

func (s *LoggerSuite) TestInitialState(c *gc.C) {
	config, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	expected := config.LoggingConfig()

	initial := "<root>=DEBUG;wibble=ERROR"
	c.Assert(expected, gc.Not(gc.Equals), initial)

	loggo.ResetLoggers()
	err = loggo.ConfigureLoggers(initial)
	c.Assert(err, gc.IsNil)

	loggingWorker := s.makeLogger()
	defer worker.Stop(loggingWorker)

	s.waitLoggingInfo(c, expected)
}
