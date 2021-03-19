// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"fmt"
	"net"
	"strconv"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/client/client"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/juju/testing"
	jujutesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
)

type machineConfigSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&machineConfigSuite{})

func (s *machineConfigSuite) TestMachineConfig(c *gc.C) {
	hc := instance.MustParseHardware("mem=4G arch=amd64")
	apiParams := params.AddMachineParams{
		Jobs:                    []model.MachineJob{model.JobHostUnits},
		InstanceId:              instance.Id("1234"),
		Nonce:                   "foo",
		HardwareCharacteristics: hc,
		Addrs:                   params.FromProviderAddresses(network.NewProviderAddress("1.2.3.4")),
	}
	machines, err := s.APIState.Client().AddMachines([]params.AddMachineParams{apiParams})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 1)

	machineId := machines[0].Machine
	instanceConfig, err := client.InstanceConfig(s.StatePool.SystemState(), s.State, machineId, apiParams.Nonce, "")
	c.Assert(err, jc.ErrorIsNil)

	cfg, err := s.State.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	apiAddrs := []string{net.JoinHostPort("localhost", strconv.Itoa(cfg.APIPort()))}

	c.Check(instanceConfig.APIInfo.Addrs, gc.DeepEquals, apiAddrs)
	toolsURL := fmt.Sprintf("https://%s/model/%s/tools/%s",
		apiAddrs[0], jujutesting.ModelTag.Id(), instanceConfig.AgentVersion())
	c.Assert(instanceConfig.ToolsList().URLs(), jc.DeepEquals, map[version.Binary][]string{
		instanceConfig.AgentVersion(): {toolsURL},
	})
}

func (s *machineConfigSuite) TestMachineConfigNoArch(c *gc.C) {
	apiParams := params.AddMachineParams{
		Jobs:       []model.MachineJob{model.JobHostUnits},
		InstanceId: instance.Id("1234"),
		Nonce:      "foo",
	}
	machines, err := s.APIState.Client().AddMachines([]params.AddMachineParams{apiParams})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 1)
	_, err = client.InstanceConfig(s.StatePool.SystemState(), s.State, machines[0].Machine, apiParams.Nonce, "")
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("arch is not set for %q", "machine-"+machines[0].Machine))
}

func (s *machineConfigSuite) TestMachineConfigNoTools(c *gc.C) {
	s.PatchValue(&envtools.DefaultBaseURL, "")
	hc := instance.MustParseHardware("mem=4G arch=amd64")
	apiParams := params.AddMachineParams{
		Series:                  "quantal",
		Jobs:                    []model.MachineJob{model.JobHostUnits},
		InstanceId:              instance.Id("1234"),
		Nonce:                   "foo",
		HardwareCharacteristics: hc,
		Addrs:                   params.FromProviderAddresses(network.NewProviderAddress("1.2.3.4")),
	}
	machines, err := s.APIState.Client().AddMachines([]params.AddMachineParams{apiParams})
	c.Assert(err, jc.ErrorIsNil)
	_, err = client.InstanceConfig(s.StatePool.SystemState(), s.State, machines[0].Machine, apiParams.Nonce, "")
	c.Assert(err, gc.ErrorMatches, "finding agent binaries: "+coretools.ErrNoMatches.Error())
}
