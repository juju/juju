// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager_test

import (
	"fmt"
	"net"
	"strconv"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	apiclient "github.com/juju/juju/v2/api/client/machinemanager"
	"github.com/juju/juju/v2/apiserver/facades/client/machinemanager"
	"github.com/juju/juju/v2/core/instance"
	"github.com/juju/juju/v2/core/model"
	"github.com/juju/juju/v2/core/network"
	envtools "github.com/juju/juju/v2/environs/tools"
	"github.com/juju/juju/v2/juju/testing"
	"github.com/juju/juju/v2/rpc/params"
	jujutesting "github.com/juju/juju/v2/testing"
	coretools "github.com/juju/juju/v2/tools"
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
		Addrs:                   params.FromProviderAddresses(network.NewMachineAddress("1.2.3.4").AsProviderAddress()),
	}
	machines, err := apiclient.NewClient(s.APIState).AddMachines([]params.AddMachineParams{apiParams})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 1)

	machineId := machines[0].Machine
	systemState, err := s.StatePool.SystemState()
	c.Assert(err, jc.ErrorIsNil)
	instanceConfig, err := machinemanager.InstanceConfig(systemState, machinemanager.StateBackend(s.State), machineId, apiParams.Nonce, "")
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
	machines, err := apiclient.NewClient(s.APIState).AddMachines([]params.AddMachineParams{apiParams})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 1)
	systemState, err := s.StatePool.SystemState()
	c.Assert(err, jc.ErrorIsNil)
	_, err = machinemanager.InstanceConfig(systemState, machinemanager.StateBackend(s.State), machines[0].Machine, apiParams.Nonce, "")
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
		Addrs:                   params.FromProviderAddresses(network.NewMachineAddress("1.2.3.4").AsProviderAddress()),
	}
	machines, err := apiclient.NewClient(s.APIState).AddMachines([]params.AddMachineParams{apiParams})
	c.Assert(err, jc.ErrorIsNil)
	systemState, err := s.StatePool.SystemState()
	c.Assert(err, jc.ErrorIsNil)
	_, err = machinemanager.InstanceConfig(systemState, machinemanager.StateBackend(s.State), machines[0].Machine, apiParams.Nonce, "")
	c.Assert(err, gc.ErrorMatches, "finding agent binaries: "+coretools.ErrNoMatches.Error())
}
