// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package presence_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/pubsub"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	"github.com/kr/pretty"
	gc "gopkg.in/check.v1"

	corepresence "github.com/juju/juju/core/presence"
	"github.com/juju/juju/pubsub/apiserver"
	"github.com/juju/juju/pubsub/centralhub"
	"github.com/juju/juju/pubsub/forwarder"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/presence"
)

type PresenceSuite struct {
	testing.IsolationSuite
	server   string
	hub      *pubsub.StructuredHub
	clock    *testclock.Clock
	recorder corepresence.Recorder
	config   presence.WorkerConfig
}

var _ = gc.Suite(&PresenceSuite{})

func (s *PresenceSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.hub = centralhub.New(ourTag)
	s.clock = testclock.NewClock(time.Time{})
	s.recorder = corepresence.New(s.clock)
	s.recorder.Enable()
	s.config = presence.WorkerConfig{
		Origin:   ourServer,
		Hub:      s.hub,
		Recorder: s.recorder,
		Logger:   loggo.GetLogger("test"),
	}
	loggo.ConfigureLoggers("<root>=trace")
}

func (s *PresenceSuite) worker(c *gc.C) worker.Worker {
	w, err := presence.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *PresenceSuite) TestWorkerConfigMissingOrigin(c *gc.C) {
	s.config.Origin = ""
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "missing origin not valid")
}

func (s *PresenceSuite) TestWorkerConfigMissingHub(c *gc.C) {
	s.config.Hub = nil
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "missing hub not valid")
}

func (s *PresenceSuite) TestWorkerConfigMissingRecorder(c *gc.C) {
	s.config.Recorder = nil
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "missing recorder not valid")
}

func (s *PresenceSuite) TestWorkerConfigMissingLogger(c *gc.C) {
	s.config.Logger = nil
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "missing logger not valid")
}

func (s *PresenceSuite) TestNewWorkerValidatesConfig(c *gc.C) {
	w, err := presence.NewWorker(presence.WorkerConfig{})
	c.Check(err, gc.ErrorMatches, "missing origin not valid")
	c.Check(w, gc.IsNil)
}

func (s *PresenceSuite) TestWorkerDies(c *gc.C) {
	w := s.worker(c)
	workertest.CleanKill(c, w)
}

func (s *PresenceSuite) TestReport(c *gc.C) {
	w := s.worker(c)
	defer workertest.CleanKill(c, w)

	s.recorder.Connect("machine-0", "model-uuid", "agent", 1, false, "")
	s.recorder.Connect("machine-0", "model-uuid", "agent", 2, false, "")
	s.recorder.Connect("machine-0", "model-uuid", "agent", 3, false, "")
	s.recorder.Connect("machine-1", "model-uuid", "agent", 4, false, "")
	s.recorder.Connect("machine-1", "model-uuid", "agent", 5, false, "")
	s.recorder.Connect("machine-2", "model-uuid", "agent", 6, false, "")

	reporter, ok := w.(worker.Reporter)
	c.Assert(ok, jc.IsTrue)
	c.Assert(reporter.Report(), jc.DeepEquals, map[string]interface{}{
		"machine-0": 3,
		"machine-1": 2,
		"machine-2": 1,
	})
}

func (s *PresenceSuite) TestForwarderConnectToOther(c *gc.C) {
	w := s.worker(c)
	defer workertest.CleanKill(c, w)

	done := make(chan struct{})

	unsub, err := s.hub.Subscribe(apiserver.PresenceRequestTopic, func(topic string, data apiserver.OriginTarget, err error) {
		c.Logf("handler called for %q", topic)
		c.Check(err, jc.ErrorIsNil)
		c.Check(data.Target, gc.Equals, otherServer)
		c.Check(data.Origin, gc.Equals, ourServer)
		close(done)
	})
	c.Assert(err, jc.ErrorIsNil)
	defer unsub()

	// When connections are established from us to them, we ask for their presence info.
	_, err = s.hub.Publish(
		forwarder.ConnectedTopic,
		apiserver.OriginTarget{Origin: ourServer, Target: otherServer})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertDone(c, done)
}

func (s *PresenceSuite) TestForwarderConnectFromOther(c *gc.C) {
	w := s.worker(c)
	defer workertest.CleanKill(c, w)

	done := make(chan struct{})

	unsub, err := s.hub.Subscribe(apiserver.PresenceRequestTopic, func(topic string, data apiserver.OriginTarget, err error) {
		c.Logf("handler called for %q", topic)
		c.Check(err, jc.ErrorIsNil)
		c.Check(data.Target, gc.Equals, otherServer)
		c.Check(data.Origin, gc.Equals, ourServer)
		close(done)
	})
	c.Assert(err, jc.ErrorIsNil)
	defer unsub()

	// When connections are established from them to us, we ask for their presence info.
	_, err = s.hub.Publish(
		forwarder.ConnectedTopic,
		apiserver.OriginTarget{Origin: otherServer, Target: ourServer})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertDone(c, done)
}

func (s *PresenceSuite) TestForwarderConnectOtherIgnored(c *gc.C) {
	w := s.worker(c)
	defer workertest.CleanKill(c, w)

	called := make(chan struct{})

	unsub, err := s.hub.Subscribe(apiserver.PresenceRequestTopic, func(topic string, data apiserver.OriginTarget, err error) {
		c.Logf("handler called for %q", topic)
		close(called)
	})
	c.Assert(err, jc.ErrorIsNil)
	defer unsub()

	_, err = s.hub.Publish(
		forwarder.ConnectedTopic,
		apiserver.OriginTarget{Origin: otherServer, Target: "machine-8"})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertNotCalled(c, called)
}

func (s *PresenceSuite) TestForwarderDisconnectConnectFromOther(c *gc.C) {
	w := s.worker(c)
	defer workertest.CleanKill(c, w)

	connect(s.recorder, agent1, agent2)

	done, err := s.hub.Publish(
		forwarder.DisconnectedTopic,
		apiserver.OriginTarget{Origin: ourServer, Target: otherServer})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertDone(c, done)
	s.AssertConnections(c, alive(agent1), missing(agent2))
}

func (s *PresenceSuite) TestForwarderDisconnectOthersIgnored(c *gc.C) {
	w := s.worker(c)
	defer workertest.CleanKill(c, w)

	connect(s.recorder, agent1, agent2)

	done, err := s.hub.Publish(
		forwarder.DisconnectedTopic,
		apiserver.OriginTarget{Origin: "machine-7", Target: otherServer})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertDone(c, done)
	s.AssertConnections(c, alive(agent1), alive(agent2))
}

func (s *PresenceSuite) TestConnectTopic(c *gc.C) {
	w := s.worker(c)
	defer workertest.CleanKill(c, w)

	done, err := s.hub.Publish(
		apiserver.ConnectTopic,
		apiserver.APIConnection{
			Origin:          "machine-5",
			ModelUUID:       "model-uuid",
			AgentTag:        "agent-2",
			ConnectionID:    42,
			ControllerAgent: true,
			UserData:        "test",
		})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertDone(c, done)
	s.AssertConnections(c, corepresence.Value{
		Model:           "model-uuid",
		Server:          "machine-5",
		Agent:           "agent-2",
		ConnectionID:    42,
		Status:          corepresence.Alive,
		ControllerAgent: true,
		UserData:        "test",
	})
}

func (s *PresenceSuite) TestDisconnectTopic(c *gc.C) {
	w := s.worker(c)
	defer workertest.CleanKill(c, w)

	connect(s.recorder, agent1, agent2)

	done, err := s.hub.Publish(
		apiserver.DisconnectTopic,
		apiserver.APIConnection{
			Origin:       agent2.Server,
			ConnectionID: agent2.ConnectionID,
		})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertDone(c, done)
	s.AssertConnections(c, alive(agent1))
}

func (s *PresenceSuite) TestPresenceRequest(c *gc.C) {
	w := s.worker(c)
	defer workertest.CleanKill(c, w)

	connect(s.recorder, agent1, agent2, agent3, agent4)

	done := make(chan struct{})
	unsub, err := s.hub.Subscribe(apiserver.PresenceResponseTopic, func(topic string, data apiserver.PresenceResponse, err error) {
		c.Logf("handler called for %q", topic)
		c.Check(err, jc.ErrorIsNil)
		c.Check(data.Origin, gc.Equals, ourServer)

		c.Check(data.Connections, gc.HasLen, 2)
		s.CheckConnection(c, data.Connections[0], agent1)
		s.CheckConnection(c, data.Connections[1], agent3)

		close(done)
	})
	c.Assert(err, jc.ErrorIsNil)
	defer unsub()

	// When asked for our presence, we respond with the agents connected to us.
	_, err = s.hub.Publish(
		apiserver.PresenceRequestTopic,
		apiserver.OriginTarget{Origin: otherServer, Target: ourServer})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertDone(c, done)
}

func (s *PresenceSuite) TestPresenceRequestOtherServer(c *gc.C) {
	w := s.worker(c)
	defer workertest.CleanKill(c, w)

	called := make(chan struct{})
	unsub, err := s.hub.Subscribe(apiserver.PresenceResponseTopic, func(topic string, data apiserver.PresenceResponse, err error) {
		c.Logf("handler called for %q", topic)
		close(called)
	})
	c.Assert(err, jc.ErrorIsNil)
	defer unsub()

	// When presence requests come in for other servers, we ignore them.
	_, err = s.hub.Publish(
		apiserver.PresenceRequestTopic,
		apiserver.OriginTarget{Origin: otherServer, Target: "another"})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertNotCalled(c, called)
}

func (s *PresenceSuite) TestPresenceResponse(c *gc.C) {
	w := s.worker(c)
	defer workertest.CleanKill(c, w)

	connect(s.recorder, agent1, agent2, agent3, agent4)
	s.recorder.ServerDown(otherServer)

	// When connections information comes from other servers, we update our recorder.
	done, err := s.hub.Publish(
		apiserver.PresenceResponseTopic,
		apiserver.PresenceResponse{
			Origin: otherServer,
			Connections: []apiserver.APIConnection{
				apiConn(agent2), apiConn(agent4),
			},
		})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertDone(c, done)

	s.AssertConnections(c, alive(agent1), alive(agent2), alive(agent3), alive(agent4))
}

func (s *PresenceSuite) AssertDone(c *gc.C, called <-chan struct{}) {
	select {
	case <-called:
	case <-time.After(coretesting.LongWait):
		c.Fatal("event not handled")
	}
}

func (s *PresenceSuite) AssertNotCalled(c *gc.C, called <-chan struct{}) {
	select {
	case <-called:
		c.Fatal("event called unexpectedly")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *PresenceSuite) AssertConnections(c *gc.C, values ...corepresence.Value) {
	connections := s.recorder.Connections()
	c.Log(pretty.Sprint(connections))
	c.Assert(connections.Values(), jc.SameContents, values)
}

func (s *PresenceSuite) CheckConnection(c *gc.C, conn apiserver.APIConnection, agent corepresence.Value) {
	c.Check(conn.AgentTag, gc.Equals, agent.Agent)
	c.Check(conn.ControllerAgent, gc.Equals, agent.ControllerAgent)
	c.Check(conn.ModelUUID, gc.Equals, agent.Model)
	c.Check(conn.ConnectionID, gc.Equals, agent.ConnectionID)
	c.Check(conn.Origin, gc.Equals, agent.Server)
	c.Check(conn.UserData, gc.Equals, agent.UserData)
}

func apiConn(value corepresence.Value) apiserver.APIConnection {
	return apiserver.APIConnection{
		AgentTag:        value.Agent,
		ControllerAgent: value.ControllerAgent,
		ModelUUID:       value.Model,
		ConnectionID:    value.ConnectionID,
		Origin:          value.Server,
		UserData:        value.UserData,
	}
}

func alive(v corepresence.Value) corepresence.Value {
	v.Status = corepresence.Alive
	return v
}

func missing(v corepresence.Value) corepresence.Value {
	v.Status = corepresence.Missing
	return v
}

func connect(r corepresence.Recorder, values ...corepresence.Value) {
	for _, info := range values {
		r.Connect(info.Server, info.Model, info.Agent, info.ConnectionID, info.ControllerAgent, info.UserData)
	}
}

const modelUUID = "model-uuid"

var (
	ourTag      = names.NewMachineTag("1")
	ourServer   = ourTag.String()
	otherServer = "machine-2"
	agent1      = corepresence.Value{
		Model:        modelUUID,
		Server:       ourServer,
		Agent:        "machine-0",
		ConnectionID: 1237,
		UserData:     "foo",
	}
	agent2 = corepresence.Value{
		Model:        modelUUID,
		Server:       otherServer,
		Agent:        "machine-1",
		ConnectionID: 1238,
		UserData:     "bar",
	}
	agent3 = corepresence.Value{
		Model:        modelUUID,
		Server:       ourServer,
		Agent:        "unit-ubuntu-0",
		ConnectionID: 1239,
		UserData:     "baz",
	}
	agent4 = corepresence.Value{
		Model:        modelUUID,
		Server:       otherServer,
		Agent:        "unit-ubuntu-1",
		ConnectionID: 1240,
		UserData:     "splat",
	}
)
