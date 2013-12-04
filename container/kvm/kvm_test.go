// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm_test

import (
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/container/kvm"
	kvmtesting "launchpad.net/juju-core/container/kvm/testing"
	containertesting "launchpad.net/juju-core/container/testing"
	"launchpad.net/juju-core/instance"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/version"
)

type KVMSuite struct {
	kvmtesting.TestSuite
	manager container.Manager
}

var _ = gc.Suite(&KVMSuite{})

func (s *KVMSuite) SetUpTest(c *gc.C) {
	s.TestSuite.SetUpTest(c)
	var err error
	s.manager, err = kvm.NewContainerManager(container.ManagerConfig{Name: "test"})
	c.Assert(err, gc.IsNil)
}

func (*KVMSuite) TestManagerNameNeeded(c *gc.C) {
	manager, err := kvm.NewContainerManager(container.ManagerConfig{})
	c.Assert(err, gc.ErrorMatches, "name is required")
	c.Assert(manager, gc.IsNil)
}

func (s *KVMSuite) TestListInitiallyEmpty(c *gc.C) {
	containers, err := s.manager.ListContainers()
	c.Assert(err, gc.IsNil)
	c.Assert(containers, gc.HasLen, 0)
}

func (s *KVMSuite) createRunningContainer(c *gc.C, name string) kvm.Container {
	kvmContainer := s.Factory.New(name)
	network := container.BridgeNetworkConfig("testbr0")
	c.Assert(kvmContainer.Start("quantal", version.Current.Arch, "userdata.txt", network), gc.IsNil)
	return kvmContainer
}

func (s *KVMSuite) TestListMatchesManagerName(c *gc.C) {
	s.createRunningContainer(c, "test-match1")
	s.createRunningContainer(c, "test-match2")
	s.createRunningContainer(c, "testNoMatch")
	s.createRunningContainer(c, "other")
	containers, err := s.manager.ListContainers()
	c.Assert(err, gc.IsNil)
	c.Assert(containers, gc.HasLen, 2)
	expectedIds := []instance.Id{"test-match1", "test-match2"}
	ids := []instance.Id{containers[0].Id(), containers[1].Id()}
	c.Assert(ids, jc.SameContents, expectedIds)
}

func (s *KVMSuite) TestListMatchesRunningContainers(c *gc.C) {
	running := s.createRunningContainer(c, "test-running")
	s.Factory.New("test-stopped")
	containers, err := s.manager.ListContainers()
	c.Assert(err, gc.IsNil)
	c.Assert(containers, gc.HasLen, 1)
	c.Assert(string(containers[0].Id()), gc.Equals, running.Name())
}

func (s *KVMSuite) TestStartContainer(c *gc.C) {
	instance := containertesting.StartContainer(c, s.manager, "1/kvm/0")
	name := string(instance.Id())
	cloudInitFilename := filepath.Join(s.ContainerDir, name, "cloud-init")
	containertesting.AssertCloudInit(c, cloudInitFilename)
}

func (s *KVMSuite) TestStopContainer(c *gc.C) {
	instance := containertesting.StartContainer(c, s.manager, "1/lxc/0")

	err := s.manager.StopContainer(instance)
	c.Assert(err, gc.IsNil)

	name := string(instance.Id())
	// Check that the container dir is no longer in the container dir
	c.Assert(filepath.Join(s.ContainerDir, name), jc.DoesNotExist)
	// but instead, in the removed container dir
	c.Assert(filepath.Join(s.RemovedDir, name), jc.IsDirectory)
}
