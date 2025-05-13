// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network"
)

type typesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&typesSuite{})

func (s *typesSuite) TestNetInterfaceToDMLSuccess(c *gc.C) {
	dev := getNetInterface()

	dml, err := netInterfaceToDML(dev, "some-node-uuid", map[string]string{"eth0": "some-device-uuid"})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(dml, gc.DeepEquals, linkLayerDeviceDML{
		UUID:              "some-device-uuid",
		NetNodeUUID:       "some-node-uuid",
		Name:              "eth0",
		MTU:               ptr(int64(1500)),
		MACAddress:        ptr("00:00:00:00:00:00"),
		DeviceTypeID:      2,
		VirtualPortTypeID: 1,
		IsAutoStart:       true,
		IsEnabled:         true,
		IsDefaultGateway:  true,
		GatewayAddress:    ptr("192.168.0.1"),
		VlanTag:           0,
	})
}

func (s *typesSuite) TestNetInterfaceToDMLBadDeviceTypeError(c *gc.C) {
	dev := getNetInterface()
	dev.Type = "bad-type"

	_, err := netInterfaceToDML(dev, "some-node-uuid", map[string]string{
		"eth0": "some-device-uuid",
	})
	c.Assert(err, gc.ErrorMatches, "unsupported device type.*")
}

func (s *typesSuite) TestNetInterfaceToDMLBadVirtualPortTypeError(c *gc.C) {
	dev := getNetInterface()
	dev.VirtualPortType = "bad-type"

	_, err := netInterfaceToDML(dev, "some-node-uuid", map[string]string{"eth0": "some-device-uuid"})
	c.Assert(err, gc.ErrorMatches, "unsupported virtual port type.*")
}

func (s *typesSuite) TestNetAddrToDMLSuccess(c *gc.C) {
	addr := getNetAddr()

	dml, err := netAddrToDML(
		addr,
		map[string]string{"eth0": "some-device-uuid"},
		map[string]string{"10.0.0.13/24": "some-addr-uuid"},
	)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(dml, gc.DeepEquals, ipAddressDML{
		UUID:         "some-addr-uuid",
		DeviceUUID:   "some-device-uuid",
		AddressValue: "10.0.0.13/24",
		SubnetUUID:   nil,
		TypeID:       0,
		ConfigTypeID: 1,
		OriginID:     0,
		ScopeID:      4,
		IsSecondary:  false,
		IsShadow:     false,
	})
}

func (s *typesSuite) TestNetAddrToDMLBadAddressTypeError(c *gc.C) {
	addr := getNetAddr()
	addr.AddressType = "bad-type"

	_, err := netAddrToDML(
		addr,
		map[string]string{"eth0": "some-device-uuid"},
		map[string]string{"10.0.0.13/24": "some-addr-uuid"},
	)
	c.Assert(err, gc.ErrorMatches, "unsupported address type.*")
}

func getNetInterface() network.NetInterface {
	return network.NetInterface{
		Name:             "eth0",
		MTU:              ptr(int64(1500)),
		MACAddress:       ptr("00:00:00:00:00:00"),
		Type:             corenetwork.EthernetDevice,
		VirtualPortType:  corenetwork.OvsPort,
		IsAutoStart:      true,
		IsEnabled:        true,
		GatewayAddress:   ptr("192.168.0.1"),
		IsDefaultGateway: true,
		VLANTag:          0,

		// TODO (manadart 2025-05-05): Handle the translations below as
		// additional *DML types.

		ParentDeviceName: "",
		ProviderID:       nil,
		DNSSearchDomains: nil,
		DNSAddresses:     nil,
	}
}

func getNetAddr() network.NetAddr {
	return network.NetAddr{
		InterfaceName: "eth0",
		AddressValue:  "10.0.0.13/24",

		// TODO (manadart 2025--05-08): This, combined with the CIDR determined
		// from the address will be used to determine a subnet UUID (if extant)
		// when we are resolving as part of network detection.
		ProviderSubnetID: nil,
		AddressType:      corenetwork.IPv4Address,
		ConfigType:       corenetwork.ConfigDHCP,
		Origin:           corenetwork.OriginMachine,
		Scope:            corenetwork.ScopeCloudLocal,
		IsSecondary:      false,
		IsShadow:         false,

		// TODO (manadart 2025-05-08): Handle the translations below as
		// additional *DML types.

		ProviderID: nil,
	}
}

func ptr[T any](v T) *T {
	return &v
}
