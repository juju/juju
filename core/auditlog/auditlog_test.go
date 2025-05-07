// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auditlog_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/core/paths"
)

type AuditLogSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&AuditLogSuite{})

func (s *AuditLogSuite) TestAuditLogFile(c *tc.C) {
	dir := c.MkDir()
	logFile := auditlog.NewLogFile(dir, 300, 10)
	err := logFile.AddConversation(auditlog.Conversation{
		Who:            "deerhoof",
		What:           "gojira",
		When:           "2017-11-27T13:21:24Z",
		ModelName:      "admin/default",
		ConversationID: "0123456789abcdef",
		ConnectionID:   "AC1",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = logFile.AddRequest(auditlog.Request{
		ConversationID: "0123456789abcdef",
		ConnectionID:   "AC1",
		RequestID:      25,
		When:           "2017-12-12T11:34:56Z",
		Facade:         "Application",
		Method:         "Deploy",
		Version:        4,
		Args:           `{"applications": [{"application": "prometheus"}]}`,
	})
	c.Assert(err, tc.ErrorIsNil)
	err = logFile.AddResponse(auditlog.ResponseErrors{
		ConversationID: "0123456789abcdef",
		ConnectionID:   "AC1",
		RequestID:      25,
		When:           "2017-12-12T11:35:11Z",
		Errors: []*auditlog.Error{
			{Message: "oops", Code: "unauthorized access"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	err = logFile.Close()
	c.Assert(err, tc.ErrorIsNil)

	bytes, err := os.ReadFile(filepath.Join(dir, "audit.log"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(bytes), tc.Equals, expectedLogContents)
}

func (s *AuditLogSuite) TestAuditLogFilePriming(c *tc.C) {
	dir := c.MkDir()
	logFile := auditlog.NewLogFile(dir, 300, 10)
	err := logFile.Close()
	c.Assert(err, tc.ErrorIsNil)

	info, err := os.Stat(filepath.Join(dir, "audit.log"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info.Mode(), tc.Equals, paths.LogfilePermission)
	// The chown will only work when run as root.
}

func (s *AuditLogSuite) TestRecorder(c *tc.C) {
	var log fakeLog
	logTime, err := time.Parse(time.RFC3339, "2017-11-27T15:45:23Z")
	c.Assert(err, tc.ErrorIsNil)
	clock := testclock.NewClock(logTime)
	rec, err := auditlog.NewRecorder(&log, clock, auditlog.ConversationArgs{
		Who:          "wildbirds and peacedrums",
		What:         "Doubt/Hope",
		ModelName:    "admin/default",
		ConnectionID: 687,
	})
	c.Assert(err, tc.ErrorIsNil)
	clock.Advance(time.Second)
	err = rec.AddRequest(auditlog.RequestArgs{
		RequestID: 246,
		Facade:    "Death Vessel",
		Method:    "Horchata",
		Version:   5,
		Args:      `{"a": "something"}`,
	})
	c.Assert(err, tc.ErrorIsNil)
	clock.Advance(time.Second)
	err = rec.AddResponse(auditlog.ResponseErrorsArgs{
		RequestID: 246,
		Errors: []*auditlog.Error{{
			Message: "something bad",
			Code:    "bad request",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)

	log.stub.CheckCallNames(c, "AddConversation", "AddRequest", "AddResponse")
	calls := log.stub.Calls()
	rec0 := calls[0].Args[0].(auditlog.Conversation)
	callID := rec0.ConversationID
	c.Assert(rec0, tc.DeepEquals, auditlog.Conversation{
		Who:            "wildbirds and peacedrums",
		What:           "Doubt/Hope",
		When:           "2017-11-27T15:45:23Z",
		ModelName:      "admin/default",
		ConnectionID:   "2AF",
		ConversationID: callID,
	})
	c.Assert(calls[1].Args[0], tc.DeepEquals, auditlog.Request{
		ConversationID: callID,
		ConnectionID:   "2AF",
		RequestID:      246,
		When:           "2017-11-27T15:45:24Z",
		Facade:         "Death Vessel",
		Method:         "Horchata",
		Version:        5,
		Args:           `{"a": "something"}`,
	})
	c.Assert(calls[2].Args[0], tc.DeepEquals, auditlog.ResponseErrors{
		ConversationID: callID,
		ConnectionID:   "2AF",
		RequestID:      246,
		When:           "2017-11-27T15:45:25Z",
		Errors: []*auditlog.Error{{
			Message: "something bad",
			Code:    "bad request",
		}},
	})
}

type fakeLog struct {
	stub testing.Stub
}

func (l *fakeLog) AddConversation(m auditlog.Conversation) error {
	l.stub.AddCall("AddConversation", m)
	return l.stub.NextErr()
}

func (l *fakeLog) AddRequest(m auditlog.Request) error {
	l.stub.AddCall("AddRequest", m)
	return l.stub.NextErr()
}

func (l *fakeLog) AddResponse(m auditlog.ResponseErrors) error {
	l.stub.AddCall("AddResponse", m)
	return l.stub.NextErr()
}

func (l *fakeLog) Close() error {
	l.stub.AddCall("Close")
	return l.stub.NextErr()
}

var (
	expectedLogContents = `
{"conversation":{"who":"deerhoof","what":"gojira","when":"2017-11-27T13:21:24Z","model-name":"admin/default","model-uuid":"","conversation-id":"0123456789abcdef","connection-id":"AC1"}}
{"request":{"conversation-id":"0123456789abcdef","connection-id":"AC1","request-id":25,"when":"2017-12-12T11:34:56Z","facade":"Application","method":"Deploy","version":4,"args":"{\"applications\": [{\"application\": \"prometheus\"}]}"}}
{"errors":{"conversation-id":"0123456789abcdef","connection-id":"AC1","request-id":25,"when":"2017-12-12T11:35:11Z","errors":[{"message":"oops","code":"unauthorized access"}]}}
`[1:]
)
