// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networkingcommon_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

type networkconfigSuite struct {
	testing.JujuConnSuite

	machine       *state.Machine
	resources     *common.Resources
	networkconfig *networkingcommon.NetworkConfigAPI
}

func (s *networkconfigSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	var err error
	s.machine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *networkconfigSuite) TestSetObservedNetworkConfig(c *gc.C) {
	c.Skip("dimitern: Test disabled until dummy provider is fixed properly")
	devices, err := s.machine.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devices, gc.HasLen, 0)

	err = s.machine.SetInstanceInfo("i-foo", "FAKE_NONCE", nil, nil, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	observedConfig := []params.NetworkConfig{{
		InterfaceName: "lo",
		InterfaceType: "loopback",
		CIDR:          "127.0.0.0/8",
		Address:       "127.0.0.1",
	}, {
		InterfaceName: "eth0",
		InterfaceType: "ethernet",
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		CIDR:          "0.10.0.0/24",
		Address:       "0.10.0.2",
	}, {
		InterfaceName: "eth1",
		InterfaceType: "ethernet",
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		CIDR:          "0.20.0.0/24",
		Address:       "0.20.0.2",
	}}
	args := params.SetMachineNetworkConfig{
		Tag:    s.machine.Tag().String(),
		Config: observedConfig,
	}

	err = s.networkconfig.SetObservedNetworkConfig(args)
	c.Assert(err, jc.ErrorIsNil)

	devices, err = s.machine.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devices, gc.HasLen, 3)

	for _, device := range devices {
		c.Check(device.Name(), gc.Matches, `(lo|eth0|eth1)`)
		c.Check(string(device.Type()), gc.Matches, `(loopback|ethernet)`)
		c.Check(device.MACAddress(), gc.Matches, `(|aa:bb:cc:dd:ee:f0|aa:bb:cc:dd:ee:f1)`)
	}
}

func (s *networkconfigSuite) TestSetObservedNetworkConfigPermissions(c *gc.C) {
	args := params.SetMachineNetworkConfig{
		Tag:    "machine-0",
		Config: nil,
	}

	err := s.networkconfig.SetObservedNetworkConfig(args)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *networkconfigSuite) TestSetProviderNetworkConfig(c *gc.C) {
	c.Skip("dimitern: Test disabled until dummy provider is fixed properly")
	devices, err := s.machine.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devices, gc.HasLen, 0)

	err = s.machine.SetInstanceInfo("i-foo", "FAKE_NONCE", nil, nil, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machine.Tag().String()},
	}}

	result, err := s.networkconfig.SetProviderNetworkConfig(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{nil}},
	})

	devices, err = s.machine.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devices, gc.HasLen, 3)

	for _, device := range devices {
		c.Check(device.Name(), gc.Matches, `eth[0-2]`)
		c.Check(string(device.Type()), gc.Equals, "ethernet")
		c.Check(device.MACAddress(), gc.Matches, `aa:bb:cc:dd:ee:f[0-2]`)
		addrs, err := device.Addresses()
		c.Check(err, jc.ErrorIsNil)
		c.Check(addrs, gc.HasLen, 1)
	}
}

func (s *networkconfigSuite) TestSetProviderNetworkConfigPermissions(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-1"},
		{Tag: "machine-0"},
		{Tag: "machine-42"},
	}}

	result, err := s.networkconfig.SetProviderNetworkConfig(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservertesting.NotProvisionedError(s.machine.Id())},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}
