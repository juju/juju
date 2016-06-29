// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"crypto/tls"
	"crypto/x509"
	"time"

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
	"github.com/juju/juju/worker/logsender"
)

type syslogSuite struct {
	agenttest.AgentSuite
	server   *rfc5424test.Server
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
}

func (s *syslogSuite) TestAuditLogForwarded(c *gc.C) {
	// Create a machine and an agent for it.
	m, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: agent.BootstrapNonce,
		Jobs:  []state.MachineJob{state.JobManageModel},
	})

	s.PrimeAgent(c, m.Tag(), password)
	agentConf := agentcmd.NewAgentConf(s.DataDir())
	agentConf.ReadConfig(m.Tag().String())

	logsCh, err := logsender.InstallBufferedLogWriter(1000)
	c.Assert(err, jc.ErrorIsNil)
	machineAgentFactory := agentcmd.MachineAgentFactoryFn(agentConf, logsCh, c.MkDir())
	a := machineAgentFactory(m.Id())

	// Ensure there's no logs to begin with.
	// Start the agent.
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer a.Stop()

	select {
	case msg, ok := <-s.received:
		c.Assert(ok, jc.IsTrue)
		c.Logf("message: %+v", msg)
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for message to be forwarded")
	}
}
