// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"io/ioutil"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/instance"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

func CreateContainer(c *gc.C, manager container.Manager, machineId string) instance.Instance {
	stateInfo := jujutesting.FakeStateInfo(machineId)
	apiInfo := jujutesting.FakeAPIInfo(machineId)
	machineConfig := environs.NewMachineConfig(machineId, "fake-nonce", stateInfo, apiInfo)
	machineConfig.Tools = &tools.Tools{
		Version: version.MustParseBinary("2.3.4-foo-bar"),
		URL:     "http://tools.testing.invalid/2.3.4-foo-bar.tgz",
	}

	series := "series"
	network := container.BridgeNetworkConfig("nic42")
	inst, hardware, err := manager.CreateContainer(machineConfig, series, network)
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
