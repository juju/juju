// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"os"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/container"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
)

func MockMachineConfig(machineId string) (*instancecfg.InstanceConfig, error) {

	apiInfo := jujutesting.FakeAPIInfo(machineId)
	instanceConfig, err := instancecfg.NewInstanceConfig(testing.ControllerTag, machineId, "fake-nonce",
		imagemetadata.ReleasedStream, corebase.MakeDefaultBase("ubuntu", "22.04"), apiInfo)
	if err != nil {
		return nil, err
	}
	err = instanceConfig.SetTools(tools.List{
		&tools.Tools{
			Version: version.MustParseBinary("2.5.2-ubuntu-amd64"),
			URL:     "http://tools.testing.invalid/2.5.2-ubuntu-amd64.tgz",
		},
	})
	if err != nil {
		return nil, err
	}

	return instanceConfig, nil
}

func CreateContainer(c *gc.C, manager container.Manager, machineId string) instances.Instance {
	instanceConfig, err := MockMachineConfig(machineId)
	c.Assert(err, jc.ErrorIsNil)
	return CreateContainerWithMachineConfig(c, manager, instanceConfig)
}

func CreateContainerWithMachineConfig(
	c *gc.C,
	manager container.Manager,
	instanceConfig *instancecfg.InstanceConfig,
) instances.Instance {

	networkConfig := container.BridgeNetworkConfig(0, nil)
	storageConfig := &container.StorageConfig{}
	return CreateContainerWithMachineAndNetworkAndStorageConfig(c, manager, instanceConfig, networkConfig, storageConfig)
}

func CreateContainerWithMachineAndNetworkAndStorageConfig(
	c *gc.C,
	manager container.Manager,
	instanceConfig *instancecfg.InstanceConfig,
	networkConfig *container.NetworkConfig,
	storageConfig *container.StorageConfig,
) instances.Instance {
	callback := func(settableStatus status.Status, info string, data map[string]interface{}) error { return nil }
	inst, hardware, err := manager.CreateContainer(
		instanceConfig, constraints.Value{}, corebase.MakeDefaultBase("ubuntu", "18.04"), networkConfig, storageConfig, callback)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hardware, gc.NotNil)
	c.Assert(hardware.String(), gc.Not(gc.Equals), "")
	return inst
}

func AssertCloudInit(c *gc.C, filename string) []byte {
	c.Assert(filename, jc.IsNonEmptyFile)
	data, err := os.ReadFile(filename)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), jc.HasPrefix, "#cloud-config\n")
	return data
}
