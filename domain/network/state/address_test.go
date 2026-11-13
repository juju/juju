// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	corenetwork "github.com/juju/juju/core/network"
)

type addressSuite struct {
	linkLayerBaseSuite
}

func TestAddressSuite(t *testing.T) {
	tc.Run(t, &addressSuite{})
}

func (s *addressSuite) TestGetNetNodeAddresses(c *tc.C) {
	// Arrange
	nodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(c, nodeUUID, "eth0", "00:11:22:33:44:55", corenetwork.EthernetDevice)
	spaceUUID := s.addSpace(c)
	subnetUUID := s.addSubnet(c, "10.0.0.0/24", spaceUUID)
	s.addIPAddressWithSubnetAndScope(c, deviceUUID, nodeUUID, subnetUUID, "10.0.0.1", corenetwork.ScopeMachineLocal)

	// Act
	addr, err := s.state.GetNetNodeAddresses(c.Context(), nodeUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addr, tc.DeepEquals, []corenetwork.DeviceAddress{
		{
			SpaceAddress: corenetwork.SpaceAddress{
				SpaceID: corenetwork.SpaceUUID(spaceUUID),
				Origin:  corenetwork.OriginProvider,
				MachineAddress: corenetwork.MachineAddress{
					Value:      "10.0.0.1",
					CIDR:       "10.0.0.0/24",
					Type:       corenetwork.IPv4Address,
					Scope:      corenetwork.ScopeMachineLocal,
					ConfigType: corenetwork.ConfigStatic,
				},
			},
			DeviceUUID: deviceUUID,
		},
	})
}

func (s *addressSuite) TestGetNetNodeAddressesNoAddresses(c *tc.C) {
	// Arrange: add address to another net node
	nodeUUID := s.addNetNode(c)
	otherNodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(c, otherNodeUUID, "eth0", "00:11:22:33:44:55", corenetwork.EthernetDevice)
	spaceUUID := s.addSpace(c)
	subnetUUID := s.addSubnet(c, "10.0.0.0/24", spaceUUID)
	s.addIPAddressWithSubnet(c, deviceUUID, otherNodeUUID, subnetUUID, "10.0.0.1")

	// Act
	addr, err := s.state.GetNetNodeAddresses(c.Context(), nodeUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr, tc.DeepEquals, []corenetwork.DeviceAddress{})
}

// TestGetNetNodeAddressesDefaultGateway checks that the device facts needed to
// identify the default route source are returned with each net node address,
// and that the state does not itself flag the source address.
func (s *addressSuite) TestGetNetNodeAddressesDefaultGateway(c *tc.C) {
	// Arrange
	nodeUUID := s.addNetNode(c)
	eth0UUID := s.addLinkLayerDevice(c, nodeUUID, "eth0", "00:11:22:33:44:55", corenetwork.EthernetDevice)
	eth1UUID := s.addLinkLayerDevice(c, nodeUUID, "eth1", "00:11:22:33:44:66", corenetwork.EthernetDevice)
	s.setDefaultGateway(c, eth0UUID, "10.1.0.1")

	spaceUUID := s.addSpace(c)
	eth0SubnetUUID := s.addSubnet(c, "10.1.0.0/24", spaceUUID)
	eth1SubnetUUID := s.addSubnet(c, "10.0.0.0/24", spaceUUID)
	s.addIPAddressWithSubnetAndScope(c, eth0UUID, nodeUUID, eth0SubnetUUID,
		"10.1.0.123", corenetwork.ScopeCloudLocal)
	secondaryUUID := s.addIPAddressWithSubnetAndScope(c, eth0UUID, nodeUUID,
		eth0SubnetUUID, "10.1.0.124", corenetwork.ScopeCloudLocal)
	s.query(c, `UPDATE ip_address SET is_secondary = true WHERE uuid = ?`,
		secondaryUUID)
	s.addIPAddressWithSubnetAndScope(c, eth1UUID, nodeUUID, eth1SubnetUUID,
		"10.0.0.123", corenetwork.ScopeCloudLocal)

	// Act
	addrs, err := s.state.GetNetNodeAddresses(c.Context(), nodeUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	expected := func(
		value, cidr, deviceUUID string,
		isSecondary, isDefaultGatewayDevice bool,
		gatewayAddress string,
	) corenetwork.DeviceAddress {
		return corenetwork.DeviceAddress{
			SpaceAddress: corenetwork.SpaceAddress{
				SpaceID: corenetwork.SpaceUUID(spaceUUID),
				Origin:  corenetwork.OriginProvider,
				MachineAddress: corenetwork.MachineAddress{
					Value:       value,
					CIDR:        cidr,
					Type:        corenetwork.IPv4Address,
					Scope:       corenetwork.ScopeCloudLocal,
					ConfigType:  corenetwork.ConfigStatic,
					IsSecondary: isSecondary,
				},
			},
			DeviceUUID:             deviceUUID,
			IsDefaultGatewayDevice: isDefaultGatewayDevice,
			GatewayAddress:         gatewayAddress,
		}
	}
	c.Check(addrs, tc.SameContents, []corenetwork.DeviceAddress{
		expected("10.1.0.123", "10.1.0.0/24", eth0UUID, false, true, "10.1.0.1"),
		expected("10.1.0.124", "10.1.0.0/24", eth0UUID, true, true, "10.1.0.1"),
		expected("10.0.0.123", "10.0.0.0/24", eth1UUID, false, false, ""),
	})
}
