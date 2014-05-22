// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mock_test

import (
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/container/kvm"
	"launchpad.net/juju-core/container/kvm/mock"
	"launchpad.net/juju-core/testing"
)

type MockSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&MockSuite{})

func (*MockSuite) TestListInitiallyEmpty(c *gc.C) {
	factory := mock.MockFactory()
	containers, err := factory.List()
	c.Assert(err, gc.IsNil)
	c.Assert(containers, gc.HasLen, 0)
}

func (*MockSuite) TestNewContainersInList(c *gc.C) {
	factory := mock.MockFactory()
	added := []kvm.Container{}
	added = append(added, factory.New("first"))
	added = append(added, factory.New("second"))
	containers, err := factory.List()
	c.Assert(err, gc.IsNil)
	c.Assert(containers, jc.SameContents, added)
}

func (*MockSuite) TestContainers(c *gc.C) {
	factory := mock.MockFactory()
	container := factory.New("first")
	c.Assert(container.Name(), gc.Equals, "first")
	c.Assert(container.IsRunning(), jc.IsFalse)
}

func (*MockSuite) TestContainerStoppingStoppedErrors(c *gc.C) {
	factory := mock.MockFactory()
	container := factory.New("first")
	err := container.Stop()
	c.Assert(err, gc.ErrorMatches, "container is not running")
}

func (*MockSuite) TestContainerStartStarts(c *gc.C) {
	factory := mock.MockFactory()
	container := factory.New("first")
	err := container.Start(kvm.StartParams{})
	c.Assert(err, gc.IsNil)
	c.Assert(container.IsRunning(), jc.IsTrue)
}

func (*MockSuite) TestContainerStartingRunningErrors(c *gc.C) {
	factory := mock.MockFactory()
	container := factory.New("first")
	err := container.Start(kvm.StartParams{})
	c.Assert(err, gc.IsNil)
	err = container.Start(kvm.StartParams{})
	c.Assert(err, gc.ErrorMatches, "container is already running")
}

func (*MockSuite) TestContainerStoppingRunningStops(c *gc.C) {
	factory := mock.MockFactory()
	container := factory.New("first")
	err := container.Start(kvm.StartParams{})
	c.Assert(err, gc.IsNil)
	err = container.Stop()
	c.Assert(err, gc.IsNil)
	c.Assert(container.IsRunning(), jc.IsFalse)
}

func (*MockSuite) TestAddListener(c *gc.C) {
	listener := make(chan mock.Event)
	factory := mock.MockFactory()
	factory.AddListener(listener)
	c.Assert(factory.HasListener(listener), jc.IsTrue)
}

func (*MockSuite) TestRemoveFirstListener(c *gc.C) {
	factory := mock.MockFactory()
	first := make(chan mock.Event)
	factory.AddListener(first)
	second := make(chan mock.Event)
	factory.AddListener(second)
	third := make(chan mock.Event)
	factory.AddListener(third)
	factory.RemoveListener(first)
	c.Assert(factory.HasListener(first), jc.IsFalse)
	c.Assert(factory.HasListener(second), jc.IsTrue)
	c.Assert(factory.HasListener(third), jc.IsTrue)
}

func (*MockSuite) TestRemoveMiddleListener(c *gc.C) {
	factory := mock.MockFactory()
	first := make(chan mock.Event)
	factory.AddListener(first)
	second := make(chan mock.Event)
	factory.AddListener(second)
	third := make(chan mock.Event)
	factory.AddListener(third)
	factory.RemoveListener(second)
	c.Assert(factory.HasListener(first), jc.IsTrue)
	c.Assert(factory.HasListener(second), jc.IsFalse)
	c.Assert(factory.HasListener(third), jc.IsTrue)
}

func (*MockSuite) TestRemoveLastListener(c *gc.C) {
	factory := mock.MockFactory()
	first := make(chan mock.Event)
	factory.AddListener(first)
	second := make(chan mock.Event)
	factory.AddListener(second)
	third := make(chan mock.Event)
	factory.AddListener(third)
	factory.RemoveListener(third)
	c.Assert(factory.HasListener(first), jc.IsTrue)
	c.Assert(factory.HasListener(second), jc.IsTrue)
	c.Assert(factory.HasListener(third), jc.IsFalse)
}

func (*MockSuite) TestEvents(c *gc.C) {
	factory := mock.MockFactory()
	listener := make(chan mock.Event, 5)
	factory.AddListener(listener)

	first := factory.New("first")
	second := factory.New("second")
	first.Start(kvm.StartParams{})
	second.Start(kvm.StartParams{})
	second.Stop()
	first.Stop()

	c.Assert(<-listener, gc.Equals, mock.Event{mock.Started, "first"})
	c.Assert(<-listener, gc.Equals, mock.Event{mock.Started, "second"})
	c.Assert(<-listener, gc.Equals, mock.Event{mock.Stopped, "second"})
	c.Assert(<-listener, gc.Equals, mock.Event{mock.Stopped, "first"})
}

func (*MockSuite) TestEventsGoToAllListeners(c *gc.C) {
	factory := mock.MockFactory()
	first := make(chan mock.Event, 5)
	factory.AddListener(first)
	second := make(chan mock.Event, 5)
	factory.AddListener(second)

	container := factory.New("container")
	container.Start(kvm.StartParams{})
	container.Stop()

	c.Assert(<-first, gc.Equals, mock.Event{mock.Started, "container"})
	c.Assert(<-second, gc.Equals, mock.Event{mock.Started, "container"})
	c.Assert(<-first, gc.Equals, mock.Event{mock.Stopped, "container"})
	c.Assert(<-second, gc.Equals, mock.Event{mock.Stopped, "container"})
}
