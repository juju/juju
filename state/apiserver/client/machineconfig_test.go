// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/environs"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/apiserver/client"
	coretools "github.com/juju/juju/tools"
)

type machineConfigSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&machineConfigSuite{})

func (s *machineConfigSuite) TestMachineConfig(c *gc.C) {
	addrs := []network.Address{network.NewAddress("1.2.3.4", network.ScopeUnknown)}
	hc := instance.MustParseHardware("mem=4G arch=amd64")
	apiParams := params.AddMachineParams{
		Jobs:       []params.MachineJob{params.JobHostUnits},
		InstanceId: instance.Id("1234"),
		Nonce:      "foo",
		HardwareCharacteristics: hc,
		Addrs: addrs,
	}
	machines, err := s.APIState.Client().AddMachines([]params.AddMachineParams{apiParams})
	c.Assert(err, gc.IsNil)
	c.Assert(len(machines), gc.Equals, 1)

	machineId := machines[0].Machine
	machineConfig, err := client.MachineConfig(s.State, machineId, apiParams.Nonce, "")
	c.Assert(err, gc.IsNil)

	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	env, err := environs.New(envConfig)
	c.Assert(err, gc.IsNil)
	stateInfo, apiInfo, err := env.StateInfo()
	c.Assert(err, gc.IsNil)
	c.Check(machineConfig.MongoInfo.Addrs, gc.DeepEquals, stateInfo.Addrs)
	c.Check(machineConfig.APIInfo.Addrs, gc.DeepEquals, apiInfo.Addrs)
	c.Assert(machineConfig.Tools.URL, gc.Not(gc.Equals), "")
}

func (s *machineConfigSuite) TestMachineConfigNoArch(c *gc.C) {
	apiParams := params.AddMachineParams{
		Jobs:       []params.MachineJob{params.JobHostUnits},
		InstanceId: instance.Id("1234"),
		Nonce:      "foo",
	}
	machines, err := s.APIState.Client().AddMachines([]params.AddMachineParams{apiParams})
	c.Assert(err, gc.IsNil)
	c.Assert(len(machines), gc.Equals, 1)
	_, err = client.MachineConfig(s.State, machines[0].Machine, apiParams.Nonce, "")
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("arch is not set for %q", "machine-"+machines[0].Machine))
}

func (s *machineConfigSuite) TestMachineConfigNoTools(c *gc.C) {
	s.PatchValue(&envtools.DefaultBaseURL, "")
	addrs := []network.Address{network.NewAddress("1.2.3.4", network.ScopeUnknown)}
	hc := instance.MustParseHardware("mem=4G arch=amd64")
	apiParams := params.AddMachineParams{
		Series:     "quantal",
		Jobs:       []params.MachineJob{params.JobHostUnits},
		InstanceId: instance.Id("1234"),
		Nonce:      "foo",
		HardwareCharacteristics: hc,
		Addrs: addrs,
	}
	machines, err := s.APIState.Client().AddMachines([]params.AddMachineParams{apiParams})
	c.Assert(err, gc.IsNil)
	_, err = client.MachineConfig(s.State, machines[0].Machine, apiParams.Nonce, "")
	c.Assert(err, gc.ErrorMatches, coretools.ErrNoMatches.Error())
}
