// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rfc5424test_test

import (
	"net"
	"time"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/standards/rfc5424"
	"github.com/juju/juju/standards/rfc5424/rfc5424test"
	"github.com/juju/juju/testing"
)

type ServerSuite struct {
	gitjujutesting.IsolationSuite
}

var _ = gc.Suite(&ServerSuite{})

func (s *ServerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *ServerSuite) TestSend(c *gc.C) {
	received := make(chan rfc5424test.Message, 1)
	server := rfc5424test.NewServer(rfc5424test.HandlerFunc(func(msg rfc5424test.Message) {
		received <- msg
	}))
	server.Start()
	defer server.Close()

	cfg := rfc5424.ClientConfig{
		MaxSize:     8192,
		SendTimeout: time.Minute,
	}
	var clientAddr net.Addr
	netDial := func(network, address string) (rfc5424.Conn, error) {
		conn, err := net.Dial(network, address)
		clientAddr = conn.LocalAddr()
		return conn, err
	}
	client, err := rfc5424.Open(server.Listener.Addr().String(), cfg, netDial)
	c.Assert(err, jc.ErrorIsNil)

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
			fakeStructuredDataElement{"sde0"},
			fakeStructuredDataElement{"sde1"},
		},
		Msg: "a message",
	}

	err = client.Send(msg)
	c.Assert(err, jc.ErrorIsNil)
	err = client.Close()
	c.Assert(err, jc.ErrorIsNil)

	select {
	case msg, ok := <-received:
		c.Assert(ok, jc.IsTrue)
		c.Assert(msg.RemoteAddr, gc.Equals, clientAddr.String())
		c.Assert(msg.Message, gc.Equals, `<28>1 1970-01-01T15:05:21.000000123Z a.b.org an-app 119 xyz... [sde0 abc="123" def="456"][sde1 abc="123" def="456"] a message`)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for message")
	}
}

type fakeStructuredDataElement struct {
	id rfc5424.StructuredDataName
}

func (f fakeStructuredDataElement) ID() rfc5424.StructuredDataName {
	return f.id
}

func (fakeStructuredDataElement) Params() []rfc5424.StructuredDataParam {
	return []rfc5424.StructuredDataParam{
		{"abc", "123"},
		{"def", "456"},
	}
}

func (fakeStructuredDataElement) Validate() error {
	return nil
}
