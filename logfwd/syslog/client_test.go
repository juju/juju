// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslog_test

import (
	"net"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/logfwd"
	"github.com/juju/juju/logfwd/syslog"
	"github.com/juju/juju/standards/rfc5424"
	"github.com/juju/juju/standards/rfc5424/sdelements"
	"github.com/juju/juju/standards/tls"
	coretesting "github.com/juju/juju/testing"
)

type ClientSuite struct {
	testing.IsolationSuite

	stub   *testing.Stub
	sender *stubSender
}

var _ = gc.Suite(&ClientSuite{})

func (s *ClientSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.sender = &stubSender{stub: s.stub}
}

func (s *ClientSuite) TestOpen(c *gc.C) {
	cfg := syslog.RawConfig{
		Host:               "a.b.c:9876",
		ExpectedServerCert: validCert2,
		ClientCACert:       coretesting.CACert,
		ClientCert:         coretesting.ServerCert,
		ClientKey:          coretesting.ServerKey,
	}
	senderOpener := &stubSenderOpener{
		stub:       s.stub,
		ReturnOpen: s.sender,
	}

	client, err := syslog.OpenForSender(cfg, senderOpener)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "DialFunc", "Open")
	tlsConfig := tls.Config{
		RawCert: tls.RawCert{
			CertPEM:   coretesting.ServerCert,
			KeyPEM:    coretesting.ServerKey,
			CACertPEM: coretesting.CACert,
		},
		ExpectedServerCertPEM: validCert2,
	}
	s.stub.CheckCall(c, 0, "DialFunc", tlsConfig, time.Duration(0))
	c.Check(client.Sender, gc.Equals, s.sender)
}

func (s *ClientSuite) TestClose(c *gc.C) {
	client := syslog.Client{Sender: s.sender}

	err := client.Close()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Close")
}

func (s *ClientSuite) TestSendLogFull(c *gc.C) {
	tag := names.NewMachineTag("99")
	cID := "9f484882-2f18-4fd2-967d-db9663db7bea"
	mID := "deadbeef-2f18-4fd2-967d-db9663db7bea"
	ver := version.MustParse("1.2.3")
	ts := time.Unix(12345, 0)
	rec := logfwd.LogRecord{
		BaseRecord: logfwd.BaseRecord{
			Origin:    logfwd.OriginForMachineAgent(tag, cID, mID, ver),
			Timestamp: time.Unix(12345, 0),
			Message:   "(╯°□°)╯︵ ┻━┻",
		},
		Level: loggo.ERROR,
		Location: logfwd.SourceLocation{
			Module:   "juju.x.y",
			Filename: "x/y/spam.go",
			Line:     42,
		},
	}
	client := syslog.Client{Sender: s.sender}

	err := client.Send(rec)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Send")
	s.stub.CheckCall(c, 0, "Send", rfc5424.Message{
		Header: rfc5424.Header{
			Priority: rfc5424.Priority{
				Severity: rfc5424.SeverityError,
				Facility: rfc5424.FacilityUser,
			},
			Timestamp: rfc5424.Timestamp{ts},
			Hostname: rfc5424.Hostname{
				FQDN: "machine-99.deadbeef-2f18-4fd2-967d-db9663db7bea",
			},
			AppName: "jujud-deadbeef-2f18-4fd2-967d-db9663db7bea",
		},
		StructuredData: rfc5424.StructuredData{
			&sdelements.Origin{
				EnterpriseID: sdelements.OriginEnterpriseID{
					Number: 28978,
				},
				SoftwareName:    "jujud-machine-agent",
				SoftwareVersion: ver,
			},
			&sdelements.Private{
				Name: "model",
				PEN:  28978,
				Data: []rfc5424.StructuredDataParam{{
					Name:  "controller-uuid",
					Value: "9f484882-2f18-4fd2-967d-db9663db7bea",
				}, {
					Name:  "model-uuid",
					Value: "deadbeef-2f18-4fd2-967d-db9663db7bea",
				}},
			},
			&sdelements.Private{
				Name: "log",
				PEN:  28978,
				Data: []rfc5424.StructuredDataParam{{
					Name:  "module",
					Value: "juju.x.y",
				}, {
					Name:  "source",
					Value: "x/y/spam.go:42",
				}},
			},
		},
		Msg: "(╯°□°)╯︵ ┻━┻",
	})
}

func (s *ClientSuite) TestSendLogLevels(c *gc.C) {
	tag := names.NewMachineTag("99")
	cID := "9f484882-2f18-4fd2-967d-db9663db7bea"
	mID := "deadbeef-2f18-4fd2-967d-db9663db7bea"
	ver := version.MustParse("1.2.3")
	rec := logfwd.LogRecord{
		BaseRecord: logfwd.BaseRecord{
			Origin:    logfwd.OriginForMachineAgent(tag, cID, mID, ver),
			Timestamp: time.Unix(12345, 0),
			Message:   "(╯°□°)╯︵ ┻━┻",
		},
		Level: loggo.ERROR,
		Location: logfwd.SourceLocation{
			Module:   "juju.x.y",
			Filename: "x/y/spam.go",
			Line:     42,
		},
	}
	client := syslog.Client{Sender: s.sender}

	levels := map[loggo.Level]rfc5424.Severity{
		loggo.ERROR:   rfc5424.SeverityError,
		loggo.WARNING: rfc5424.SeverityWarning,
		loggo.INFO:    rfc5424.SeverityInformational,
		loggo.DEBUG:   rfc5424.SeverityDebug,
		loggo.TRACE:   rfc5424.SeverityDebug,
	}
	for level, expected := range levels {
		c.Logf("trying %s -> %s", level, expected)
		s.stub.ResetCalls()
		rec.Level = level

		err := client.Send(rec)
		c.Assert(err, jc.ErrorIsNil)

		msg := s.stub.Calls()[0].Args[0].(rfc5424.Message)
		c.Check(msg.Severity, gc.Equals, expected)
	}
}

func (s *ClientSuite) TestSendAudit(c *gc.C) {
	tag := names.NewUserTag("bob")
	cID := "9f484882-2f18-4fd2-967d-db9663db7bea"
	mID := "deadbeef-2f18-4fd2-967d-db9663db7bea"
	ver := version.MustParse("1.2.3")
	origin, err := logfwd.OriginForJuju(tag, cID, mID, ver)
	c.Assert(err, jc.ErrorIsNil)
	origin.Hostname = "10.0.0.1"
	ts := time.Unix(12345, 0)
	rec := logfwd.AuditRecord{
		BaseRecord: logfwd.BaseRecord{
			Origin:    origin,
			Timestamp: ts,
			Message:   "(╯°□°)╯︵ ┻━┻",
		},
		Audit: logfwd.Audit{
			Operation: "Model-1:Get",
			Args:      map[string]string{"x": "y"},
		},
	}
	client := syslog.Client{Sender: s.sender}

	err = client.Send(rec)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Send")
	s.stub.CheckCall(c, 0, "Send", rfc5424.Message{
		Header: rfc5424.Header{
			Priority: rfc5424.Priority{
				Severity: rfc5424.SeverityInformational,
				Facility: rfc5424.FacilityUser,
			},
			Timestamp: rfc5424.Timestamp{ts},
			Hostname: rfc5424.Hostname{
				StaticIP: net.ParseIP("10.0.0.1"),
			},
			AppName: "juju-deadbeef-2f18-4fd2-967d-db9663db7bea",
		},
		StructuredData: rfc5424.StructuredData{
			&sdelements.Origin{
				EnterpriseID: sdelements.OriginEnterpriseID{
					Number: 28978,
				},
				SoftwareName:    "juju",
				SoftwareVersion: ver,
			},
			&sdelements.Private{
				Name: "model",
				PEN:  28978,
				Data: []rfc5424.StructuredDataParam{{
					Name:  "controller-uuid",
					Value: "9f484882-2f18-4fd2-967d-db9663db7bea",
				}, {
					Name:  "model-uuid",
					Value: "deadbeef-2f18-4fd2-967d-db9663db7bea",
				}},
			},
			&sdelements.Private{
				Name: "audit",
				PEN:  28978,
				Data: []rfc5424.StructuredDataParam{{
					Name:  "origin-type",
					Value: "user",
				}, {
					Name:  "origin-name",
					Value: "bob",
				}, {
					Name:  "operation",
					Value: "Model-1:Get",
				}, {
					Name:  "x",
					Value: "y",
				}},
			},
		},
		Msg: "(╯°□°)╯︵ ┻━┻",
	})
}

type stubSenderOpener struct {
	stub *testing.Stub

	ReturnDialFunc rfc5424.DialFunc
	ReturnOpen     syslog.Sender
}

func (s *stubSenderOpener) DialFunc(cfg tls.Config, timeout time.Duration) (rfc5424.DialFunc, error) {
	s.stub.AddCall("DialFunc", cfg, timeout)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}

	dial := s.ReturnDialFunc
	if dial == nil {
		dial = func(network, address string) (rfc5424.Conn, error) {
			s.stub.AddCall("dial", network, address)
			if err := s.stub.NextErr(); err != nil {
				return nil, err
			}

			return nil, nil
		}
	}
	return dial, nil
}

func (s *stubSenderOpener) Open(host string, cfg rfc5424.ClientConfig, dial rfc5424.DialFunc) (syslog.Sender, error) {
	s.stub.AddCall("Open", host, cfg, dial)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}

	return s.ReturnOpen, nil
}

type stubSender struct {
	stub *testing.Stub
}

func (s *stubSender) Send(msg rfc5424.Message) error {
	s.stub.AddCall("Send", msg)
	if err := s.stub.NextErr(); err != nil {
		return err
	}

	return nil
}

func (s *stubSender) Close() error {
	s.stub.AddCall("Close")
	if err := s.stub.NextErr(); err != nil {
		return err
	}

	return nil
}
