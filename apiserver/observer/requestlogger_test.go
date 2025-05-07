// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package observer_test

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/apiserver/observer"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type RequestLoggerSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&RequestLoggerSuite{})

func (s *RequestLoggerSuite) TestAgentLoginWritesLog(c *tc.C) {
	notifier, logger := s.makeNotifier(c)

	agent := names.NewMachineTag("42")
	model := names.NewModelTag("fake-uuid")
	notifier.Login(context.Background(), agent, model, "abc", false, "user data")

	c.Assert(logger.entries, tc.SameContents, []string{
		`INFO: connection agent login: machine-42 for fake-uuid`,
	})
}

func (s *RequestLoggerSuite) TestUserConnectionsNoLogs(c *tc.C) {
	notifier, logger := s.makeNotifier(c)

	user := names.NewUserTag("bob")
	model := names.NewModelTag("fake-uuid")
	notifier.Login(context.Background(), user, model, "abc", false, "user data")

	c.Assert(logger.entries, tc.HasLen, 0)
}

func (s *RequestLoggerSuite) TestControllerMachineAgentConnectionNoLogs(c *tc.C) {
	s.assertControllerAgentConnectionNoLogs(c, names.NewMachineTag("2"))
}

func (s *RequestLoggerSuite) TestControllerUnitAgentConnectionNoLogs(c *tc.C) {
	s.assertControllerAgentConnectionNoLogs(c, names.NewUnitTag("mariadb/0"))
}

func (s *RequestLoggerSuite) TestControllerApplicationAgentConnectionNoLogs(c *tc.C) {
	s.assertControllerAgentConnectionNoLogs(c, names.NewApplicationTag("gitlab"))
}

func (s *RequestLoggerSuite) TestMachineAgentConnectionLogs(c *tc.C) {
	s.assertAgentConnectionLogs(c, names.NewMachineTag("2"))
}

func (s *RequestLoggerSuite) TestUnitAgentConnectionLogs(c *tc.C) {
	s.assertAgentConnectionLogs(c, names.NewUnitTag("mariadb/0"))
}

func (s *RequestLoggerSuite) TestApplicationAgentConnectionLogs(c *tc.C) {
	s.assertAgentConnectionLogs(c, names.NewApplicationTag("gitlab"))
}

func (s *RequestLoggerSuite) TestAgentDisconnectionLogs(c *tc.C) {
	notifier, logger := s.makeNotifier(c)

	agent := names.NewMachineTag("42")
	model := names.NewModelTag("fake-uuid")
	// All details are saved from Login.
	notifier.Login(context.Background(), agent, model, "abc", false, "user data")
	notifier.Leave(context.Background())

	c.Assert(logger.entries, tc.HasLen, 3)

	// Ignore the last log entry, which is about connection termination.
	c.Check(logger.entries[:2], tc.DeepEquals, []string{
		"INFO: connection agent login: machine-42 for fake-uuid",
		"INFO: connection agent disconnected: machine-42 for fake-uuid",
	})
}

func (s *RequestLoggerSuite) TestControllerAgentDisconnectionLogs(c *tc.C) {
	notifier, logger := s.makeNotifier(c)

	agent := names.NewMachineTag("42")
	model := names.NewModelTag("fake-uuid")
	// All details are saved from Login.
	notifier.Login(context.Background(), agent, model, "abc", true, "user data")
	notifier.Leave(context.Background())

	c.Assert(logger.entries, tc.HasLen, 1)
}

func (s *RequestLoggerSuite) TestUserDisconnectionNoLogs(c *tc.C) {
	notifier, logger := s.makeNotifier(c)

	agent := names.NewUserTag("bob")
	model := names.NewModelTag("fake-uuid")
	// All details are saved from Login.
	notifier.Login(context.Background(), agent, model, "abc", true, "user data")
	notifier.Leave(context.Background())

	c.Assert(logger.entries, tc.HasLen, 1)
}

func (s *RequestLoggerSuite) assertControllerAgentConnectionNoLogs(c *tc.C, agent names.Tag) {
	notifier, logger := s.makeNotifier(c)

	model := names.NewModelTag("fake-uuid")
	notifier.Login(context.Background(), agent, model, "abc", true, "user data")

	c.Assert(logger.entries, tc.HasLen, 0)
}

func (s *RequestLoggerSuite) assertAgentConnectionLogs(c *tc.C, agent names.Tag) {
	notifier, logger := s.makeNotifier(c)

	model := names.NewModelTag("fake-uuid")
	notifier.Login(context.Background(), agent, model, "abc", false, "user data")

	c.Assert(logger.entries, tc.HasLen, 1)
	c.Check(logger.entries[0], tc.Matches, fmt.Sprintf(`INFO: connection agent login: %s for fake-uuid`, agent.String()))
}

func (*RequestLoggerSuite) makeNotifier(c *tc.C) (*observer.RequestLogger, *testLogger) {
	testLogger := &testLogger{}
	recorder := loggertesting.RecordLog(func(s string, a ...interface{}) {
		if len(a) != 1 {
			panic("unexpected number of arguments")
		}
		switch v := a[0].(type) {
		case []interface{}:
			a = v
		default:
			panic("unexpected type")
		}
		testLogger.entries = append(testLogger.entries, fmt.Sprintf(s, a...))
	})
	return observer.NewRequestLogger(observer.RequestLoggerConfig{
		Clock:  testclock.NewClock(time.Now()),
		Logger: loggertesting.WrapCheckLog(recorder),
	}), testLogger
}

type testLogger struct {
	entries []string
}
