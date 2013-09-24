// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mock_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/container/kvm"
	"launchpad.net/juju-core/container/kvm/mock"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

type MockSuite struct {
	testbase.LoggingSuite
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
	err := container.Start()
	c.Assert(err, gc.IsNil)
	c.Assert(container.IsRunning(), jc.IsTrue)
}

func (*MockSuite) TestContainerStartingRunningErrors(c *gc.C) {
	factory := mock.MockFactory()
	container := factory.New("first")
	err := container.Start()
	c.Assert(err, gc.IsNil)
	err = container.Start()
	c.Assert(err, gc.ErrorMatches, "container is already running")
}

func (*MockSuite) TestContainerStoppingRunningStops(c *gc.C) {
	factory := mock.MockFactory()
	container := factory.New("first")
	err := container.Start()
	c.Assert(err, gc.IsNil)
	err = container.Stop()
	c.Assert(err, gc.IsNil)
	c.Assert(container.IsRunning(), jc.IsFalse)
}
