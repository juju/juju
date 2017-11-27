// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package auditlog_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/auditlog"
)

type AuditLogSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&AuditLogSuite{})

func (s *AuditLogSuite) TestAuditLogFile(c *gc.C) {
	dir := c.MkDir()
	logFile := auditlog.NewLogFile(dir)
	err := logFile.AddCall(auditlog.Call{
		Who:          "deerhoof",
		What:         "gojira",
		When:         "2017-11-27T13:21:24Z",
		ModelName:    "admin/default",
		CallID:       "0123456789abcdef",
		ConnectionID: "AC1",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = logFile.AddRequest(auditlog.FacadeRequest{
		CallID:       "0123456789abcdef",
		ConnectionID: "AC1",
		RequestID:    25,
		Facade:       "Application",
		Method:       "Deploy",
		Version:      4,
		Args:         `{"applications": [{"application": "prometheus"}]}`,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = logFile.AddResponse(auditlog.FacadeResponse{
		CallID:       "0123456789abcdef",
		ConnectionID: "AC1",
		RequestID:    25,
		Errors: []*auditlog.Error{
			{Message: "oops", Code: "unauthorized access"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = logFile.Close()
	c.Assert(err, jc.ErrorIsNil)

	bytes, err := ioutil.ReadFile(filepath.Join(dir, "audit.log"))
	c.Assert(string(bytes), gc.Equals, expectedLogContents)
}

func (s *AuditLogSuite) TestAuditLogFilePriming(c *gc.C) {
	dir := c.MkDir()
	logFile := auditlog.NewLogFile(dir)
	err := logFile.Close()
	c.Assert(err, jc.ErrorIsNil)

	info, err := os.Stat(filepath.Join(dir, "audit.log"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Mode(), gc.Equals, os.FileMode(0600))
	// The chown will only work when run as root.
}

func (s *AuditLogSuite) TestRecorder(c *gc.C) {
	var log fakeLog
	logTime, err := time.Parse(time.RFC3339, "2017-11-27T15:45:23Z")
	c.Assert(err, jc.ErrorIsNil)
	rec, err := auditlog.NewRecorder(&log, auditlog.CallArgs{
		Who:          "wildbirds and peacedrums",
		What:         "Doubt/Hope",
		When:         logTime,
		ModelName:    "admin/default",
		ConnectionID: 687,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = rec.AddRequest(auditlog.RequestArgs{
		RequestID: 246,
		Facade:    "Death Vessel",
		Method:    "Horchata",
		Version:   5,
		Args:      `{"a": "something"}`,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = rec.AddResponse(auditlog.ResponseArgs{
		RequestID: 246,
		Errors: []*auditlog.Error{{
			Message: "something bad",
			Code:    "bad request",
		}},
	})

	log.stub.CheckCallNames(c, "AddCall", "AddRequest", "AddResponse")
	calls := log.stub.Calls()
	rec0 := calls[0].Args[0].(auditlog.Call)
	callID := rec0.CallID
	c.Assert(rec0, gc.DeepEquals, auditlog.Call{
		Who:          "wildbirds and peacedrums",
		What:         "Doubt/Hope",
		When:         "2017-11-27T15:45:23Z",
		ModelName:    "admin/default",
		ConnectionID: "2AF",
		CallID:       callID,
	})
	c.Assert(calls[1].Args[0], gc.DeepEquals, auditlog.FacadeRequest{
		CallID:       callID,
		ConnectionID: "2AF",
		RequestID:    246,
		Facade:       "Death Vessel",
		Method:       "Horchata",
		Version:      5,
		Args:         `{"a": "something"}`,
	})
	c.Assert(calls[2].Args[0], gc.DeepEquals, auditlog.FacadeResponse{
		CallID:       callID,
		ConnectionID: "2AF",
		RequestID:    246,
		Errors: []*auditlog.Error{{
			Message: "something bad",
			Code:    "bad request",
		}},
	})
}

type fakeLog struct {
	stub testing.Stub
}

func (l *fakeLog) AddCall(m auditlog.Call) error {
	l.stub.AddCall("AddCall", m)
	return l.stub.NextErr()
}

func (l *fakeLog) AddRequest(m auditlog.FacadeRequest) error {
	l.stub.AddCall("AddRequest", m)
	return l.stub.NextErr()
}

func (l *fakeLog) AddResponse(m auditlog.FacadeResponse) error {
	l.stub.AddCall("AddResponse", m)
	return l.stub.NextErr()
}

func (l *fakeLog) Close() error {
	l.stub.AddCall("Close")
	return l.stub.NextErr()
}

var (
	expectedLogContents = `
{"call":{"who":"deerhoof","what":"gojira","when":"2017-11-27T13:21:24Z","model-name":"admin/default","model-uuid":"","call-id":"0123456789abcdef","connection-id":"AC1"}}
{"request":{"call-id":"0123456789abcdef","connection-id":"AC1","request-id":25,"facade":"Application","method":"Deploy","version":4,"args":"{\"applications\": [{\"application\": \"prometheus\"}]}"}}
{"response":{"call-id":"0123456789abcdef","connection-id":"AC1","request-id":25,"errors":[{"message":"oops","code":"unauthorized access"}]}}
`[1:]
)
