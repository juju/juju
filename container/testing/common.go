// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"io/ioutil"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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
	c.Assert(err, jc.ErrorIsNil)

	envConfig, err := config.New(config.NoDefaults, dummy.SampleConfig())
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hardware, gc.NotNil)
	c.Assert(hardware.String(), gc.Not(gc.Equals), "")
	return inst
}

func AssertCloudInit(c *gc.C, filename string) []byte {
	c.Assert(filename, jc.IsNonEmptyFile)
	data, err := ioutil.ReadFile(filename)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), jc.HasPrefix, "#cloud-config\n")
	return data
}

// CreateContainerTest tries to create a container and returns any errors encountered along the
// way
func CreateContainerTest(c *gc.C, manager container.Manager, machineId string) (instance.Instance, error) {
	machineConfig, err := MockMachineConfig(machineId)
	if err != nil {
		return nil, errors.Trace(err)
	}

	envConfig, err := config.New(config.NoDefaults, dummy.SampleConfig())
	if err != nil {
		return nil, errors.Trace(err)
	}
	machineConfig.Config = envConfig

	network := container.BridgeNetworkConfig("nic42")

	inst, hardware, err := manager.CreateContainer(machineConfig, "quantal", network)

	if err != nil {
		return nil, errors.Trace(err)
	}
	if hardware == nil {
		return nil, errors.New("nil hardware characteristics")
	}
	if hardware.String() == "" {
		return nil, errors.New("empty hardware characteristics")
	}
	return inst, nil

}
