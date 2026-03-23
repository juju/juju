// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/collections/transform"
	"github.com/juju/tc"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/unitstate/internal"
	"github.com/juju/juju/internal/uuid"
)

type infoSuite struct {
	commitHookBaseSuite
	relationCount int
}

func TestInfoSuite(t *testing.T) {
	tc.Run(t, &infoSuite{})
}

// TestGetUnitRelationNetworkInfos tests the main function that retrieves network information for unit endpoints
func (s *infoSuite) TestGetUnitRelationNetworkInfos(c *tc.C) {
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

	// Arrange: add endpoint
	endpointUUID := s.addApplicationEndpoint(c, appUUID, charmUUID, "endpoint1", "")

	// Arrange: add relation and join.
	relationUUID := s.addRelation(c)
	relationEndpointUUID := s.addRelationEndpoint(c, relationUUID.String(), endpointUUID)
	s.addRelationUnit(c, relationEndpointUUID, string(unitUUID))

	// Act
	infos, err := s.state.GetUnitRelationNetworkInfos(c.Context(), unitUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(infos, tc.SameContents, []internal.RelationNetworkInfo{{
		RelationUUID:   relationUUID,
		IngressAddress: expectedAddr,
		// No egress subnets
	}})
}

// TestGetUnitRelationNetworkInfosMultipleRelations tests retrieving relation network
// information for multiple relations
func (s *infoSuite) TestGetUnitRelationNetworkInfosMultipleRelations(c *tc.C) {
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

	// Arrange: add endpoints
	endpoint1UUID := s.addApplicationEndpoint(c, appUUID, charmUUID, "endpoint1", space1UUID)
	endpoint2UUID := s.addApplicationEndpoint(c, appUUID, charmUUID, "endpoint2", space2UUID)

	// Arrange: add relations and join.
	relation1UUID := s.addRelation(c)
	relation2UUID := s.addRelation(c)
	relationEndpoint1UUID := s.addRelationEndpoint(c, relation1UUID.String(), endpoint1UUID)
	relationEndpoint2UUID := s.addRelationEndpoint(c, relation2UUID.String(), endpoint2UUID)
	s.addRelationUnit(c, relationEndpoint1UUID, string(unitUUID))
	s.addRelationUnit(c, relationEndpoint2UUID, string(unitUUID))

	// Act
	infos, err := s.state.GetUnitRelationNetworkInfos(c.Context(), unitUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(infos, tc.SameContents, []internal.RelationNetworkInfo{
		{
			RelationUUID:   relation1UUID,
			IngressAddress: "10.0.0.1",
			// No egress subnets
		}, {
			RelationUUID:   relation2UUID,
			IngressAddress: "10.0.1.1",
			// No egress subnets
		}})
}

// TestGetUnitRelationNetworkInfosCaasUnit tests retrieving network information for a CAAS unit
func (s *infoSuite) TestGetUnitRelationNetworkInfosCaasUnit(c *tc.C) {
	// Arrange
	podNodeUUID := s.addNetNode(c)
	svcNodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(c, podNodeUUID, "eth0", "00:11:22:33:44:55", corenetwork.EthernetDevice)
	spaceUUID := s.addSpace(c)
	cidr := "10.0.0.0/24"
	subnetUUID := s.addSubnet(c, cidr, spaceUUID)

	// Arrange: add pod address (machine local)
	s.addIPAddressWithSubnetAndScope(c, deviceUUID, podNodeUUID, subnetUUID, "10.0.0.1", corenetwork.ScopeMachineLocal)

	// Arrange: add service address (public)
	svcDeviceUUID := s.addLinkLayerDevice(c, svcNodeUUID, "eth1", "00:11:22:33:44:66", corenetwork.EthernetDevice)
	s.addIPAddressWithSubnetAndScope(c, svcDeviceUUID, svcNodeUUID, subnetUUID, "10.0.0.2",
		corenetwork.ScopeCloudLocal)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, podNodeUUID)
	s.addK8sService(c, svcNodeUUID, appUUID)

	// Arrange: add endpoint
	endpointUUID := s.addApplicationEndpoint(c, appUUID, charmUUID, "endpoint1", spaceUUID)

	// Arrange: add relation and join.
	relationUUID := s.addRelation(c)
	relationEndpointUUID := s.addRelationEndpoint(c, relationUUID.String(), endpointUUID)
	s.addRelationUnit(c, relationEndpointUUID, string(unitUUID))

	// Act
	relationNetworkInfos, err := s.state.GetUnitRelationNetworkInfos(c.Context(), unitUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(relationNetworkInfos, tc.HasLen, 1)
	c.Assert(relationNetworkInfos[0].RelationUUID, tc.Equals, relationUUID)

	// For CAAS units, only non-machine-local addresses should be in ingress addresses
	c.Assert(relationNetworkInfos[0].IngressAddress, tc.Equals, "10.0.0.2")
}

// TestGetUnitRelationNetworkInfosNoAddresses tests retrieving network information when no addresses are available
func (s *infoSuite) TestGetUnitRelationNetworkInfosNoAddresses(c *tc.C) {
	// Arrange
	nodeUUID := s.addNetNode(c)
	spaceUUID := s.addSpace(c)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	// Arrange: add endpoint
	endpointUUID := s.addApplicationEndpoint(c, appUUID, charmUUID, "endpoint1", spaceUUID)

	// Arrange: add relation and join.
	relationUUID := s.addRelation(c)
	relationEndpointUUID := s.addRelationEndpoint(c, relationUUID.String(), endpointUUID)
	s.addRelationUnit(c, relationEndpointUUID, string(unitUUID))

	// Act
	infos, err := s.state.GetUnitRelationNetworkInfos(c.Context(), unitUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(infos, tc.SameContents, []internal.RelationNetworkInfo{{
		RelationUUID: relationUUID,
		// No devices nor ingresses
	}})
}

func (s *infoSuite) TestGetUnitRelationNetworkInfosNetworkingNotSupported(c *tc.C) {
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

	// Arrange: add endpoint
	endpointUUID := s.addApplicationEndpoint(c, appUUID, charmUUID, "endpoint1", spaceUUID)

	// Arrange: add relation and join.
	relationUUID := s.addRelation(c)
	relationEndpointUUID := s.addRelationEndpoint(c, relationUUID.String(), endpointUUID)
	s.addRelationUnit(c, relationEndpointUUID, string(unitUUID))

	// Act
	infos, err := s.state.GetUnitRelationNetworkInfosNetworkingNotSupported(c.Context(), unitUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(infos, tc.SameContents, []internal.RelationNetworkInfo{{
		RelationUUID:   relationUUID,
		IngressAddress: expectedAddr,
		// No devices nor ingresses
	}})

}

func (s *infoSuite) TestGetUnitRelationNetworkInfosNetworkingNotSupportedCaasUnit(c *tc.C) {
	// Arrange
	podNodeUUID := s.addNetNode(c)
	svcNodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(c, podNodeUUID, "eth0", "00:11:22:33:44:55", corenetwork.EthernetDevice)
	spaceUUID := s.addSpace(c)
	cidr := "10.0.0.0/24"
	subnetUUID := s.addSubnet(c, cidr, spaceUUID)

	// Arrange: add pod address (machine local).
	s.addIPAddressWithSubnetAndScope(c, deviceUUID, podNodeUUID, subnetUUID, "10.0.0.1", corenetwork.ScopeMachineLocal)

	// Arrange: add service address (public).
	svcDeviceUUID := s.addLinkLayerDevice(c, svcNodeUUID, "eth1", "00:11:22:33:44:66", corenetwork.EthernetDevice)
	s.addIPAddressWithSubnetAndScope(c, svcDeviceUUID, svcNodeUUID, subnetUUID, "10.0.0.2", corenetwork.ScopeCloudLocal)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, podNodeUUID)
	s.addK8sService(c, svcNodeUUID, appUUID)

	// Arrange: add endpoint
	endpointUUID := s.addApplicationEndpoint(c, appUUID, charmUUID, "endpoint1", spaceUUID)

	// Arrange: add relation and join.
	relationUUID := s.addRelation(c)
	relationEndpointUUID := s.addRelationEndpoint(c, relationUUID.String(), endpointUUID)
	s.addRelationUnit(c, relationEndpointUUID, string(unitUUID))

	// Act
	info, err := s.state.GetUnitRelationNetworkInfosNetworkingNotSupported(c.Context(), unitUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	c.Check(info, tc.SameContents, []internal.RelationNetworkInfo{{
		RelationUUID:   relationUUID,
		IngressAddress: "10.0.0.2",
	}})
}

// TestGetAllSpacesForUnitsRelations tests retrieving space information for endpoints
func (s *infoSuite) TestGetAllSpacesForUnitsRelations(c *tc.C) {
	// Arrange
	nodeUUID := s.addNetNode(c)
	defaultSpace := s.addSpace(c)
	specificSpace := s.addSpace(c)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, defaultSpace)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	// Arrange: add endpoints
	endpoint1Name := "endpoint1"
	endpoint2Name := "endpoint2"
	endpoint1UUID := s.addApplicationEndpoint(c, appUUID, charmUUID, endpoint1Name, "")
	endpoint2UUID := s.addApplicationEndpoint(c, appUUID, charmUUID, endpoint2Name, specificSpace)

	// Arrange: add relations and join.
	relation1UUID := s.addRelation(c)
	relation2UUID := s.addRelation(c)
	relationEndpoint1UUID := s.addRelationEndpoint(c, relation1UUID.String(), endpoint1UUID)
	relationEndpoint2UUID := s.addRelationEndpoint(c, relation2UUID.String(), endpoint2UUID)
	s.addRelationUnit(c, relationEndpoint1UUID, string(unitUUID))
	s.addRelationUnit(c, relationEndpoint2UUID, string(unitUUID))

	// Act
	spaces, err := s.state.getAllSpacesForUnitsRelations(c.Context(), unitUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(spaces, tc.HasLen, 2)

	// Verify both relations are returned with correct space
	c.Assert(spaces, tc.DeepEquals, map[relation.UUID][]string{
		relation1UUID: []string{defaultSpace},
		relation2UUID: []string{specificSpace},
	})
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
	// Clean addresses to only check what we have set in test
	// (we don't test spaceaddress functionnalities)
	addresses = transform.Slice(addresses, func(addr corenetwork.SpaceAddress) corenetwork.SpaceAddress {
		return corenetwork.SpaceAddress{
			MachineAddress: corenetwork.MachineAddress{
				Value: addr.Value,
				CIDR:  addr.CIDR,
			},
			SpaceID: addr.SpaceID,
		}
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addresses, tc.SameContents, []corenetwork.SpaceAddress{
		{
			MachineAddress: corenetwork.MachineAddress{
				Value: "10.0.0.1",
				CIDR:  "10.0.0.0/24",
			},
			SpaceID: corenetwork.SpaceUUID(spaceUUID1),
		}, {
			MachineAddress: corenetwork.MachineAddress{
				Value: "10.1.0.1",
				CIDR:  "10.1.0.0/24",
			},
			SpaceID: corenetwork.SpaceUUID(spaceUUID2),
		},
	})
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
	isCaasIaaS, err := s.state.isCaasUnit(c.Context(), string(iaasUnitUUID))
	c.Assert(err, tc.ErrorIsNil)
	isCaasCaas, err := s.state.isCaasUnit(c.Context(), string(caasUnitUUID))
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

	relationEndpoint1UUID := s.addRelationEndpoint(c, relation1UUID.String(), endpoint1UUID)
	relationEndpoint2UUID := s.addRelationEndpoint(c, relation2UUID.String(), endpoint2UUID)

	s.addRelationUnit(c, relationEndpoint1UUID, string(unitUUID))
	s.addRelationUnit(c, relationEndpoint2UUID, string(unitUUID))

	// Arrange: add egress CIDRs.
	s.addRelationNetworkEgress(c, relation1UUID.String(), "10.0.1.0/24")
	s.addRelationNetworkEgress(c, relation1UUID.String(), "10.0.2.0/24")
	s.addRelationNetworkEgress(c, relation2UUID.String(), "10.0.3.0/24")

	// Act
	cidrs, err := s.state.getUnitEgressSubnets(c.Context(), string(unitUUID))

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cidrs, tc.Equals, "10.0.1.0/24, 10.0.2.0/24, 10.0.3.0/24")
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
	cidrs, err := s.state.getUnitEgressSubnets(c.Context(), string(unitUUID))

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
	relationEndpointUUID := s.addRelationEndpoint(c, relationUUID.String(), endpointUUID)
	s.addRelationUnit(c, relationEndpointUUID, string(unitUUID))

	// Act
	cidrs, err := s.state.getUnitEgressSubnets(c.Context(), string(unitUUID))

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

	relationEndpoint1UUID := s.addRelationEndpoint(c, relation1UUID.String(), endpoint1UUID)
	relationEndpoint2UUID := s.addRelationEndpoint(c, relation2UUID.String(), endpoint2UUID)

	s.addRelationUnit(c, relationEndpoint1UUID, string(unitUUID))
	s.addRelationUnit(c, relationEndpoint2UUID, string(unitUUID))

	// Arrange: add the same CIDR to both relations.
	s.addRelationNetworkEgress(c, relation1UUID.String(), "10.0.1.0/24")
	s.addRelationNetworkEgress(c, relation2UUID.String(), "10.0.1.0/24")

	// Act
	subnets, err := s.state.getUnitEgressSubnets(c.Context(), string(unitUUID))

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(subnets, tc.Equals, "10.0.1.0/24")
}

// TestGetUnitRelationNetworkInfosWithEgressSubnets tests the integration of egress subnets in GetUnitRelationNetworkInfos
func (s *infoSuite) TestGetUnitRelationNetworkInfosWithEgressSubnets(c *tc.C) {
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

	// Arrange: add endpoint
	endpointName := "endpoint1"
	endpointUUID := s.addApplicationEndpoint(c, appUUID, charmUUID, endpointName, "")

	// Arrange: add relation with egress
	relationUUID := s.addRelation(c)
	relationEndpointUUID := s.addRelationEndpoint(c, relationUUID.String(), endpointUUID)
	s.addRelationUnit(c, relationEndpointUUID, string(unitUUID))
	s.addRelationNetworkEgress(c, relationUUID.String(), "192.168.1.0/24")
	s.addRelationNetworkEgress(c, relationUUID.String(), "192.168.2.0/24")

	// Act
	infos, err := s.state.GetUnitRelationNetworkInfos(c.Context(), unitUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infos, tc.HasLen, 1)
	c.Check(infos[0].EgressSubnets, tc.Equals, "192.168.1.0/24, 192.168.2.0/24")
	c.Check(infos[0].IngressAddress, tc.DeepEquals, expectedAddr)
}

// TestGetUnitRelationNetworkInfosIgnoresLoopbackAddresses ensures loopback IPs are filtered out from device and ingress info.
func (s *infoSuite) TestGetUnitRelationNetworkInfosIgnoresLoopbackAddresses(c *tc.C) {
	// Arrange
	nodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(c, nodeUUID, "eth0", "00:11:22:33:44:55", corenetwork.EthernetDevice)
	spaceUUID := s.addSpace(c)

	// Normal subnet and address
	normalCIDR := "10.0.0.0/24"
	normalSubnetUUID := s.addSubnet(c, normalCIDR, spaceUUID)
	normalAddr := "10.0.0.1"
	s.addIPAddressWithSubnetAndScope(c, deviceUUID, nodeUUID, normalSubnetUUID, normalAddr, corenetwork.ScopeCloudLocal)

	// Loopback subnet and address (should be ignored)
	loopCIDR := "127.0.0.0/8"
	loopSubnetUUID := s.addSubnet(c, loopCIDR, spaceUUID)
	loopAddr := "127.0.0.1"
	s.addIPAddressWithSubnetAndScope(c, deviceUUID, nodeUUID, loopSubnetUUID, loopAddr, corenetwork.ScopeMachineLocal)

	// Loopback subnet and address (should be ignored)
	loopIpv6CIDR := "::1/128"
	loopIpv6SubnetUUID := s.addSubnet(c, loopIpv6CIDR, spaceUUID)
	loopIpv6Addr := "::1"
	s.addIPAddressWithSubnetAndScope(c, deviceUUID, nodeUUID, loopIpv6SubnetUUID, loopIpv6Addr, corenetwork.ScopeMachineLocal)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	// Endpoint bound to app default space
	endpointName := "endpoint1"
	endpoint1UUID := s.addApplicationEndpoint(c, appUUID, charmUUID, endpointName, "")

	// Arrange: add relation and join.
	relation1UUID := s.addRelation(c)
	relationEndpoint1UUID := s.addRelationEndpoint(c, relation1UUID.String(), endpoint1UUID)
	s.addRelationUnit(c, relationEndpoint1UUID, string(unitUUID))

	// Act
	infos, err := s.state.GetUnitRelationNetworkInfos(c.Context(), unitUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(infos, tc.SameContents, []internal.RelationNetworkInfo{{
		RelationUUID:   relation1UUID,
		IngressAddress: normalAddr,
		// No egress subnets
	}})
}

// TestGetUnitRelationNetworkInfosOnlyLoopbackIgnored ensures that when only loopback IPs exist, result is empty.
func (s *infoSuite) TestGetUnitRelationNetworkInfosOnlyLoopbackIgnored(c *tc.C) {
	// Arrange
	nodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(c, nodeUUID, "lo", "00:00:00:00:00:00", corenetwork.LoopbackDevice)
	spaceUUID := s.addSpace(c)
	loopCIDR := "127.0.0.0/8"
	loopSubnetUUID := s.addSubnet(c, loopCIDR, spaceUUID)
	loopAddr := "127.0.0.2"
	s.addIPAddressWithSubnetAndScope(c, deviceUUID, nodeUUID, loopSubnetUUID, loopAddr, corenetwork.ScopeMachineLocal)

	// Loopback subnet and address (should be ignored)
	loopIpv6CIDR := "::1/8"
	loopIpv6SubnetUUID := s.addSubnet(c, loopIpv6CIDR, spaceUUID)
	loopIpv6Addr := "::1"
	s.addIPAddressWithSubnetAndScope(c, deviceUUID, nodeUUID, loopIpv6SubnetUUID, loopIpv6Addr, corenetwork.ScopeMachineLocal)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)
	endpointName := "endpoint1"
	endpointUUID := s.addApplicationEndpoint(c, appUUID, charmUUID, endpointName, "")

	relationUUID := s.addRelation(c)
	relationEndpointUUID := s.addRelationEndpoint(c, relationUUID.String(), endpointUUID)
	s.addRelationUnit(c, relationEndpointUUID, string(unitUUID))

	// Act
	infos, err := s.state.GetUnitRelationNetworkInfos(c.Context(), unitUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(infos, tc.SameContents, []internal.RelationNetworkInfo{
		{RelationUUID: relationUUID},
	})
}

// Helper methods

// addApplicationEndpoint creates a charm relation and an application endpoint
// in the database, returning its UUID.
func (s *infoSuite) addApplicationEndpoint(c *tc.C, appUUID, charmUUID, endpointName, spaceUUID string) string {
	// Arrange: add charm relation
	relationUUID := tc.Must(c, relation.NewUUID).String()
	s.query(c, `INSERT INTO charm_relation (uuid, charm_uuid, name, role_id, scope_id) VALUES (?, ?, ?, 0, 0)`,
		relationUUID, charmUUID, endpointName)

	// Arrange: add application endpoint
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
func (s *infoSuite) addRelation(c *tc.C) relation.UUID {
	relationUUID := tc.Must(c, relation.NewUUID)
	s.relationCount++
	s.query(c, `INSERT INTO relation (uuid, life_id, relation_id, scope_id) VALUES (?, 0, ?, 0)`,
		relationUUID, s.relationCount)
	return relationUUID
}

// addRelationEndpoint creates a relation_endpoint linking a relation to an application endpoint.
func (s *infoSuite) addRelationEndpoint(c *tc.C, relationUUID, endpointUUID string) string {
	relationEndpointUUID := tc.Must(c, relation.NewUUID).String()
	s.query(c, `INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid) VALUES (?, ?, ?)`,
		relationEndpointUUID, relationUUID, endpointUUID)
	return relationEndpointUUID
}

// addRelationUnit creates a relation_unit linking a relation endpoint to a unit.
func (s *infoSuite) addRelationUnit(c *tc.C, relationEndpointUUID, unitUUID string) {
	relationUnitUUID := tc.Must(c, relation.NewUUID).String()
	s.query(c, `INSERT INTO relation_unit (uuid, relation_endpoint_uuid, unit_uuid) VALUES (?, ?, ?)`,
		relationUnitUUID, relationEndpointUUID, unitUUID)
}

// addRelationNetworkEgress adds an egress CIDR to a relation.
func (s *infoSuite) addRelationNetworkEgress(c *tc.C, relationUUID, cidr string) {
	s.query(c, `INSERT INTO relation_network_egress (relation_uuid, cidr) VALUES (?, ?)`,
		relationUUID, cidr)
}
