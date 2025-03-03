// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package observer_test

import (
	"context"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/observer"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type RequestObserverSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RequestObserverSuite{})

func (*RequestObserverSuite) makeNotifier(c *gc.C) *observer.RequestObserver {
	return observer.NewRequestObserver(observer.RequestObserverConfig{
		Clock:  testclock.NewClock(time.Now()),
		Logger: loggertesting.WrapCheckLog(c),
	})
}

func (s *RequestObserverSuite) TestAgentConnection(c *gc.C) {
	notifier := s.makeNotifier(c)

	agent := names.NewMachineTag("42")
	model := names.NewModelTag("fake-uuid")
	notifier.Login(context.Background(), agent, model, false, "user data")
}

func (s *RequestObserverSuite) assertControllerAgentConnection(c *gc.C, agent names.Tag) {
	notifier := s.makeNotifier(c)

	model := names.NewModelTag("fake-uuid")
	notifier.Login(context.Background(), agent, model, true, "user data")
}

func (s *RequestObserverSuite) TestControllerMachineAgentConnection(c *gc.C) {
	s.assertControllerAgentConnection(c, names.NewMachineTag("2"))
}

func (s *RequestObserverSuite) TestControllerUnitAgentConnection(c *gc.C) {
	s.assertControllerAgentConnection(c, names.NewUnitTag("mariadb/0"))
}

func (s *RequestObserverSuite) TestControllerApplicationAgentConnection(c *gc.C) {
	s.assertControllerAgentConnection(c, names.NewApplicationTag("gitlab"))
}

func (s *RequestObserverSuite) TestUserConnectionsNot(c *gc.C) {
	notifier := s.makeNotifier(c)

	user := names.NewUserTag("bob")
	model := names.NewModelTag("fake-uuid")
	notifier.Login(context.Background(), user, model, false, "user data")
}

func (s *RequestObserverSuite) TestAgentDisconnection(c *gc.C) {
	notifier := s.makeNotifier(c)

	agent := names.NewMachineTag("42")
	model := names.NewModelTag("fake-uuid")
	// All details are saved from Login.
	notifier.Login(context.Background(), agent, model, false, "user data")
	notifier.Leave(context.Background())
}

func (s *RequestObserverSuite) TestControllerAgentDisconnection(c *gc.C) {
	notifier := s.makeNotifier(c)

	agent := names.NewMachineTag("2")
	model := names.NewModelTag("fake-uuid")
	// All details are saved from Login.
	notifier.Login(context.Background(), agent, model, true, "user data")
	notifier.Leave(context.Background())
}

func (s *RequestObserverSuite) TestUserDisconnections(c *gc.C) {
	notifier := s.makeNotifier(c)

	user := names.NewUserTag("bob")
	model := names.NewModelTag("fake-uuid")
	// All details are saved from Login.
	notifier.Login(context.Background(), user, model, false, "user data")
	notifier.Leave(context.Background())
}
