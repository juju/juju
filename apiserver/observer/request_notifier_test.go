// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package observer_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/pubsub/apiserver"
)

type RequestObserverSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RequestObserverSuite{})

func (*RequestObserverSuite) makeNotifier(c *gc.C) (*observer.RequestObserver, *connectionHub) {
	hub := &connectionHub{c: c}
	return observer.NewRequestObserver(observer.RequestObserverContext{
		Clock:  testclock.NewClock(time.Now()),
		Hub:    hub,
		Logger: loggo.GetLogger("test"),
	}), hub
}

func (s *RequestObserverSuite) TestAgentConnectionPublished(c *gc.C) {
	notifier, hub := s.makeNotifier(c)

	agent := names.NewMachineTag("42")
	model := names.NewModelTag("fake-uuid")
	notifier.Login(agent, model, false, "user data")

	c.Assert(hub.called, gc.Equals, 1)
	c.Assert(hub.topic, gc.Equals, apiserver.ConnectTopic)
	c.Assert(hub.details, jc.DeepEquals, apiserver.APIConnection{
		AgentTag:     "machine-42",
		ModelUUID:    "fake-uuid",
		UserData:     "user data",
		ConnectionID: 0,
	})
}

func (s *RequestObserverSuite) assertControllerAgentConnectionPublished(c *gc.C, agent names.Tag) {
	notifier, hub := s.makeNotifier(c)

	model := names.NewModelTag("fake-uuid")
	notifier.Login(agent, model, true, "user data")

	c.Assert(hub.called, gc.Equals, 1)
	c.Assert(hub.topic, gc.Equals, apiserver.ConnectTopic)
	c.Assert(hub.details, jc.DeepEquals, apiserver.APIConnection{
		AgentTag:        agent.String(),
		ModelUUID:       "fake-uuid",
		ControllerAgent: true,
		UserData:        "user data",
		ConnectionID:    0,
	})
}

func (s *RequestObserverSuite) TestControllerMachineAgentConnectionPublished(c *gc.C) {
	s.assertControllerAgentConnectionPublished(c, names.NewMachineTag("2"))
}

func (s *RequestObserverSuite) TestControllerUnitAgentConnectionPublished(c *gc.C) {
	s.assertControllerAgentConnectionPublished(c, names.NewUnitTag("mariadb/0"))
}

func (s *RequestObserverSuite) TestControllerApplicationAgentConnectionPublished(c *gc.C) {
	s.assertControllerAgentConnectionPublished(c, names.NewApplicationTag("gitlab"))
}

func (s *RequestObserverSuite) TestUserConnectionsNotPublished(c *gc.C) {
	notifier, hub := s.makeNotifier(c)

	user := names.NewUserTag("bob")
	model := names.NewModelTag("fake-uuid")
	notifier.Login(user, model, false, "user data")

	c.Assert(hub.called, gc.Equals, 0)
}

func (s *RequestObserverSuite) TestAgentDisconnectionPublished(c *gc.C) {
	notifier, hub := s.makeNotifier(c)

	agent := names.NewMachineTag("42")
	model := names.NewModelTag("fake-uuid")
	// All details are saved from Login.
	notifier.Login(agent, model, false, "user data")
	notifier.Leave()

	c.Assert(hub.called, gc.Equals, 2)
	c.Assert(hub.topic, gc.Equals, apiserver.DisconnectTopic)
	c.Assert(hub.details, jc.DeepEquals, apiserver.APIConnection{
		AgentTag:     "machine-42",
		ModelUUID:    "fake-uuid",
		ConnectionID: 0,
	})
}

func (s *RequestObserverSuite) TestControllerAgentDisconnectionPublished(c *gc.C) {
	notifier, hub := s.makeNotifier(c)

	agent := names.NewMachineTag("2")
	model := names.NewModelTag("fake-uuid")
	// All details are saved from Login.
	notifier.Login(agent, model, true, "user data")
	notifier.Leave()

	c.Assert(hub.called, gc.Equals, 2)
	c.Assert(hub.topic, gc.Equals, apiserver.DisconnectTopic)
	c.Assert(hub.details, jc.DeepEquals, apiserver.APIConnection{
		AgentTag:        "machine-2",
		ModelUUID:       "fake-uuid",
		ControllerAgent: true,
		ConnectionID:    0,
	})
}

func (s *RequestObserverSuite) TestUserDisconnectionsNotPublished(c *gc.C) {
	notifier, hub := s.makeNotifier(c)

	user := names.NewUserTag("bob")
	model := names.NewModelTag("fake-uuid")
	// All details are saved from Login.
	notifier.Login(user, model, false, "user data")
	notifier.Leave()

	c.Assert(hub.called, gc.Equals, 0)
}

type connectionHub struct {
	c       *gc.C
	called  int
	topic   string
	details apiserver.APIConnection
}

func (hub *connectionHub) Publish(topic string, data interface{}) (<-chan struct{}, error) {
	hub.called++
	hub.topic = topic
	hub.details = data.(apiserver.APIConnection)
	return nil, nil
}
