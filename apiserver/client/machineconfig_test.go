// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"fmt"
	"net"
	"strconv"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/client"
	"github.com/juju/juju/apiserver/params"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	jujutesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
)

type machineConfigSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&machineConfigSuite{})

func (s *machineConfigSuite) TestMachineConfig(c *gc.C) {
	addrs := network.NewAddresses("1.2.3.4")
	hc := instance.MustParseHardware("mem=4G arch=amd64")
	apiParams := params.AddMachineParams{
		Jobs:       []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
		InstanceId: instance.Id("1234"),
		Nonce:      "foo",
		HardwareCharacteristics: hc,
		Addrs: params.FromNetworkAddresses(addrs),
	}
	machines, err := s.APIState.Client().AddMachines([]params.AddMachineParams{apiParams})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 1)

	machineId := machines[0].Machine
	instanceConfig, err := client.InstanceConfig(s.State, machineId, apiParams.Nonce, "")
	c.Assert(err, jc.ErrorIsNil)

	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	mongoAddrs := s.State.MongoConnectionInfo().Addrs
	apiAddrs := []string{net.JoinHostPort("localhost", strconv.Itoa(envConfig.APIPort()))}

	c.Check(instanceConfig.MongoInfo.Addrs, gc.DeepEquals, mongoAddrs)
	c.Check(instanceConfig.APIInfo.Addrs, gc.DeepEquals, apiAddrs)
	toolsURL := fmt.Sprintf("https://%s/environment/%s/tools/%s",
		apiAddrs[0], jujutesting.EnvironmentTag.Id(), instanceConfig.Tools.Version)
	c.Assert(instanceConfig.Tools.URL, gc.Equals, toolsURL)
	c.Assert(instanceConfig.AgentEnvironment[agent.AllowsSecureConnection], gc.Equals, "true")
}

func (s *machineConfigSuite) TestSecureConnectionDisallowed(c *gc.C) {
	// StateServingInfo without CAPrivateKey will not allow secure connections.
	servingInfo := state.StateServingInfo{
		PrivateKey:   jujutesting.ServerKey,
		Cert:         jujutesting.ServerCert,
		SharedSecret: "really, really secret",
		APIPort:      4321,
		StatePort:    1234,
	}
	s.State.SetStateServingInfo(servingInfo)
	hc := instance.MustParseHardware("mem=4G arch=amd64")
	apiParams := params.AddMachineParams{
		Jobs:       []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
		InstanceId: instance.Id("1234"),
		Nonce:      "foo",
		HardwareCharacteristics: hc,
	}
	machines, err := s.APIState.Client().AddMachines([]params.AddMachineParams{apiParams})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 1)

	machineId := machines[0].Machine
	instanceConfig, err := client.InstanceConfig(s.State, machineId, apiParams.Nonce, "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instanceConfig.AgentEnvironment[agent.AllowsSecureConnection], gc.Equals, "false")
}

func (s *machineConfigSuite) TestMachineConfigNoArch(c *gc.C) {
	apiParams := params.AddMachineParams{
		Jobs:       []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
		InstanceId: instance.Id("1234"),
		Nonce:      "foo",
	}
	machines, err := s.APIState.Client().AddMachines([]params.AddMachineParams{apiParams})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 1)
	_, err = client.InstanceConfig(s.State, machines[0].Machine, apiParams.Nonce, "")
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("arch is not set for %q", "machine-"+machines[0].Machine))
}

func (s *machineConfigSuite) TestMachineConfigNoTools(c *gc.C) {
	s.PatchValue(&envtools.DefaultBaseURL, "")
	addrs := network.NewAddresses("1.2.3.4")
	hc := instance.MustParseHardware("mem=4G arch=amd64")
	apiParams := params.AddMachineParams{
		Series:     "quantal",
		Jobs:       []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
		InstanceId: instance.Id("1234"),
		Nonce:      "foo",
		HardwareCharacteristics: hc,
		Addrs: params.FromNetworkAddresses(addrs),
	}
	machines, err := s.APIState.Client().AddMachines([]params.AddMachineParams{apiParams})
	c.Assert(err, jc.ErrorIsNil)
	_, err = client.InstanceConfig(s.State, machines[0].Machine, apiParams.Nonce, "")
	c.Assert(err, gc.ErrorMatches, coretools.ErrNoMatches.Error())
}
