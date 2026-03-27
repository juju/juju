// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/collections/transform"
	"github.com/juju/tc"

	corenetwork "github.com/juju/juju/core/network"
	networkinternal "github.com/juju/juju/domain/network/internal"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/internal/uuid"
)

type infoSuite struct {
	linkLayerBaseSuite
	relationCount int
}

func TestInfoSuite(t *testing.T) {
	tc.Run(t, &infoSuite{})
}

func (s *infoSuite) TestGetUnitEndpointAddresses(c *tc.C) {
	// Arrange
	nodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(c, nodeUUID, "eth0", "00:11:22:33:44:55", corenetwork.EthernetDevice)
	spaceUUID := corenetwork.AlphaSpaceId.String()
	cidr := "10.0.0.0/24"
	subnetUUID := s.addSubnet(c, cidr, spaceUUID)
	expectedAddr := "10.0.0.1"
	s.addIPAddressWithSubnetAndScope(c, deviceUUID, nodeUUID, subnetUUID, expectedAddr, corenetwork.ScopeCloudLocal)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	// Add endpoint
	endpointName := "endpoint1"
	s.addApplicationEndpoint(c, appUUID, charmUUID, endpointName, "")

	// Act
	endpointAddresses, err := s.state.GetUnitEndpointNetworkAddresses(
		c.Context(), string(unitUUID), []string{endpointName},
	)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(normalizeEndpointAddresses(endpointAddresses), tc.DeepEquals, []networkinternal.EndpointAddresses{{
		EndpointName: endpointName,
		Addresses: []networkinternal.UnitAddress{{
			SpaceAddress: corenetwork.SpaceAddress{
				MachineAddress: corenetwork.MachineAddress{
					Value: expectedAddr,
					CIDR:  cidr,
					Scope: corenetwork.ScopeCloudLocal,
				},
				SpaceID: corenetwork.SpaceUUID(spaceUUID),
			},
			DeviceName: "eth0",
			MACAddress: "00:11:22:33:44:55",
			DeviceType: corenetwork.EthernetDevice,
		}},
	}})
}

func (s *infoSuite) TestGetUnitEndpointAddressesMultipleEndpoints(c *tc.C) {
	// Arrange
	nodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(c, nodeUUID, "eth0", "00:11:22:33:44:55", corenetwork.EthernetDevice)
	space1UUID := s.addSpace(c)
	space2UUID := s.addSpace(c)
	subnet1UUID := s.addSubnet(c, "10.0.0.0/24", space1UUID)
	subnet2UUID := s.addSubnet(c, "10.0.1.0/24", space2UUID)

	s.addIPAddressWithSubnetAndScope(c, deviceUUID, nodeUUID, subnet1UUID, "10.0.0.1", corenetwork.ScopeCloudLocal)
	s.addIPAddressWithSubnetAndScope(c, deviceUUID, nodeUUID, subnet2UUID, "10.0.1.1", corenetwork.ScopeCloudLocal)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, space1UUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	// Add endpoints
	s.addApplicationEndpoint(c, appUUID, charmUUID, "endpoint1", space1UUID)
	s.addApplicationEndpoint(c, appUUID, charmUUID, "endpoint2", space2UUID)

	// Act
	endpointAddresses, err := s.state.GetUnitEndpointNetworkAddresses(
		c.Context(), string(unitUUID), []string{"endpoint1", "endpoint2"},
	)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(normalizeEndpointAddresses(endpointAddresses), tc.SameContents, []networkinternal.EndpointAddresses{{
		EndpointName: "endpoint1",
		Addresses: []networkinternal.UnitAddress{{
			SpaceAddress: corenetwork.SpaceAddress{
				MachineAddress: corenetwork.MachineAddress{
					Value: "10.0.0.1",
					CIDR:  "10.0.0.0/24",
					Scope: corenetwork.ScopeCloudLocal,
				},
				SpaceID: corenetwork.SpaceUUID(space1UUID),
			},
			DeviceName: "eth0",
			MACAddress: "00:11:22:33:44:55",
			DeviceType: corenetwork.EthernetDevice,
		}},
	}, {
		EndpointName: "endpoint2",
		Addresses: []networkinternal.UnitAddress{{
			SpaceAddress: corenetwork.SpaceAddress{
				MachineAddress: corenetwork.MachineAddress{
					Value: "10.0.1.1",
					CIDR:  "10.0.1.0/24",
					Scope: corenetwork.ScopeCloudLocal,
				},
				SpaceID: corenetwork.SpaceUUID(space2UUID),
			},
			DeviceName: "eth0",
			MACAddress: "00:11:22:33:44:55",
			DeviceType: corenetwork.EthernetDevice,
		}},
	}})
}

func (s *infoSuite) TestGetUnitEndpointAddressesCaasUnit(c *tc.C) {
	// Arrange
	podNodeUUID := s.addNetNode(c)
	svcNodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(c, podNodeUUID, "eth0", "00:11:22:33:44:55", corenetwork.EthernetDevice)
	spaceUUID := s.addSpace(c)
	cidr := "10.0.0.0/24"
	subnetUUID := s.addSubnet(c, cidr, spaceUUID)

	// Add pod address (machine local)
	s.addIPAddressWithSubnetAndScope(c, deviceUUID, podNodeUUID, subnetUUID, "10.0.0.1", corenetwork.ScopeMachineLocal)

	// Add service address (public)
	svcDeviceUUID := s.addLinkLayerDevice(c, svcNodeUUID, "eth1", "00:11:22:33:44:66", corenetwork.EthernetDevice)
	s.addIPAddressWithSubnetAndScope(c, svcDeviceUUID, svcNodeUUID, subnetUUID, "10.0.0.2",
		corenetwork.ScopeCloudLocal)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, podNodeUUID)
	s.addK8sService(c, svcNodeUUID, appUUID)

	// Add endpoint
	endpointName := "endpoint1"
	s.addApplicationEndpoint(c, appUUID, charmUUID, endpointName, spaceUUID)

	// Act
	endpointAddresses, err := s.state.GetUnitEndpointNetworkAddresses(
		c.Context(), string(unitUUID), []string{endpointName},
	)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(endpointAddresses, tc.HasLen, 1)
	c.Check(endpointAddresses[0].EndpointName, tc.Equals, endpointName)
	c.Check(normalizeUnitAddresses(endpointAddresses[0].Addresses), tc.SameContents, []networkinternal.UnitAddress{{
		SpaceAddress: corenetwork.SpaceAddress{
			MachineAddress: corenetwork.MachineAddress{
				Value: "10.0.0.1",
				CIDR:  "10.0.0.0/24",
				Scope: corenetwork.ScopeMachineLocal,
			},
			SpaceID: corenetwork.SpaceUUID(spaceUUID),
		},
		DeviceName: "eth0",
		MACAddress: "00:11:22:33:44:55",
		DeviceType: corenetwork.EthernetDevice,
	}, {
		SpaceAddress: corenetwork.SpaceAddress{
			MachineAddress: corenetwork.MachineAddress{
				Value: "10.0.0.2",
				CIDR:  "10.0.0.0/24",
				Scope: corenetwork.ScopeCloudLocal,
			},
			SpaceID: corenetwork.SpaceUUID(spaceUUID),
		},
		DeviceName: "eth1",
		MACAddress: "00:11:22:33:44:66",
		DeviceType: corenetwork.EthernetDevice,
	}})
}

func (s *infoSuite) TestGetUnitEndpointAddressesNoAddresses(c *tc.C) {
	// Arrange
	nodeUUID := s.addNetNode(c)
	spaceUUID := s.addSpace(c)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	// Add endpoint
	endpointName := "endpoint1"
	s.addApplicationEndpoint(c, appUUID, charmUUID, endpointName, spaceUUID)

	// Act
	endpointAddresses, err := s.state.GetUnitEndpointNetworkAddresses(
		c.Context(), string(unitUUID), []string{endpointName},
	)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(endpointAddresses, tc.SameContents, []networkinternal.EndpointAddresses{{
		EndpointName: endpointName,
		// No addresses.
	}})
}

func (s *infoSuite) TestGetUnitAddresses(c *tc.C) {
	// Arrange
	nodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(c, nodeUUID, "eth0",
		"00:11:22:33:44:55", corenetwork.EthernetDevice)
	spaceUUID := corenetwork.AlphaSpaceId.String()
	cidr := "10.0.0.0/24"
	subnetUUID := s.addSubnet(c, cidr, spaceUUID)
	expectedAddr := "10.0.0.1"
	s.addIPAddressWithSubnetAndScope(c, deviceUUID, nodeUUID, subnetUUID,
		expectedAddr, corenetwork.ScopeCloudLocal)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	// Act
	addresses, err := s.state.GetUnitNetworkAddresses(c.Context(), string(unitUUID))

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(normalizeUnitAddresses(addresses), tc.DeepEquals, []networkinternal.UnitAddress{{
		SpaceAddress: corenetwork.SpaceAddress{
			MachineAddress: corenetwork.MachineAddress{
				Value: expectedAddr,
				CIDR:  cidr,
				Scope: corenetwork.ScopeCloudLocal,
			},
			SpaceID: corenetwork.SpaceUUID(spaceUUID),
		},
		DeviceName: "eth0",
		MACAddress: "00:11:22:33:44:55",
		DeviceType: corenetwork.EthernetDevice,
	}})
}

func (s *infoSuite) TestGetUnitAddressesCaasUnit(c *tc.C) {
	// Arrange
	podNodeUUID := s.addNetNode(c)
	svcNodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(c, podNodeUUID, "eth0",
		"00:11:22:33:44:55", corenetwork.EthernetDevice)
	spaceUUID := corenetwork.AlphaSpaceId.String()
	cidr := "10.0.0.0/24"
	subnetUUID := s.addSubnet(c, cidr, spaceUUID)
	s.addIPAddressWithSubnetAndScope(c, deviceUUID, podNodeUUID, subnetUUID,
		"10.0.0.1", corenetwork.ScopeMachineLocal)

	svcDeviceUUID := s.addLinkLayerDevice(c, svcNodeUUID, "eth1",
		"00:11:22:33:44:66", corenetwork.EthernetDevice)
	s.addIPAddressWithSubnetAndScope(c, svcDeviceUUID, svcNodeUUID, subnetUUID,
		"10.0.0.2", corenetwork.ScopeCloudLocal)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, podNodeUUID)
	s.addK8sService(c, svcNodeUUID, appUUID)

	// Act
	addresses, err := s.state.GetUnitNetworkAddresses(c.Context(), string(unitUUID))

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(normalizeUnitAddresses(addresses), tc.SameContents, []networkinternal.UnitAddress{{
		SpaceAddress: corenetwork.SpaceAddress{
			MachineAddress: corenetwork.MachineAddress{
				Value: "10.0.0.1",
				CIDR:  cidr,
				Scope: corenetwork.ScopeMachineLocal,
			},
			SpaceID: corenetwork.SpaceUUID(spaceUUID),
		},
		DeviceName: "eth0",
		MACAddress: "00:11:22:33:44:55",
		DeviceType: corenetwork.EthernetDevice,
	}, {
		SpaceAddress: corenetwork.SpaceAddress{
			MachineAddress: corenetwork.MachineAddress{
				Value: "10.0.0.2",
				CIDR:  cidr,
				Scope: corenetwork.ScopeCloudLocal,
			},
			SpaceID: corenetwork.SpaceUUID(spaceUUID),
		},
		DeviceName: "eth1",
		MACAddress: "00:11:22:33:44:66",
		DeviceType: corenetwork.EthernetDevice,
	}})
}

func (s *infoSuite) TestGetUnitRelationEndpointName(c *tc.C) {
	nodeUUID := s.addNetNode(c)
	charmUUID := s.addCharm(c)
	spaceUUID := s.addSpace(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	endpointName := "database"
	endpointUUID := s.addApplicationEndpoint(
		c, appUUID, charmUUID, endpointName, spaceUUID,
	)
	relationUUID := s.addRelation(c)
	relationEndpointUUID := s.addRelationEndpoint(c, relationUUID, endpointUUID)
	s.addRelationUnit(c, relationEndpointUUID, string(unitUUID))

	name, err := s.state.GetUnitRelationEndpointName(
		c.Context(), string(unitUUID), relationUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(name, tc.Equals, endpointName)
}

func (s *infoSuite) TestGetUnitRelationEndpointNameRelationNotFound(c *tc.C) {
	nodeUUID := s.addNetNode(c)
	charmUUID := s.addCharm(c)
	spaceUUID := s.addSpace(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	name, err := s.state.GetUnitRelationEndpointName(
		c.Context(), string(unitUUID), uuid.MustNewUUID().String(),
	)
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
	c.Check(name, tc.Equals, "")
}

// TestGetAllSpacesForEndpoints tests retrieving space information for endpoints
func (s *infoSuite) TestGetAllSpacesForEndpoints(c *tc.C) {
	// Arrange
	nodeUUID := s.addNetNode(c)
	defaultSpace := s.addSpace(c)
	specificSpace := s.addSpace(c)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, defaultSpace)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	// Add endpoints
	endpoint1Name := "endpoint1"
	endpoint2Name := "endpoint2"
	s.addApplicationEndpoint(c, appUUID, charmUUID, endpoint1Name, "")
	s.addApplicationEndpoint(c, appUUID, charmUUID, endpoint2Name, specificSpace)

	// Act
	spaces, err := s.state.getAllSpacesForEndpoints(c.Context(), string(unitUUID), []string{endpoint1Name, endpoint2Name})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(spaces, tc.HasLen, 2)

	// Verify both endpoints are returned with correct space
	c.Assert(spaces, tc.SameContents, []spaceEndpoint{{
		EndpointName: endpoint1Name,
		SpaceUUID:    defaultSpace,
	}, {
		EndpointName: endpoint2Name,
		SpaceUUID:    specificSpace,
	}})
}

// TestGetAllUnitAddressesInSpaces tests retrieving addresses for a unit in specified spaces
func (s *infoSuite) TestGetAllUnitAddressesInSpaces(c *tc.C) {
	// Arrange
	nodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(c, nodeUUID, "eth0", "00:11:22:33:44:55", corenetwork.EthernetDevice)

	// First expected address
	spaceUUID1 := s.addSpace(c)
	subnetUUID1 := s.addSubnet(c, "10.0.0.0/24", spaceUUID1)
	s.addIPAddressWithSubnet(c, deviceUUID, nodeUUID, subnetUUID1, "10.0.0.1")

	// second expected address
	spaceUUID2 := s.addSpace(c)
	subnetUUID2 := s.addSubnet(c, "10.1.0.0/24", spaceUUID2)
	s.addIPAddressWithSubnet(c, deviceUUID, nodeUUID, subnetUUID2, "10.1.0.1")

	// Not expected address
	s.addIPAddressWithSubnet(c, deviceUUID, nodeUUID, s.addSubnet(c, "10.2.0.0/24", s.addSpace(c)), "10.2.0.1")

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, corenetwork.AlphaSpaceId.String())
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	// Act
	addresses, err := s.state.getAllUnitAddressesInSpaces(c.Context(), string(unitUUID), []string{spaceUUID1, spaceUUID2})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	addresses = transform.Slice(addresses, func(addr networkinternal.UnitAddress) networkinternal.UnitAddress {
		return networkinternal.UnitAddress{
			SpaceAddress: corenetwork.SpaceAddress{
				MachineAddress: corenetwork.MachineAddress{
					Value: addr.Value,
					CIDR:  addr.CIDR,
				},
				SpaceID: addr.SpaceID,
			},
			DeviceName: addr.DeviceName,
			MACAddress: addr.MACAddress,
			DeviceType: addr.DeviceType,
		}
	})
	c.Assert(addresses, tc.SameContents, []networkinternal.UnitAddress{{
		SpaceAddress: corenetwork.SpaceAddress{
			MachineAddress: corenetwork.MachineAddress{
				Value: "10.0.0.1",
				CIDR:  "10.0.0.0/24",
			},
			SpaceID: corenetwork.SpaceUUID(spaceUUID1),
		},
		DeviceName: "eth0",
		MACAddress: "00:11:22:33:44:55",
		DeviceType: corenetwork.EthernetDevice,
	}, {
		SpaceAddress: corenetwork.SpaceAddress{
			MachineAddress: corenetwork.MachineAddress{
				Value: "10.1.0.1",
				CIDR:  "10.1.0.0/24",
			},
			SpaceID: corenetwork.SpaceUUID(spaceUUID2),
		},
		DeviceName: "eth0",
		MACAddress: "00:11:22:33:44:55",
		DeviceType: corenetwork.EthernetDevice,
	}})
}

// TestIsCaasUnit tests checking if a unit is a CAAS unit
func (s *infoSuite) TestIsCaasUnit(c *tc.C) {
	// Arrange
	nodeUUID := s.addNetNode(c)
	svcNodeUUID := s.addNetNode(c)

	charmUUID := s.addCharm(c)
	iaasAppUUID := s.addApplication(c, charmUUID, corenetwork.AlphaSpaceId.String())
	caasAppUUID := s.addApplication(c, charmUUID, corenetwork.AlphaSpaceId.String())
	caasUnitUUID := s.addUnit(c, caasAppUUID, charmUUID, nodeUUID)
	iaasUnitUUID := s.addUnit(c, iaasAppUUID, charmUUID, nodeUUID)
	s.addK8sService(c, svcNodeUUID, caasAppUUID)

	// Act - Check both Caas and Iaas Apps
	isCaasIaaS, err := s.state.IsCaasUnit(c.Context(), string(iaasUnitUUID))
	c.Assert(err, tc.ErrorIsNil)
	isCaasCaas, err := s.state.IsCaasUnit(c.Context(), string(caasUnitUUID))
	c.Assert(err, tc.ErrorIsNil)

	// Assert
	c.Assert(isCaasIaaS, tc.Equals, false)
	c.Assert(isCaasCaas, tc.Equals, true)
}

// TestGetUnitEgressSubnetsWithMultipleCIDRs tests retrieving egress subnets
// when a unit has multiple relations with egress CIDRs.
func (s *infoSuite) TestGetUnitEgressSubnetsWithMultipleCIDRs(c *tc.C) {
	// Arrange
	nodeUUID := s.addNetNode(c)
	charmUUID := s.addCharm(c)
	spaceUUID := s.addSpace(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	endpoint1UUID := s.addApplicationEndpoint(c, appUUID, charmUUID, "endpoint1", spaceUUID)
	endpoint2UUID := s.addApplicationEndpoint(c, appUUID, charmUUID, "endpoint2", spaceUUID)

	relation1UUID := s.addRelation(c)
	relation2UUID := s.addRelation(c)

	relationEndpoint1UUID := s.addRelationEndpoint(c, relation1UUID, endpoint1UUID)
	relationEndpoint2UUID := s.addRelationEndpoint(c, relation2UUID, endpoint2UUID)

	s.addRelationUnit(c, relationEndpoint1UUID, string(unitUUID))
	s.addRelationUnit(c, relationEndpoint2UUID, string(unitUUID))

	// Add egress CIDRs.
	s.addRelationNetworkEgress(c, relation1UUID, "10.0.1.0/24")
	s.addRelationNetworkEgress(c, relation1UUID, "10.0.2.0/24")
	s.addRelationNetworkEgress(c, relation2UUID, "10.0.3.0/24")

	// Act
	cidrs, err := s.state.GetUnitEgressSubnets(c.Context(), string(unitUUID))

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cidrs, tc.SameContents, []string{"10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"})
}

// TestGetUnitEgressSubnetsWithNoRelations tests retrieving egress subnets when
// a unit has no relations
func (s *infoSuite) TestGetUnitEgressSubnetsWithNoRelations(c *tc.C) {
	// Arrange
	nodeUUID := s.addNetNode(c)
	charmUUID := s.addCharm(c)
	spaceUUID := s.addSpace(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	// Act
	cidrs, err := s.state.GetUnitEgressSubnets(c.Context(), string(unitUUID))

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cidrs, tc.HasLen, 0)
}

// TestGetUnitEgressSubnetsWithRelationsButNoEgress tests retrieving egress
// subnets when relations exist but have no egress CIDRs.
func (s *infoSuite) TestGetUnitEgressSubnetsWithRelationsButNoEgress(c *tc.C) {
	// Arrange
	nodeUUID := s.addNetNode(c)
	charmUUID := s.addCharm(c)
	spaceUUID := s.addSpace(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	endpointUUID := s.addApplicationEndpoint(c, appUUID, charmUUID, "endpoint1", spaceUUID)
	relationUUID := s.addRelation(c)
	relationEndpointUUID := s.addRelationEndpoint(c, relationUUID, endpointUUID)
	s.addRelationUnit(c, relationEndpointUUID, string(unitUUID))

	// Act
	cidrs, err := s.state.GetUnitEgressSubnets(c.Context(), string(unitUUID))

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cidrs, tc.HasLen, 0)
}

// TestGetUnitEgressSubnetsDeduplicated tests that duplicate CIDRs across
// relations are deduplicated.
func (s *infoSuite) TestGetUnitEgressSubnetsDeduplicated(c *tc.C) {
	// Arrange
	nodeUUID := s.addNetNode(c)
	charmUUID := s.addCharm(c)
	spaceUUID := s.addSpace(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	endpoint1UUID := s.addApplicationEndpoint(c, appUUID, charmUUID, "endpoint1", spaceUUID)
	endpoint2UUID := s.addApplicationEndpoint(c, appUUID, charmUUID, "endpoint2", spaceUUID)

	relation1UUID := s.addRelation(c)
	relation2UUID := s.addRelation(c)

	relationEndpoint1UUID := s.addRelationEndpoint(c, relation1UUID, endpoint1UUID)
	relationEndpoint2UUID := s.addRelationEndpoint(c, relation2UUID, endpoint2UUID)

	s.addRelationUnit(c, relationEndpoint1UUID, string(unitUUID))
	s.addRelationUnit(c, relationEndpoint2UUID, string(unitUUID))

	// Add the same CIDR to both relations.
	s.addRelationNetworkEgress(c, relation1UUID, "10.0.1.0/24")
	s.addRelationNetworkEgress(c, relation2UUID, "10.0.1.0/24")

	// Act
	cidrs, err := s.state.GetUnitEgressSubnets(c.Context(), string(unitUUID))

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cidrs, tc.HasLen, 1)
	c.Check(cidrs, tc.DeepEquals, []string{"10.0.1.0/24"})
}

func (s *infoSuite) TestGetRelationEgressSubnets(c *tc.C) {
	relationUUID := s.addRelation(c)

	s.addRelationNetworkEgress(c, relationUUID, "10.0.1.0/24")
	s.addRelationNetworkEgress(c, relationUUID, "10.0.2.0/24")

	cidrs, err := s.state.GetRelationEgressSubnets(c.Context(), relationUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cidrs, tc.SameContents, []string{"10.0.1.0/24", "10.0.2.0/24"})
}

func (s *infoSuite) TestGetRelationEgressSubnetsEmpty(c *tc.C) {
	relationUUID := s.addRelation(c)

	cidrs, err := s.state.GetRelationEgressSubnets(c.Context(), relationUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cidrs, tc.HasLen, 0)
}

// Helper methods

func normalizeEndpointAddresses(
	addresses []networkinternal.EndpointAddresses,
) []networkinternal.EndpointAddresses {
	return transform.Slice(addresses, func(addr networkinternal.EndpointAddresses) networkinternal.EndpointAddresses {
		addr.Addresses = normalizeUnitAddresses(addr.Addresses)
		return addr
	})
}

func normalizeUnitAddresses(addresses []networkinternal.UnitAddress) []networkinternal.UnitAddress {
	return transform.Slice(addresses, func(addr networkinternal.UnitAddress) networkinternal.UnitAddress {
		return networkinternal.UnitAddress{
			SpaceAddress: corenetwork.SpaceAddress{
				MachineAddress: corenetwork.MachineAddress{
					Value: addr.Value,
					CIDR:  addr.CIDR,
					Scope: addr.Scope,
				},
				SpaceID: addr.SpaceID,
			},
			DeviceName: addr.DeviceName,
			MACAddress: addr.MACAddress,
			DeviceType: addr.DeviceType,
		}
	})
}

// addApplicationEndpoint creates a charm relation and an application endpoint
// in the database, returning its UUID.
func (s *infoSuite) addApplicationEndpoint(c *tc.C, appUUID, charmUUID, endpointName, spaceUUID string) string {
	// Add charm relation
	relationUUID := uuid.MustNewUUID().String()
	s.query(c, `INSERT INTO charm_relation (uuid, charm_uuid, name, role_id, scope_id) VALUES (?, ?, ?, 0, 0)`,
		relationUUID, charmUUID, endpointName)

	// Add application endpoint
	var spacePtr *string
	if spaceUUID != "" {
		spacePtr = &spaceUUID
	}
	appEndpointUUID := uuid.MustNewUUID().String()
	s.query(c, `
INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid, space_uuid) 
VALUES (?, ?,?, ?)`,
		appEndpointUUID, appUUID, relationUUID, spacePtr)
	return appEndpointUUID
}

// addRelation creates a relation in the database, returning its UUID.
func (s *infoSuite) addRelation(c *tc.C) string {
	relationUUID := uuid.MustNewUUID().String()
	s.relationCount++
	s.query(c, `INSERT INTO relation (uuid, life_id, relation_id, scope_id) VALUES (?, 0, ?, 0)`,
		relationUUID, s.relationCount)
	return relationUUID
}

// addRelationEndpoint creates a relation_endpoint linking a relation to an application endpoint.
func (s *infoSuite) addRelationEndpoint(c *tc.C, relationUUID, endpointUUID string) string {
	relationEndpointUUID := uuid.MustNewUUID().String()
	s.query(c, `INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid) VALUES (?, ?, ?)`,
		relationEndpointUUID, relationUUID, endpointUUID)
	return relationEndpointUUID
}

// addRelationUnit creates a relation_unit linking a relation endpoint to a unit.
func (s *infoSuite) addRelationUnit(c *tc.C, relationEndpointUUID, unitUUID string) {
	relationUnitUUID := uuid.MustNewUUID().String()
	s.query(c, `INSERT INTO relation_unit (uuid, relation_endpoint_uuid, unit_uuid) VALUES (?, ?, ?)`,
		relationUnitUUID, relationEndpointUUID, unitUUID)
}

// addRelationNetworkEgress adds an egress CIDR to a relation.
func (s *infoSuite) addRelationNetworkEgress(c *tc.C, relationUUID, cidr string) {
	s.query(c, `INSERT INTO relation_network_egress (relation_uuid, cidr) VALUES (?, ?)`,
		relationUUID, cidr)
}
