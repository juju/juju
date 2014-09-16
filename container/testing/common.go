// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"io/ioutil"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/container"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudinit"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/instance"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

func MockMachineConfig(machineId string) (*cloudinit.MachineConfig, error) {

	stateInfo := jujutesting.FakeStateInfo(machineId)
	apiInfo := jujutesting.FakeAPIInfo(machineId)
	machineConfig, err := environs.NewMachineConfig(machineId, "fake-nonce", imagemetadata.ReleasedStream, "quantal", nil, stateInfo, apiInfo)
	if err != nil {
		return nil, err
	}
	machineConfig.Tools = &tools.Tools{
		Version: version.MustParseBinary("2.3.4-quantal-amd64"),
		URL:     "http://tools.testing.invalid/2.3.4-quantal-amd64.tgz",
	}

	return machineConfig, nil
}

func CreateContainer(c *gc.C, manager container.Manager, machineId string) instance.Instance {
	machineConfig, err := MockMachineConfig(machineId)
	c.Assert(err, gc.IsNil)

	envConfig, err := config.New(config.NoDefaults, dummy.SampleConfig())
	c.Assert(err, gc.IsNil)
	machineConfig.Config = envConfig
	return CreateContainerWithMachineConfig(c, manager, machineConfig)
}

func CreateContainerWithMachineConfig(
	c *gc.C,
	manager container.Manager,
	machineConfig *cloudinit.MachineConfig,
) instance.Instance {

	network := container.BridgeNetworkConfig("nic42")
	inst, hardware, err := manager.CreateContainer(machineConfig, "quantal", network)
	c.Assert(err, gc.IsNil)
	c.Assert(hardware, gc.NotNil)
	c.Assert(hardware.String(), gc.Not(gc.Equals), "")
	return inst
}

func AssertCloudInit(c *gc.C, filename string) []byte {
	c.Assert(filename, jc.IsNonEmptyFile)
	data, err := ioutil.ReadFile(filename)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), jc.HasPrefix, "#cloud-config\n")
	return data
}
