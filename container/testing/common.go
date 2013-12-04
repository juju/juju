// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/instance"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

func StartContainer(c *gc.C, manager container.Manager, machineId string) instance.Instance {
	stateInfo := jujutesting.FakeStateInfo(machineId)
	apiInfo := jujutesting.FakeAPIInfo(machineId)
	machineConfig := environs.NewMachineConfig(machineId, "fake-nonce", stateInfo, apiInfo)
	machineConfig.SyslogPort = 2345
	machineConfig.Tools = &tools.Tools{
		Version: version.MustParseBinary("2.3.4-foo-bar"),
		URL:     "http://tools.testing.invalid/2.3.4-foo-bar.tgz",
	}

	series := "series"
	network := container.BridgeNetworkConfig("nic42")
	inst, err := manager.StartContainer(machineConfig, series, network)
	c.Assert(err, gc.IsNil)
	return inst
}
