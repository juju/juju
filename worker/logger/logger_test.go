// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger_test

import (
	"time"

	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/logger"
)

// worstCase is used for timeouts when timing out
// will fail the test. Raising this value should
// not affect the overall running time of the tests
// unless they fail.
const worstCase = 5 * time.Second

type LoggerSuite struct {
	testing.IsolationSuite

	loggerAPI *mockAPI
	agent     names.Tag

	value    string
	override string
}

var _ = gc.Suite(&LoggerSuite{})

func (s *LoggerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.loggerAPI = &mockAPI{
		// IsolationSuite setup resets logging info, so just grab that.
		config:  loggo.LoggerInfo(),
		watcher: &mockNotifyWatcher{},
	}
	s.agent = names.NewMachineTag("42")
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
	w, err := logger.NewLogger(s.loggerAPI, s.agent, s.override, func(v string) error {
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
	expected := s.loggerAPI.config

	initial := "<root>=DEBUG;wibble=ERROR"
	c.Assert(expected, gc.Not(gc.Equals), initial)

	loggo.DefaultContext().ResetLoggerLevels()
	err := loggo.ConfigureLoggers(initial)
	c.Assert(err, jc.ErrorIsNil)

	loggingWorker := s.makeLogger(c)
	s.waitLoggingInfo(c, expected)
	err = worker.Stop(loggingWorker)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.value, gc.Equals, expected)
	c.Check(s.loggerAPI.loggingTag, gc.Equals, s.agent)
	c.Check(s.loggerAPI.watchingTag, gc.Equals, s.agent)
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

type mockNotifyWatcher struct {
	changes chan struct{}
}

func (m *mockNotifyWatcher) Kill() {}

func (m *mockNotifyWatcher) Wait() error {
	return nil
}

func (m *mockNotifyWatcher) Changes() watcher.NotifyChannel {
	return m.changes
}

var _ watcher.NotifyWatcher = (*mockNotifyWatcher)(nil)

type mockAPI struct {
	watcher *mockNotifyWatcher
	config  string

	loggingTag  names.Tag
	watchingTag names.Tag
}

func (m *mockAPI) LoggingConfig(agentTag names.Tag) (string, error) {
	m.loggingTag = agentTag
	return m.config, nil
}

func (m *mockAPI) WatchLoggingConfig(agentTag names.Tag) (watcher.NotifyWatcher, error) {
	m.watchingTag = agentTag
	return m.watcher, nil
}
