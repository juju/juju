// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"regexp"
	"time"

	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cert"
	agentcmd "github.com/juju/juju/cmd/jujud/agent"
	"github.com/juju/juju/cmd/jujud/agent/agenttest"
	"github.com/juju/juju/standards/rfc5424/rfc5424test"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/logsender"
)

type syslogSuite struct {
	agenttest.AgentSuite
	server   *rfc5424test.Server
	logsCh   logsender.LogRecordCh
	received chan rfc5424test.Message
}

var _ = gc.Suite(&syslogSuite{})

func (s *syslogSuite) SetUpTest(c *gc.C) {
	s.AgentSuite.SetUpTest(c)

	s.received = make(chan rfc5424test.Message, 1)
	s.server = rfc5424test.NewServer(rfc5424test.HandlerFunc(func(msg rfc5424test.Message) {
		select {
		case s.received <- msg:
		default:
		}
	}))
	s.AddCleanup(func(*gc.C) { s.server.Close() })

	serverCert, err := tls.X509KeyPair(
		[]byte(coretesting.ServerCert),
		[]byte(coretesting.ServerKey),
	)
	c.Assert(err, jc.ErrorIsNil)
	caCert, err := cert.ParseCert(coretesting.CACert)
	c.Assert(err, jc.ErrorIsNil)
	clientCAs := x509.NewCertPool()
	clientCAs.AddCert(caCert)
	s.server.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    clientCAs,
	}
	s.server.StartTLS()

	err = s.State.UpdateModelConfig(map[string]interface{}{
		"syslog-host":        s.server.Listener.Addr().String(),
		"syslog-server-cert": coretesting.ServerCert,
		"syslog-ca-cert":     coretesting.CACert,
		"syslog-client-cert": coretesting.ServerCert,
		"syslog-client-key":  coretesting.ServerKey,
	}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	s.logsCh, err = logsender.InstallBufferedLogWriter(1000)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *syslogSuite) newRecord(msg string) *logsender.LogRecord {
	return &logsender.LogRecord{
		Time:     time.Now(),
		Module:   "juju.featuretests.syslog",
		Location: "syslog_test.go:99999",
		Level:    loggo.ERROR,
		Message:  msg,
	}
}

func (s *syslogSuite) sendRecord(c *gc.C, rec *logsender.LogRecord) {
	select {
	case s.logsCh <- rec:
	case <-time.After(coretesting.LongWait):
		c.Fatal(`timed out "sending" message`)
	}
}

func (s *syslogSuite) popMessagesUntil(c *gc.C, expected string) rfc5424test.Message {
	re, err := regexp.Compile(expected)
	c.Assert(err, jc.ErrorIsNil)

	c.Logf("popping messages")
	for {
		msg := s.nextMessage(c)
		c.Logf("message: %+v", msg)
		if re.MatchString(msg.Message) {
			return msg
		}
	}
}

func (s *syslogSuite) nextMessage(c *gc.C) rfc5424test.Message {
	select {
	case msg, ok := <-s.received:
		c.Assert(ok, jc.IsTrue)
		return msg
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for message to be forwarded")
	}
	return rfc5424test.Message{}
}

func (s *syslogSuite) TestLogRecordForwarded(c *gc.C) {
	// Create a machine and an agent for it.
	m, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: agent.BootstrapNonce,
		Jobs:  []state.MachineJob{state.JobManageModel},
	})

	s.PrimeAgent(c, m.Tag(), password)
	agentConf := agentcmd.NewAgentConf(s.DataDir())
	agentConf.ReadConfig(m.Tag().String())

	machineAgentFactory := agentcmd.MachineAgentFactoryFn(agentConf, s.logsCh, c.MkDir())
	a := machineAgentFactory(m.Id())

	// Ensure there's no logs to begin with.
	// Start the agent.
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer a.Stop()

	// Pop off all initial log records.
	s.sendRecord(c, s.newRecord("<stop here>"))
	s.popMessagesUntil(c, `.*<stop here>`)

	// Ensure that a specific log record gets forwarded.
	rec := s.newRecord("something happened!")
	rec.Time = time.Date(2015, time.June, 1, 23, 2, 1, 23, time.UTC)
	s.sendRecord(c, rec)
	msg := s.popMessagesUntil(c, `something happened!`)
	expected := `<11>1 2015-06-01T23:02:01.000000023Z machine-0.%s jujud-machine-agent-%s - - [origin enterpriseID="28978" sofware="jujud-machine-agent" swVersion="%s"][model@28978 controller-uuid="%s" model-uuid="%s"][log@28978 module="juju.featuretests.syslog" source="syslog_test.go:99999"] something happened!`
	modelID := coretesting.ModelTag.Id()
	ctlrID := modelID
	c.Check(msg.Message, gc.Equals, fmt.Sprintf(expected, modelID, modelID[:28], version.Current, ctlrID, modelID))
}
