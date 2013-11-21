// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm_test

import (
	"os"
	"runtime"

	gc "launchpad.net/gocheck"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/container/kvm"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	jujutesting "launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

type LiveSuite struct {
	testbase.LoggingSuite
	ContainerDir string
	RemovedDir   string
}

var _ = gc.Suite(&LiveSuite{})

func (s *LiveSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	// Skip if not linux
	if runtime.GOOS != "linux" {
		c.Skip("not running linux")
	}
	// Skip if not running as root.
	if os.Getuid() != 0 {
		c.Skip("not running as root")
	}
	s.ContainerDir = c.MkDir()
	s.PatchValue(&container.ContainerDir, s.ContainerDir)
	s.RemovedDir = c.MkDir()
	s.PatchValue(&container.RemovedContainerDir, s.RemovedDir)
	loggo.GetLogger("juju.container").SetLogLevel(loggo.TRACE)
}

func (s *LiveSuite) newManager(c *gc.C, name string) container.Manager {
	manager, err := kvm.NewContainerManager(
		container.ManagerConfig{
			Name:   name,
			LogDir: c.MkDir(),
		})
	c.Assert(err, gc.IsNil)
	return manager
}

func assertNumberOfContainers(c *gc.C, manager container.Manager, count int) {
	containers, err := manager.ListContainers()
	c.Assert(err, gc.IsNil)
	c.Assert(containers, gc.HasLen, count)
}

func (s *LiveSuite) TestNoInitialContainers(c *gc.C) {
	manager := s.newManager(c, "test")
	assertNumberOfContainers(c, manager, 0)
}

func shutdownMachines(manager container.Manager) func(*gc.C) {
	return func(c *gc.C) {
		instances, err := manager.ListContainers()
		c.Assert(err, gc.IsNil)
		for _, instance := range instances {
			err := manager.StopContainer(instance)
			c.Check(err, gc.IsNil)
		}
	}
}

func startContainer(c *gc.C, manager container.Manager, machineId string) instance.Instance {
	machineNonce := "fake-nonce"
	stateInfo := jujutesting.FakeStateInfo(machineId)
	apiInfo := jujutesting.FakeAPIInfo(machineId)
	machineConfig := environs.NewMachineConfig(machineId, machineNonce, stateInfo, apiInfo)
	network := container.BridgeNetworkConfig("virbr0")

	machineConfig.Tools = &tools.Tools{
		Version: version.MustParseBinary("2.3.4-foo-bar"),
		URL:     "http://tools.testing.invalid/2.3.4-foo-bar.tgz",
	}
	environConfig := dummyConfig(c)
	err := environs.FinishMachineConfig(machineConfig, environConfig, constraints.Value{})
	c.Assert(err, gc.IsNil)

	instance, err := manager.StartContainer(machineConfig, "precise", network)
	c.Assert(err, gc.IsNil)
	return instance
}

func (s *LiveSuite) TestShutdownMachines(c *gc.C) {
	manager := s.newManager(c, "test")
	startContainer(c, manager, "1/kvm/0")
	startContainer(c, manager, "1/kvm/1")
	assertNumberOfContainers(c, manager, 2)

	shutdownMachines(manager)(c)
	assertNumberOfContainers(c, manager, 0)
}

func dummyConfig(c *gc.C) *config.Config {
	testConfig, err := config.New(config.UseDefaults, coretesting.FakeConfig())
	c.Assert(err, gc.IsNil)
	testConfig, err = testConfig.Apply(map[string]interface{}{
		"type":          "dummy",
		"state-server":  false,
		"agent-version": version.Current.Number.String(),
	})
	c.Assert(err, gc.IsNil)
	return testConfig
}
