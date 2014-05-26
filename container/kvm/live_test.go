// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm_test

import (
	"fmt"
	"os"
	"runtime"

	"github.com/juju/loggo"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/container/kvm"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	jujutesting "launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

type LiveSuite struct {
	coretesting.BaseSuite
	ContainerDir string
	RemovedDir   string
}

var _ = gc.Suite(&LiveSuite{})

func (s *LiveSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
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
			container.ConfigName:   name,
			container.ConfigLogDir: c.MkDir(),
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
			err := manager.DestroyContainer(instance.Id())
			c.Check(err, gc.IsNil)
		}
	}
}

func createContainer(c *gc.C, manager container.Manager, machineId string) instance.Instance {
	machineNonce := "fake-nonce"
	stateInfo := jujutesting.FakeStateInfo(machineId)
	apiInfo := jujutesting.FakeAPIInfo(machineId)
	machineConfig := environs.NewMachineConfig(machineId, machineNonce, nil, nil, stateInfo, apiInfo)
	network := container.BridgeNetworkConfig("virbr0")

	machineConfig.Tools = &tools.Tools{
		Version: version.MustParseBinary("2.3.4-foo-bar"),
		URL:     "http://tools.testing.invalid/2.3.4-foo-bar.tgz",
	}
	environConfig := dummyConfig(c)
	err := environs.FinishMachineConfig(machineConfig, environConfig, constraints.Value{})
	c.Assert(err, gc.IsNil)

	inst, hardware, err := manager.CreateContainer(machineConfig, "precise", network)
	c.Assert(err, gc.IsNil)
	c.Assert(hardware, gc.NotNil)
	expected := fmt.Sprintf("arch=%s cpu-cores=1 mem=512M root-disk=8192M", version.Current.Arch)
	c.Assert(hardware.String(), gc.Equals, expected)
	return inst
}

func (s *LiveSuite) TestShutdownMachines(c *gc.C) {
	manager := s.newManager(c, "test")
	createContainer(c, manager, "1/kvm/0")
	createContainer(c, manager, "1/kvm/1")
	assertNumberOfContainers(c, manager, 2)

	shutdownMachines(manager)(c)
	assertNumberOfContainers(c, manager, 0)
}

func (s *LiveSuite) TestManagerIsolation(c *gc.C) {
	firstManager := s.newManager(c, "first")
	s.AddCleanup(shutdownMachines(firstManager))

	createContainer(c, firstManager, "1/kvm/0")
	createContainer(c, firstManager, "1/kvm/1")

	secondManager := s.newManager(c, "second")
	s.AddCleanup(shutdownMachines(secondManager))

	createContainer(c, secondManager, "1/kvm/0")

	assertNumberOfContainers(c, firstManager, 2)
	assertNumberOfContainers(c, secondManager, 1)
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
