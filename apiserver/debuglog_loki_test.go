// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"testing"
	"time"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type debugLogLokiSuite struct {
	coretesting.BaseSuite
}

func TestDebugLogLokiSuite(t *testing.T) {
	tc.Run(t, &debugLogLokiSuite{})
}

// recordingSocket is a test debugLogSocket that records the records and
// errors it is asked to send.
type recordingSocket struct {
	ok      bool
	errs    []error
	records []*params.LogMessage
}

func (s *recordingSocket) sendOk()             { s.ok = true }
func (s *recordingSocket) sendError(err error) { s.errs = append(s.errs, err) }
func (s *recordingSocket) sendLogRecord(record *params.LogMessage, _ int) error {
	s.records = append(s.records, record)
	return nil
}

func (s *debugLogLokiSuite) TestSendLokiForwardingNotice(c *tc.C) {
	socket := &recordingSocket{}

	sendLokiForwardingNotice(socket, 2)

	c.Check(socket.ok, tc.IsTrue)
	c.Check(socket.records, tc.HasLen, 1)
	rec := socket.records[0]
	c.Check(rec.Message, tc.Equals, lokiForwardingNoticeMessage)
	c.Check(rec.Severity, tc.Equals, "INFO")
	c.Check(rec.Module, tc.Equals, "juju.apiserver")
}

func (s *debugLogLokiSuite) TestLokiForwardingNoticeRecord(c *tc.C) {
	now := time.Now()
	rec := lokiForwardingNoticeRecord(now)
	c.Check(rec.Timestamp, tc.Equals, now)
	c.Check(rec.Entity, tc.Equals, "controller")
	c.Check(rec.Severity, tc.Equals, "INFO")
	c.Check(rec.Message, tc.Equals, lokiForwardingNoticeMessage)
}
