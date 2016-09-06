// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"regexp"
	"runtime"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/rfc/rfc5424/rfc5424test"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/os"
	"github.com/juju/utils/series"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cert"
	agentcmd "github.com/juju/juju/cmd/jujud/agent"
	"github.com/juju/juju/cmd/jujud/agent/agenttest"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/logsender"
	"github.com/juju/juju/worker/peergrouper"
)

type syslogSuite struct {
	agenttest.AgentSuite
	logsCh          logsender.LogRecordCh
	received        chan rfc5424test.Message
	fakeEnsureMongo *agenttest.FakeEnsureMongo
}

var _ = gc.Suite(&syslogSuite{})

func (s *syslogSuite) SetUpSuite(c *gc.C) {
	s.LoggingSuite.SetUpSuite(c)
	// Tailing logs requires a replica set. Restart mongo with a
	// replicaset before initialising AgentSuite.
	mongod := gitjujutesting.MgoServer
	mongod.Params = []string{"--replSet", "juju"}
	mongod.Restart()

	info := mongod.DialInfo()
	args := peergrouper.InitiateMongoParams{
		DialInfo:       info,
		MemberHostPort: mongod.Addr(),
	}
	err := peergrouper.InitiateMongoServer(args)
	c.Assert(err, jc.ErrorIsNil)

	s.AgentSuite.SetUpSuite(c)
	s.AddCleanup(func(*gc.C) {
		mongod.Params = nil
		mongod.Restart()
	})
}

func (s *syslogSuite) createSyslogServer(c *gc.C, received chan rfc5424test.Message, done chan struct{}) string {
	server := rfc5424test.NewServer(rfc5424test.HandlerFunc(func(msg rfc5424test.Message) {
		select {
		case received <- msg:
		case <-done:
		}
	}))
	s.AddCleanup(func(*gc.C) { server.Close() })
	s.AddCleanup(func(*gc.C) { close(done) })

	serverCert, err := tls.X509KeyPair(
		[]byte(coretesting.ServerCert),
		[]byte(coretesting.ServerKey),
	)
	c.Assert(err, jc.ErrorIsNil)
	caCert, err := cert.ParseCert(coretesting.CACert)
	c.Assert(err, jc.ErrorIsNil)
	clientCAs := x509.NewCertPool()
	clientCAs.AddCert(caCert)
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    clientCAs,
	}
	server.StartTLS()

	// We must use "localhost", as the certificate does not
	// have any IP SANs.
	port := server.Listener.Addr().(*net.TCPAddr).Port
	addr := net.JoinHostPort("localhost", fmt.Sprint(port))
	return addr
}

func (s *syslogSuite) SetUpTest(c *gc.C) {
	if runtime.GOOS != "linux" {
		c.Skip(fmt.Sprintf("this test requires a controller, therefore does not support %q", runtime.GOOS))
	}
	currentSeries := series.HostSeries()
	osFromSeries, err := series.GetOSFromSeries(currentSeries)
	c.Assert(err, jc.ErrorIsNil)
	if osFromSeries != os.Ubuntu {
		c.Skip(fmt.Sprintf("this test requires a controller, therefore does not support OS %q only Ubuntu", osFromSeries.String()))
	}
	s.AgentSuite.SetUpTest(c)
	// TODO(perrito666) 200160701:
	// This needs to be done to stop the test from trying to install mongo
	// while running, but it is a huge footprint for such little benefit.
	// This test should not need JujuConnSuite or AgentSuite.
	s.fakeEnsureMongo = agenttest.InstallFakeEnsureMongo(s)

	done := make(chan struct{})
	s.received = make(chan rfc5424test.Message)
	addr := s.createSyslogServer(c, s.received, done)

	// Leave log forwarding disabled initially, it will be enabled
	// via a model config update in the test.
	err = s.State.UpdateModelConfig(map[string]interface{}{
		"syslog-host":        addr,
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

func (s *syslogSuite) popMessagesUntil(c *gc.C, expected string, received chan rfc5424test.Message) rfc5424test.Message {
	re, err := regexp.Compile(expected)
	c.Assert(err, jc.ErrorIsNil)

	c.Logf("popping messages")
	for {
		msg := s.nextMessage(c, received)
		c.Logf("message: %+v", msg)
		if re.MatchString(msg.Message) {
			return msg
		}
	}
}

func (s *syslogSuite) nextMessage(c *gc.C, received chan rfc5424test.Message) rfc5424test.Message {
	select {
	case msg, ok := <-received:
		c.Assert(ok, jc.IsTrue)
		return msg
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for message to be forwarded")
	}
	return rfc5424test.Message{}
}

func (s *syslogSuite) assertLogRecordForwarded(c *gc.C, received chan rfc5424test.Message) {
	// Pop off all initial log records.
	s.sendRecord(c, s.newRecord("<stop here>"))
	s.popMessagesUntil(c, `.*<stop here>`, received)

	// Ensure that a specific log record gets forwarded.
	rec := s.newRecord("something happened!")
	rec.Time = time.Date(2099, time.June, 1, 23, 2, 1, 23, time.UTC)
	s.sendRecord(c, rec)
	msg := s.popMessagesUntil(c, `something happened!`, received)
	expected := `<11>1 2099-06-01T23:02:01.000000023Z machine-0.%s jujud-machine-agent-%s - - [origin enterpriseID="28978" sofware="jujud-machine-agent" swVersion="%s"][model@28978 controller-uuid="%s" model-uuid="%s"][log@28978 module="juju.featuretests.syslog" source="syslog_test.go:99999"] something happened!`
	modelID := coretesting.ModelTag.Id()
	ctlrID := coretesting.ControllerTag.Id()
	c.Check(msg.Message, gc.Equals, fmt.Sprintf(expected, modelID, modelID[:28], version.Current, ctlrID, modelID))
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

	err := s.State.UpdateModelConfig(map[string]interface{}{
		"logforward-enabled": true,
	}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	s.assertLogRecordForwarded(c, s.received)
}

func (s *syslogSuite) TestConfigChange(c *gc.C) {
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

	done := make(chan struct{})
	received := make(chan rfc5424test.Message)
	addr := s.createSyslogServer(c, received, done)

	err := s.State.UpdateModelConfig(map[string]interface{}{
		"logforward-enabled": true,
		"syslog-host":        addr,
		"syslog-ca-cert":     coretesting.CACert,
		"syslog-client-cert": coretesting.ServerCert,
		"syslog-client-key":  coretesting.ServerKey,
	}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertLogRecordForwarded(c, received)
}
