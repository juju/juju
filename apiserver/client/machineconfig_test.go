// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"fmt"
	"net"
	"strconv"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/client"
	"github.com/juju/juju/apiserver/params"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
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
	mongoAddrs := s.State.MongoConnectionInfo().Addrs
	apiAddrs := []string{net.JoinHostPort("localhost", strconv.Itoa(envConfig.APIPort()))}

	c.Check(machineConfig.MongoInfo.Addrs, gc.DeepEquals, mongoAddrs)
	c.Check(machineConfig.APIInfo.Addrs, gc.DeepEquals, apiAddrs)
	toolsURL := fmt.Sprintf("https://%s/environment/90168e4c-2f10-4e9c-83c2-feedfacee5a9/tools/%s", apiAddrs[0], machineConfig.Tools.Version)
	c.Assert(machineConfig.Tools.URL, gc.Equals, toolsURL)
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
