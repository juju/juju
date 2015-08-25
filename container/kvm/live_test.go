// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm_test

import (
	"fmt"
	"os"
	"runtime"

	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/kvm"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/arch"
	jujutesting "github.com/juju/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
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
	c.Assert(err, jc.ErrorIsNil)
	return manager
}

func assertNumberOfContainers(c *gc.C, manager container.Manager, count int) {
	containers, err := manager.ListContainers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(containers, gc.HasLen, count)
}

func (s *LiveSuite) TestNoInitialContainers(c *gc.C) {
	manager := s.newManager(c, "test")
	assertNumberOfContainers(c, manager, 0)
}

func shutdownMachines(manager container.Manager) func(*gc.C) {
	return func(c *gc.C) {
		instances, err := manager.ListContainers()
		c.Assert(err, jc.ErrorIsNil)
		for _, instance := range instances {
			err := manager.DestroyContainer(instance.Id())
			c.Check(err, jc.ErrorIsNil)
		}
	}
}

func createContainer(c *gc.C, manager container.Manager, machineId string) instance.Instance {
	machineNonce := "fake-nonce"
	stateInfo := jujutesting.FakeStateInfo(machineId)
	apiInfo := jujutesting.FakeAPIInfo(machineId)
	instanceConfig, err := instancecfg.NewInstanceConfig(machineId, machineNonce, imagemetadata.ReleasedStream, "quantal", true, nil, stateInfo, apiInfo)
	c.Assert(err, jc.ErrorIsNil)
	network := container.BridgeNetworkConfig("virbr0", 0, nil)

	instanceConfig.Tools = &tools.Tools{
		Version: version.MustParseBinary("2.3.4-foo-bar"),
		URL:     "http://tools.testing.invalid/2.3.4-foo-bar.tgz",
	}
	environConfig := dummyConfig(c)
	err = instancecfg.FinishInstanceConfig(instanceConfig, environConfig)
	c.Assert(err, jc.ErrorIsNil)

	inst, hardware, err := manager.CreateContainer(instanceConfig, "precise", network, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hardware, gc.NotNil)
	expected := fmt.Sprintf("arch=%s cpu-cores=1 mem=512M root-disk=8192M", arch.HostArch())
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
	c.Assert(err, jc.ErrorIsNil)
	testConfig, err = testConfig.Apply(map[string]interface{}{
		"type":          "dummy",
		"state-server":  false,
		"agent-version": version.Current.Number.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	return testConfig
}
