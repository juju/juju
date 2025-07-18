// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/testhelpers"
)

type typesSuite struct {
	testhelpers.IsolationSuite
}

func TestTypesSuite(t *testing.T) {
	tc.Run(t, &typesSuite{})
}

func (s *typesSuite) TestNetInterfaceToDMLSuccess(c *tc.C) {
	dev := getNetInterface()

	nicDML, dnsSearch, dnsAddr, err :=
		netInterfaceToDML(
			dev,
			"some-node-uuid",
			map[string]string{"eth0": "some-device-uuid"},
			getNetAddressTypes(),
		)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(nicDML, tc.DeepEquals, linkLayerDeviceDML{
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

	c.Check(dnsSearch, tc.DeepEquals, []dnsSearchDomainRow{{
		DeviceUUID:   "some-device-uuid",
		SearchDomain: "search.maas.net",
	}})

	c.Check(dnsAddr, tc.DeepEquals, []dnsAddressRow{
		{
			DeviceUUID: "some-device-uuid",
			Address:    "1.1.1.1",
		},
		{
			DeviceUUID: "some-device-uuid",
			Address:    "8.8.8.8",
		},
	})
}

func (s *typesSuite) TestNetInterfaceToDMLBadDeviceTypeError(c *tc.C) {
	dev := getNetInterface()
	dev.Type = "bad-type"

	_, _, _, err := netInterfaceToDML(dev, "some-node-uuid", map[string]string{
		"eth0": "some-device-uuid",
	}, getNetAddressTypes())
	c.Assert(err, tc.ErrorMatches, "unsupported device type.*")
}

func (s *typesSuite) TestNetInterfaceToDMLBadVirtualPortTypeError(c *tc.C) {
	dev := getNetInterface()
	dev.VirtualPortType = "bad-type"

	_, _, _, err := netInterfaceToDML(dev, "some-node-uuid", map[string]string{"eth0": "some-device-uuid"}, getNetAddressTypes())
	c.Assert(err, tc.ErrorMatches, "unsupported virtual port type.*")
}

func (s *typesSuite) TestNetAddrToDMLSuccess(c *tc.C) {
	addr := getNetAddr()

	dml, err := netAddrToDML(
		addr, "some-node-uuid", "some-device-uuid", map[string]string{"10.0.0.13/24": "some-addr-uuid"}, getNetAddressTypes())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(dml, tc.DeepEquals, ipAddressDML{
		UUID:         "some-addr-uuid",
		NodeUUID:     "some-node-uuid",
		DeviceUUID:   "some-device-uuid",
		AddressValue: "10.0.0.13/24",
		SubnetUUID:   nil,
		TypeID:       0,
		ConfigTypeID: 1,
		OriginID:     0,
		ScopeID:      2,
		IsSecondary:  false,
		IsShadow:     false,
	})
}

func (s *typesSuite) TestNetAddrToDMLBadAddressTypeError(c *tc.C) {
	addr := getNetAddr()
	addr.AddressType = "bad-type"

	_, err := netAddrToDML(
		addr, "some-node-uuid", "some-device-uuid", map[string]string{"10.0.0.13/24": "some-addr-uuid"}, getNetAddressTypes())
	c.Assert(err, tc.ErrorMatches, "unsupported address type.*")
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
		DNSSearchDomains: []string{"search.maas.net"},
		DNSAddresses:     []string{"1.1.1.1", "8.8.8.8"},

		// TODO (manadart 2025-05-05): Handle the translations below as
		// additional *DML types.

		ParentDeviceName: "",
		ProviderID:       nil,
	}
}

func getNetAddr() network.NetAddr {
	return network.NetAddr{
		InterfaceName: "eth0",
		AddressValue:  "10.0.0.13/24",

		// TODO (manadart 2025-05-08): This, combined with the CIDR determined
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

// getNetAddressTypes returns the minimal config needed
// for these tests.
func getNetAddressTypes() nameToIDTable {
	return nameToIDTable{
		DeviceMap: map[corenetwork.LinkLayerDeviceType]int{
			corenetwork.EthernetDevice: 2,
		},
		PortMap: map[corenetwork.VirtualPortType]int{
			corenetwork.OvsPort: 1,
		},
		AddrMap: map[corenetwork.AddressType]int{
			corenetwork.IPv4Address: 0,
		},
		AddrConfigMap: map[corenetwork.AddressConfigType]int{
			corenetwork.ConfigDHCP: 1,
		},
		OriginMap: map[corenetwork.Origin]int{
			corenetwork.OriginMachine: 0,
		},
		ScopeMap: map[corenetwork.Scope]int{
			corenetwork.ScopeCloudLocal: 2,
		},
	}
}

func ptr[T any](v T) *T {
	return &v
}
