// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rfc5424_test

import (
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/standards/rfc5424"
)

type ClientSuite struct {
	testing.IsolationSuite

	stub       *testing.Stub
	conn       *stubConn
	returnDial rfc5424.Conn
}

var _ = gc.Suite(&ClientSuite{})

func (s *ClientSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.conn = &stubConn{stub: s.stub}
	s.returnDial = s.conn
}

func (s *ClientSuite) dial(network, address string) (rfc5424.Conn, error) {
	s.stub.AddCall("dial", network, address)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}

	return s.returnDial, nil
}

func (s *ClientSuite) TestOpen(c *gc.C) {
	var cfg rfc5424.ClientConfig

	_, err := rfc5424.Open("a.b.c:1234", cfg, s.dial)

	c.Check(err, jc.ErrorIsNil)
	s.stub.CheckCallNames(c, "dial")
	s.stub.CheckCall(c, 0, "dial", "tcp", "a.b.c:1234")
}

func (s *ClientSuite) TestClose(c *gc.C) {
	var cfg rfc5424.ClientConfig
	client, err := rfc5424.Open("a.b.c:1234", cfg, s.dial)
	c.Assert(err, jc.ErrorIsNil)
	s.stub.ResetCalls()

	err = client.Close()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Close")
}

func (s *ClientSuite) TestSend(c *gc.C) {
	cfg := rfc5424.ClientConfig{
		MaxSize:     8192,
		SendTimeout: 10 * time.Minute,
	}
	client, err := rfc5424.Open("a.b.c:1234", cfg, s.dial)
	c.Assert(err, jc.ErrorIsNil)
	s.stub.ResetCalls()
	msg := rfc5424.Message{
		Header: rfc5424.Header{
			Priority: rfc5424.Priority{
				Severity: rfc5424.SeverityWarning,
				Facility: rfc5424.FacilityDaemon,
			},
			Timestamp: rfc5424.Timestamp{time.Unix(54321, 123).UTC()},
			Hostname:  rfc5424.Hostname{FQDN: "a.b.org"},
			AppName:   "an-app",
			ProcID:    "119",
			MsgID:     "xyz...",
		},
		StructuredData: rfc5424.StructuredData{
			newStubElement(&testing.Stub{}, "spam", "x=y"),
		},
		Msg: "a message",
	}

	err = client.Send(msg)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "SetWriteDeadline", "Write")
}

type stubConn struct {
	stub *testing.Stub

	ReturnWrite int
}

func (s *stubConn) Close() error {
	s.stub.AddCall("Close")
	if err := s.stub.NextErr(); err != nil {
		return err
	}

	return nil
}

func (s *stubConn) Write(data []byte) (int, error) {
	s.stub.AddCall("Write", string(data))
	if err := s.stub.NextErr(); err != nil {
		return 0, err
	}

	return s.ReturnWrite, nil
}

func (s *stubConn) SetWriteDeadline(d time.Time) error {
	s.stub.AddCall("SetWriteDeadline", d)
	if err := s.stub.NextErr(); err != nil {
		return err
	}

	return nil
}
