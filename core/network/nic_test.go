// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
)

type nicSuite struct {
	info network.InterfaceInfos
}

var _ = gc.Suite(&nicSuite{})

func (s *nicSuite) SetUpTest(_ *gc.C) {
	s.info = network.InterfaceInfos{
		{VLANTag: 1, DeviceIndex: 0, InterfaceName: "eth0", MACAddress: "00:16:3e:aa:bb:cc"},
		{VLANTag: 0, DeviceIndex: 1, InterfaceName: "eth1"},
		{VLANTag: 42, DeviceIndex: 2, InterfaceName: "br2"},
		{ConfigType: network.ConfigDHCP, NoAutoStart: true},
		{Addresses: network.ProviderAddresses{network.NewMachineAddress("0.1.2.3").AsProviderAddress()}},
		{DNSServers: []string{"1.1.1.1", "2.2.2.2"}},
		{GatewayAddress: network.NewMachineAddress("4.3.2.1").AsProviderAddress()},
		{Routes: []network.Route{{
			DestinationCIDR: "0.1.2.3/24",
			GatewayIP:       "0.1.2.1",
			Metric:          0,
		}}},
		{DeviceIndex: 42, InterfaceName: "ovsbr0", VirtualPortType: network.OvsPort},
	}
}

func (s *nicSuite) TestActualInterfaceName(c *gc.C) {
	c.Check(s.info[0].ActualInterfaceName(), gc.Equals, "eth0.1")
	c.Check(s.info[1].ActualInterfaceName(), gc.Equals, "eth1")
	c.Check(s.info[2].ActualInterfaceName(), gc.Equals, "br2.42")
}

func (s *nicSuite) TestIsVirtual(c *gc.C) {
	c.Check(s.info[0].IsVirtual(), jc.IsTrue)
	c.Check(s.info[1].IsVirtual(), jc.IsFalse)
	c.Check(s.info[2].IsVirtual(), jc.IsTrue)
	c.Check(s.info[8].IsVirtual(), jc.IsTrue, gc.Commentf("expected NIC with OVS virtual port type to be treated as virtual"))
}

func (s *nicSuite) TestIsVLAN(c *gc.C) {
	c.Check(s.info[0].IsVLAN(), jc.IsTrue)
	c.Check(s.info[1].IsVLAN(), jc.IsFalse)
	c.Check(s.info[2].IsVLAN(), jc.IsTrue)
}

func (s *nicSuite) TestAdditionalFields(c *gc.C) {
	c.Check(s.info[3].ConfigType, gc.Equals, network.ConfigDHCP)
	c.Check(s.info[3].NoAutoStart, jc.IsTrue)
	c.Check(s.info[4].Addresses, jc.DeepEquals, network.ProviderAddresses{network.NewMachineAddress("0.1.2.3").AsProviderAddress()})
	c.Check(s.info[5].DNSServers, jc.DeepEquals, []string{"1.1.1.1", "2.2.2.2"})
	c.Check(s.info[6].GatewayAddress, jc.DeepEquals, network.NewMachineAddress("4.3.2.1").AsProviderAddress())
	c.Check(s.info[7].Routes, jc.DeepEquals, []network.Route{{
		DestinationCIDR: "0.1.2.3/24",
		GatewayIP:       "0.1.2.1",
		Metric:          0,
	}})
}

func (*nicSuite) TestInterfaceInfoValidate(c *gc.C) {
	dev := network.InterfaceInfo{InterfaceName: ""}
	c.Check(dev.Validate(), jc.ErrorIs, coreerrors.NotValid)

	dev = network.InterfaceInfo{MACAddress: "do you even MAC bro?"}
	c.Check(dev.Validate(), jc.ErrorIs, coreerrors.NotValid)

	dev = network.InterfaceInfo{
		InterfaceName: "eth0",
		MACAddress:    network.GenerateVirtualMACAddress(),
		InterfaceType: "invalid",
	}
	c.Check(dev.Validate(), jc.ErrorIs, coreerrors.NotValid)

	dev = network.InterfaceInfo{
		InterfaceName: "not#valid",
		InterfaceType: "bond",
	}
	c.Check(dev.Validate(), jc.ErrorIsNil)
}

func (*nicSuite) TestInterfaceInfosValidate(c *gc.C) {
	c.Check(getInterFaceInfos().Validate(), jc.ErrorIsNil)
}

func (*nicSuite) TestInterfaceInfosFiltering(c *gc.C) {
	filtered := getInterFaceInfos().Filter(func(iface network.InterfaceInfo) bool {
		return strings.HasPrefix(iface.InterfaceName, "eth")
	})

	var devs []string
	for _, iface := range filtered {
		devs = append(devs, iface.ParentInterfaceName+":"+iface.InterfaceName)
	}

	c.Check(devs, gc.DeepEquals, []string{
		":eth2",
		"bond0:eth0",
		"bond0:eth1",
	})

	// Filter again
	filtered = filtered.Filter(func(iface network.InterfaceInfo) bool {
		return iface.InterfaceName == "eth1"
	})

	devs = devs[0:0]
	for _, iface := range filtered {
		devs = append(devs, iface.ParentInterfaceName+":"+iface.InterfaceName)
	}

	c.Check(devs, gc.DeepEquals, []string{
		"bond0:eth1",
	})
}

func (s *nicSuite) TestInterfaceInfosGetByName(c *gc.C) {
	devs := s.info.GetByName("wrong-name")
	c.Assert(devs, gc.IsNil)

	devs = s.info.GetByName("eth0")
	c.Assert(devs, gc.HasLen, 1)
}

func (s *nicSuite) TestNormalizeMACAddress(c *gc.C) {
	specs := []struct {
		descr string
		in    string
		exp   string
	}{
		{
			descr: "uppercased MAC",
			in:    "AA:BB:CC:DD:EE:FF",
			exp:   "aa:bb:cc:dd:ee:ff",
		},
		{
			descr: "MAC with dashes instead of colons",
			in:    "AA-BB-CC-DD-EE-FF",
			exp:   "aa:bb:cc:dd:ee:ff",
		},
		{
			descr: "already normalized MAC",
			in:    "aa:bb:cc:dd:ee:ff",
			exp:   "aa:bb:cc:dd:ee:ff",
		},
	}

	for i, spec := range specs {
		c.Logf("%d. %s", i, spec.descr)
		got := network.NormalizeMACAddress(spec.in)
		c.Assert(got, gc.Equals, spec.exp)
	}
}

func getInterFaceInfos() network.InterfaceInfos {
	return network.InterfaceInfos{
		{
			DeviceIndex:   0,
			InterfaceName: "br-bond0",
			InterfaceType: network.BondDevice,
		},
		{
			DeviceIndex:   1,
			InterfaceName: "eth2",
			InterfaceType: network.EthernetDevice,
		},
		{
			DeviceIndex:         2,
			InterfaceName:       "bond0",
			ParentInterfaceName: "br-bond0",
			InterfaceType:       network.BondDevice,
		},
		{
			DeviceIndex:         3,
			InterfaceName:       "eth0",
			ParentInterfaceName: "bond0",
			InterfaceType:       network.BondDevice,
		},
		{
			DeviceIndex:         4,
			InterfaceName:       "eth1",
			ParentInterfaceName: "bond0",
			InterfaceType:       network.BondDevice,
		},
	}
}
