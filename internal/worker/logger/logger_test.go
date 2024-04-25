// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	gc "gopkg.in/check.v1"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	internallogger "github.com/juju/juju/internal/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/logger"
)

type LoggerSuite struct {
	testing.IsolationSuite

	context   corelogger.LoggerContext
	agent     names.Tag
	loggerAPI *mockAPI
	config    logger.WorkerConfig

	value string
}

var _ = gc.Suite(&LoggerSuite{})

func (s *LoggerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.context = internallogger.WrapLoggoContext(loggo.NewContext(loggo.DEBUG))
	s.agent = names.NewMachineTag("42")
	s.loggerAPI = &mockAPI{
		config:  s.context.Config().String(),
		watcher: &mockNotifyWatcher{},
	}
	s.config = logger.WorkerConfig{
		Context: s.context,
		API:     s.loggerAPI,
		Tag:     s.agent,
		Logger:  loggertesting.WrapCheckLog(c),
		Callback: func(v string) error {
			s.value = v
			return nil
		},
	}
	s.value = ""
}

func (s *LoggerSuite) TestMissingContext(c *gc.C) {
	s.config.Context = nil
	w, err := logger.NewLogger(s.config)
	c.Assert(w, gc.IsNil)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), gc.Equals, "missing logging context not valid")
}

func (s *LoggerSuite) TestMissingAPI(c *gc.C) {
	s.config.API = nil
	w, err := logger.NewLogger(s.config)
	c.Assert(w, gc.IsNil)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), gc.Equals, "missing api not valid")
}

func (s *LoggerSuite) TestMissingLogger(c *gc.C) {
	s.config.Logger = nil
	w, err := logger.NewLogger(s.config)
	c.Assert(w, gc.IsNil)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), gc.Equals, "missing logger not valid")
}

func (s *LoggerSuite) waitLoggingInfo(c *gc.C, expected string) {
	timeout := time.After(testing.LongWait)
	for {
		select {
		case <-timeout:
			c.Fatalf("timeout while waiting for logging info to change")
		case <-time.After(10 * time.Millisecond):
			loggerInfo := s.context.Config().String()
			if loggerInfo != expected {
				c.Logf("logging is %q, still waiting", loggerInfo)
				continue
			}
			return
		}
	}
}

func (s *LoggerSuite) makeLogger(c *gc.C) worker.Worker {
	w, err := logger.NewLogger(s.config)
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *LoggerSuite) TestRunStop(c *gc.C) {
	loggingWorker := s.makeLogger(c)
	c.Assert(worker.Stop(loggingWorker), gc.IsNil)
}

func (s *LoggerSuite) TestInitialState(c *gc.C) {
	expected := s.context.Config().String()

	initial := "<root>=DEBUG;wibble=ERROR"
	c.Assert(expected, gc.Not(gc.Equals), initial)

	s.context.ResetLoggerLevels()
	err := s.context.ConfigureLoggers(initial)
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
	s.config.Override = "test=TRACE"

	s.context.ResetLoggerLevels()
	err := s.context.ConfigureLoggers("<root>=DEBUG;wibble=ERROR")
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
