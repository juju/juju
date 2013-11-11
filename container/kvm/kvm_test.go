// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/container/kvm"
	"launchpad.net/juju-core/instance"
	jc "launchpad.net/juju-core/testing/checkers"
)

type KVMSuite struct {
	kvm.TestSuite
}

var _ = gc.Suite(&KVMSuite{})

// TODO: work out how to test the actual kvm implementations.

func (*KVMSuite) TestListInitiallyEmpty(c *gc.C) {
	manager, err := kvm.NewContainerManager(container.ManagerConfig{Name: "test"})
	c.Assert(err, gc.IsNil)
	containers, err := manager.ListContainers()
	c.Assert(err, gc.IsNil)
	c.Assert(containers, gc.HasLen, 0)
}

func (s *KVMSuite) createRunningContainer(c *gc.C, name string) container.Container {
	container := s.Factory.New(name)
	c.Assert(container.Start(), gc.IsNil)
	return container
}

func (s *KVMSuite) TestListMatchesManagerName(c *gc.C) {
	manager, err := kvm.NewContainerManager(container.ManagerConfig{Name: "test"})
	c.Assert(err, gc.IsNil)
	s.createRunningContainer(c, "test-match1")
	s.createRunningContainer(c, "test-match2")
	s.createRunningContainer(c, "testNoMatch")
	s.createRunningContainer(c, "other")
	containers, err := manager.ListContainers()
	c.Assert(err, gc.IsNil)
	c.Assert(containers, gc.HasLen, 2)
	expectedIds := []instance.Id{"test-match1", "test-match2"}
	ids := []instance.Id{containers[0].Id(), containers[1].Id()}
	c.Assert(ids, jc.SameContents, expectedIds)
}

func (s *KVMSuite) TestListMatchesRunningContainers(c *gc.C) {
	manager, err := kvm.NewContainerManager(container.ManagerConfig{Name: "test"})
	c.Assert(err, gc.IsNil)
	running := s.createRunningContainer(c, "test-running")
	s.Factory.New("test-stopped")
	containers, err := manager.ListContainers()
	c.Assert(err, gc.IsNil)
	c.Assert(containers, gc.HasLen, 1)
	c.Assert(string(containers[0].Id()), gc.Equals, running.Name())
}
