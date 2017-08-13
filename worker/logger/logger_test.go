// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger_test

import (
	"time"

	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	worker "gopkg.in/juju/worker.v1"

	apilogger "github.com/juju/juju/api/logger"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/logger"
)

// worstCase is used for timeouts when timing out
// will fail the test. Raising this value should
// not affect the overall running time of the tests
// unless they fail.
const worstCase = 5 * time.Second

type LoggerSuite struct {
	testing.JujuConnSuite

	loggerAPI *apilogger.State
	machine   *state.Machine

	value    string
	override string
}

var _ = gc.Suite(&LoggerSuite{})

func (s *LoggerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	apiConn, machine := s.OpenAPIAsNewMachine(c)
	// Create the machiner API facade.
	s.loggerAPI = apilogger.NewState(apiConn)
	c.Assert(s.loggerAPI, gc.NotNil)
	s.machine = machine
	s.value = ""
	s.override = ""
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

func (s *LoggerSuite) makeLogger(c *gc.C) worker.Worker {
	w, err := logger.NewLogger(s.loggerAPI, s.machine.Tag(), s.override, func(v string) error {
		s.value = v
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *LoggerSuite) TestRunStop(c *gc.C) {
	loggingWorker := s.makeLogger(c)
	c.Assert(worker.Stop(loggingWorker), gc.IsNil)
}

func (s *LoggerSuite) TestInitialState(c *gc.C) {
	config, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	expected := config.LoggingConfig()

	initial := "<root>=DEBUG;wibble=ERROR"
	c.Assert(expected, gc.Not(gc.Equals), initial)

	loggo.DefaultContext().ResetLoggerLevels()
	err = loggo.ConfigureLoggers(initial)
	c.Assert(err, jc.ErrorIsNil)

	loggingWorker := s.makeLogger(c)
	defer worker.Stop(loggingWorker)

	s.waitLoggingInfo(c, expected)
	c.Check(s.value, gc.Equals, expected)
}

func (s *LoggerSuite) TestConfigOverride(c *gc.C) {
	s.override = "test=TRACE"

	loggo.DefaultContext().ResetLoggerLevels()
	err := loggo.ConfigureLoggers("<root>=DEBUG;wibble=ERROR")
	c.Assert(err, jc.ErrorIsNil)

	loggingWorker := s.makeLogger(c)
	defer worker.Stop(loggingWorker)

	// When reset, the root defaults to WARNING.
	expected := "<root>=WARNING;test=TRACE"
	s.waitLoggingInfo(c, expected)
}
