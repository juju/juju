// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm_test

import (
	"path/filepath"

	"github.com/juju/loggo"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/container/kvm"
	kvmtesting "launchpad.net/juju-core/container/kvm/testing"
	containertesting "launchpad.net/juju-core/container/testing"
	"launchpad.net/juju-core/instance"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
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
	s.manager, err = kvm.NewContainerManager(container.ManagerConfig{container.ConfigName: "test"})
	c.Assert(err, gc.IsNil)
}

func (*KVMSuite) TestManagerNameNeeded(c *gc.C) {
	manager, err := kvm.NewContainerManager(container.ManagerConfig{container.ConfigName: ""})
	c.Assert(err, gc.ErrorMatches, "name is required")
	c.Assert(manager, gc.IsNil)
}

func (*KVMSuite) TestManagerWarnsAboutUnknownOption(c *gc.C) {
	_, err := kvm.NewContainerManager(container.ManagerConfig{
		container.ConfigName: "BillyBatson",
		"shazam":             "Captain Marvel",
	})
	c.Assert(err, gc.IsNil)
	c.Assert(c.GetTestLog(), gc.Matches, `^.*WARNING juju.container.kvm Found unused config option with key: "shazam" and value: "Captain Marvel"\n*`)
}

func (s *KVMSuite) TestListInitiallyEmpty(c *gc.C) {
	containers, err := s.manager.ListContainers()
	c.Assert(err, gc.IsNil)
	c.Assert(containers, gc.HasLen, 0)
}

func (s *KVMSuite) createRunningContainer(c *gc.C, name string) kvm.Container {
	kvmContainer := s.Factory.New(name)
	network := container.BridgeNetworkConfig("testbr0")
	c.Assert(kvmContainer.Start(kvm.StartParams{
		Series:       "quantal",
		Arch:         version.Current.Arch,
		UserDataFile: "userdata.txt",
		Network:      network}), gc.IsNil)
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

type ConstraintsSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&ConstraintsSuite{})

func (s *ConstraintsSuite) TestDefaults(c *gc.C) {

	for _, test := range []struct {
		cons     string
		expected kvm.StartParams
		infoLog  []string
	}{{
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: kvm.DefaultCpu,
			RootDisk: kvm.DefaultDisk,
		},
	}, {
		cons: "mem=256M",
		expected: kvm.StartParams{
			Memory:   kvm.MinMemory,
			CpuCores: kvm.DefaultCpu,
			RootDisk: kvm.DefaultDisk,
		},
	}, {
		cons: "mem=4G",
		expected: kvm.StartParams{
			Memory:   4 * 1024,
			CpuCores: kvm.DefaultCpu,
			RootDisk: kvm.DefaultDisk,
		},
	}, {
		cons: "cpu-cores=4",
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: 4,
			RootDisk: kvm.DefaultDisk,
		},
	}, {
		cons: "cpu-cores=0",
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: kvm.MinCpu,
			RootDisk: kvm.DefaultDisk,
		},
	}, {
		cons: "root-disk=512M",
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: kvm.DefaultCpu,
			RootDisk: kvm.MinDisk,
		},
	}, {
		cons: "root-disk=4G",
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: kvm.DefaultCpu,
			RootDisk: 4,
		},
	}, {
		cons: "arch=arm",
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: kvm.DefaultCpu,
			RootDisk: kvm.DefaultDisk,
		},
		infoLog: []string{
			`arch constraint of "arm" being ignored as not supported`,
		},
	}, {
		cons: "container=lxc",
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: kvm.DefaultCpu,
			RootDisk: kvm.DefaultDisk,
		},
		infoLog: []string{
			`container constraint of "lxc" being ignored as not supported`,
		},
	}, {
		cons: "cpu-power=100",
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: kvm.DefaultCpu,
			RootDisk: kvm.DefaultDisk,
		},
		infoLog: []string{
			`cpu-power constraint of 100 being ignored as not supported`,
		},
	}, {
		cons: "tags=foo,bar",
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: kvm.DefaultCpu,
			RootDisk: kvm.DefaultDisk,
		},
		infoLog: []string{
			`tags constraint of "foo,bar" being ignored as not supported`,
		},
	}, {
		cons: "mem=4G cpu-cores=4 root-disk=20G arch=arm cpu-power=100 container=lxc tags=foo,bar",
		expected: kvm.StartParams{
			Memory:   4 * 1024,
			CpuCores: 4,
			RootDisk: 20,
		},
		infoLog: []string{
			`arch constraint of "arm" being ignored as not supported`,
			`container constraint of "lxc" being ignored as not supported`,
			`cpu-power constraint of 100 being ignored as not supported`,
			`tags constraint of "foo,bar" being ignored as not supported`,
		},
	}} {
		tw := &loggo.TestWriter{}
		c.Assert(loggo.RegisterWriter("constraint-tester", tw, loggo.DEBUG), gc.IsNil)
		cons := constraints.MustParse(test.cons)
		params := kvm.ParseConstraintsToStartParams(cons)
		c.Check(params, gc.DeepEquals, test.expected)
		c.Check(tw.Log, jc.LogMatches, test.infoLog)
		loggo.RemoveWriter("constraint-tester")
	}
}
