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
	c.Check(addr, tc.DeepEquals, corenetwork.SpaceAddresses{
		{
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
	c.Assert(addr, tc.DeepEquals, corenetwork.SpaceAddresses{})
}
