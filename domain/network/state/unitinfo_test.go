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
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/uuid"
)

type infoSuite struct {
	linkLayerBaseSuite
	relationCount int
}

func TestInfoSuite(t *testing.T) {
	tc.Run(t, &infoSuite{})
}

func (s *infoSuite) TestGetUnitEndpointNetworkInfo(c *tc.C) {
	nodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(
		c, nodeUUID, "eth0", "00:11:22:33:44:55", corenetwork.EthernetDevice,
	)
	spaceUUID := corenetwork.AlphaSpaceId.String()
	cidr := "10.0.0.0/24"
	subnetUUID := s.addSubnet(c, cidr, spaceUUID)
	expectedAddr := "10.0.0.1"
	s.addIPAddressWithSubnetAndScope(
		c, deviceUUID, nodeUUID, subnetUUID, expectedAddr, corenetwork.ScopeCloudLocal,
	)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	endpointName := "endpoint1"
	s.addApplicationEndpoint(c, appUUID, charmUUID, endpointName, "")

	info, err := s.state.GetUnitEndpointNetworkInfo(
		c.Context(), string(unitUUID), []string{endpointName},
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(normalizeEndpointNetworkInfo(info), tc.DeepEquals, []networkinternal.EndpointNetworkInfo{{
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
		IngressAddresses: []string{expectedAddr},
	}})
}

func (s *infoSuite) TestGetUnitEndpointNetworkInfoOrdersIngress(c *tc.C) {
	nodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(
		c, nodeUUID, "eth0", "00:11:22:33:44:55", corenetwork.EthernetDevice,
	)
	spaceUUID := corenetwork.AlphaSpaceId.String()
	subnetUUID := s.addSubnet(c, "198.51.100.0/24", spaceUUID)
	s.addIPAddressWithSubnetAndOrigin(
		c, deviceUUID, nodeUUID, subnetUUID, "198.51.100.20", 0,
	)
	s.query(c, `
UPDATE ip_address
SET scope_id = (SELECT id FROM ip_address_scope WHERE name = 'public')
WHERE uuid = ?
`, "address-198.51.100.20-uuid")
	s.addIPAddressWithSubnetAndOrigin(
		c, deviceUUID, nodeUUID, subnetUUID, "198.51.100.10", 1,
	)
	s.query(c, `
UPDATE ip_address
SET scope_id = (SELECT id FROM ip_address_scope WHERE name = 'public')
WHERE uuid = ?
`, "address-198.51.100.10-uuid")

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	endpointName := "endpoint1"
	s.addApplicationEndpoint(c, appUUID, charmUUID, endpointName, "")

	info, err := s.state.GetUnitEndpointNetworkInfo(
		c.Context(), string(unitUUID), []string{endpointName},
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.HasLen, 1)
	c.Check(
		info[0].IngressAddresses,
		tc.DeepEquals,
		[]string{"198.51.100.10", "198.51.100.20"},
	)
}

func (s *infoSuite) TestGetUnitEndpointNetworkInfoPrioritizesPrimaryIngress(c *tc.C) {
	nodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(
		c, nodeUUID, "eth0", "00:11:22:33:44:55", corenetwork.EthernetDevice,
	)
	spaceUUID := corenetwork.AlphaSpaceId.String()
	subnetUUID := s.addSubnet(c, "10.0.0.0/24", spaceUUID)
	s.addIPAddressWithSubnetAndScope(
		c, deviceUUID, nodeUUID, subnetUUID, "10.0.0.20", corenetwork.ScopeCloudLocal,
	)
	secondaryUUID := s.addIPAddressWithSubnetAndScope(
		c, deviceUUID, nodeUUID, subnetUUID, "10.0.0.10", corenetwork.ScopeCloudLocal,
	)
	s.markIPAddressSecondary(c, secondaryUUID)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	endpointName := "endpoint1"
	s.addApplicationEndpoint(c, appUUID, charmUUID, endpointName, "")

	info, err := s.state.GetUnitEndpointNetworkInfo(
		c.Context(), string(unitUUID), []string{endpointName},
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.HasLen, 1)
	c.Check(
		info[0].IngressAddresses,
		tc.DeepEquals,
		[]string{"10.0.0.20", "10.0.0.10"},
	)
}

func (s *infoSuite) TestGetUnitEndpointNetworkInfoMultipleEndpointsSameSpace(c *tc.C) {
	nodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(
		c, nodeUUID, "eth0", "00:11:22:33:44:55", corenetwork.EthernetDevice,
	)
	spaceUUID := corenetwork.AlphaSpaceId.String()
	cidr := "10.0.0.0/24"
	subnetUUID := s.addSubnet(c, cidr, spaceUUID)
	s.addIPAddressWithSubnetAndScope(
		c, deviceUUID, nodeUUID, subnetUUID, "10.0.0.1", corenetwork.ScopeCloudLocal,
	)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	s.addApplicationEndpoint(c, appUUID, charmUUID, "endpoint1", "")
	s.addApplicationEndpoint(c, appUUID, charmUUID, "endpoint2", "")

	info, err := s.state.GetUnitEndpointNetworkInfo(
		c.Context(), string(unitUUID), []string{"endpoint1", "endpoint2"},
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(normalizeEndpointNetworkInfo(info), tc.DeepEquals, []networkinternal.EndpointNetworkInfo{{
		EndpointName: "endpoint1",
		Addresses: []networkinternal.UnitAddress{{
			SpaceAddress: corenetwork.SpaceAddress{
				MachineAddress: corenetwork.MachineAddress{
					Value: "10.0.0.1",
					CIDR:  cidr,
					Scope: corenetwork.ScopeCloudLocal,
				},
				SpaceID: corenetwork.SpaceUUID(spaceUUID),
			},
			DeviceName: "eth0",
			MACAddress: "00:11:22:33:44:55",
			DeviceType: corenetwork.EthernetDevice,
		}},
		IngressAddresses: []string{"10.0.0.1"},
	}, {
		EndpointName: "endpoint2",
		Addresses: []networkinternal.UnitAddress{{
			SpaceAddress: corenetwork.SpaceAddress{
				MachineAddress: corenetwork.MachineAddress{
					Value: "10.0.0.1",
					CIDR:  cidr,
					Scope: corenetwork.ScopeCloudLocal,
				},
				SpaceID: corenetwork.SpaceUUID(spaceUUID),
			},
			DeviceName: "eth0",
			MACAddress: "00:11:22:33:44:55",
			DeviceType: corenetwork.EthernetDevice,
		}},
		IngressAddresses: []string{"10.0.0.1"},
	}})
}

func (s *infoSuite) TestGetUnitEndpointNetworkInfoCaasUnit(c *tc.C) {
	podNodeUUID := s.addNetNode(c)
	svcNodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(
		c, podNodeUUID, "eth0", "00:11:22:33:44:55", corenetwork.EthernetDevice,
	)
	spaceUUID := s.addSpace(c)
	cidr := "10.0.0.0/24"
	subnetUUID := s.addSubnet(c, cidr, spaceUUID)
	s.addIPAddressWithSubnetAndScope(
		c, deviceUUID, podNodeUUID, subnetUUID, "10.0.0.1", corenetwork.ScopeMachineLocal,
	)

	svcDeviceUUID := s.addLinkLayerDevice(
		c, svcNodeUUID, "eth1", "00:11:22:33:44:66", corenetwork.EthernetDevice,
	)
	s.addIPAddressWithSubnetAndScope(
		c, svcDeviceUUID, svcNodeUUID, subnetUUID, "10.0.0.2", corenetwork.ScopeCloudLocal,
	)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, podNodeUUID)
	s.addK8sService(c, svcNodeUUID, appUUID)

	endpointName := "endpoint1"
	s.addApplicationEndpoint(c, appUUID, charmUUID, endpointName, spaceUUID)

	info, err := s.state.GetUnitEndpointNetworkInfo(
		c.Context(), string(unitUUID), []string{endpointName},
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.HasLen, 1)
	c.Check(info[0].EndpointName, tc.Equals, endpointName)
	c.Check(info[0].IngressAddresses, tc.DeepEquals, []string{"10.0.0.2"})
	c.Check(normalizeUnitAddresses(info[0].Addresses), tc.SameContents, []networkinternal.UnitAddress{{
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

func (s *infoSuite) TestGetUnitEndpointNetworkInfoNoAddresses(c *tc.C) {
	nodeUUID := s.addNetNode(c)
	spaceUUID := s.addSpace(c)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	endpointName := "endpoint1"
	s.addApplicationEndpoint(c, appUUID, charmUUID, endpointName, spaceUUID)

	info, err := s.state.GetUnitEndpointNetworkInfo(
		c.Context(), string(unitUUID), []string{endpointName},
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(info, tc.DeepEquals, []networkinternal.EndpointNetworkInfo{{
		EndpointName: endpointName,
	}})
}

func (s *infoSuite) TestGetUnitNetworkInfo(c *tc.C) {
	nodeUUID := s.addNetNode(c)
	ethDeviceUUID := s.addLinkLayerDevice(
		c, nodeUUID, "eth0", "00:11:22:33:44:55", corenetwork.EthernetDevice,
	)
	vethDeviceUUID := s.addLinkLayerDevice(
		c, nodeUUID, "veth0", "00:11:22:33:44:66", corenetwork.VirtualEthernetDevice,
	)
	spaceUUID := corenetwork.AlphaSpaceId.String()
	subnetUUID := s.addSubnet(c, "198.51.100.0/24", spaceUUID)

	s.addIPAddressWithSubnetAndOrigin(
		c, ethDeviceUUID, nodeUUID, subnetUUID, "198.51.100.20", 0,
	)
	s.query(c, `
UPDATE ip_address
SET scope_id = (SELECT id FROM ip_address_scope WHERE name = 'public')
WHERE uuid = ?
`, "address-198.51.100.20-uuid")

	s.addIPAddressWithSubnetAndOrigin(
		c, ethDeviceUUID, nodeUUID, subnetUUID, "198.51.100.10", 1,
	)
	s.query(c, `
UPDATE ip_address
SET scope_id = (SELECT id FROM ip_address_scope WHERE name = 'public')
WHERE uuid = ?
`, "address-198.51.100.10-uuid")

	s.addIPAddressWithSubnetAndOrigin(
		c, vethDeviceUUID, nodeUUID, subnetUUID, "198.51.100.30", 1,
	)
	s.query(c, `
UPDATE ip_address
SET scope_id = (SELECT id FROM ip_address_scope WHERE name = 'public')
WHERE uuid = ?
`, "address-198.51.100.30-uuid")

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	info, err := s.state.GetUnitNetworkInfo(c.Context(), string(unitUUID))

	c.Assert(err, tc.ErrorIsNil)
	c.Check(
		info.IngressAddresses,
		tc.DeepEquals,
		[]string{"198.51.100.10", "198.51.100.20", "198.51.100.30"},
	)
	c.Check(normalizeUnitAddresses(info.Addresses), tc.SameContents, []networkinternal.UnitAddress{{
		SpaceAddress: corenetwork.SpaceAddress{
			MachineAddress: corenetwork.MachineAddress{
				Value: "198.51.100.20",
				CIDR:  "198.51.100.0/24",
				Scope: corenetwork.ScopePublic,
			},
			SpaceID: corenetwork.SpaceUUID(spaceUUID),
		},
		DeviceName: "eth0",
		MACAddress: "00:11:22:33:44:55",
		DeviceType: corenetwork.EthernetDevice,
	}, {
		SpaceAddress: corenetwork.SpaceAddress{
			MachineAddress: corenetwork.MachineAddress{
				Value: "198.51.100.10",
				CIDR:  "198.51.100.0/24",
				Scope: corenetwork.ScopePublic,
			},
			SpaceID: corenetwork.SpaceUUID(spaceUUID),
		},
		DeviceName: "eth0",
		MACAddress: "00:11:22:33:44:55",
		DeviceType: corenetwork.EthernetDevice,
	}, {
		SpaceAddress: corenetwork.SpaceAddress{
			MachineAddress: corenetwork.MachineAddress{
				Value: "198.51.100.30",
				CIDR:  "198.51.100.0/24",
				Scope: corenetwork.ScopePublic,
			},
			SpaceID: corenetwork.SpaceUUID(spaceUUID),
		},
		DeviceName: "veth0",
		MACAddress: "00:11:22:33:44:66",
		DeviceType: corenetwork.VirtualEthernetDevice,
	}})
}

func (s *infoSuite) TestGetUnitNetworkInfoPrioritizesPrimaryIngress(c *tc.C) {
	nodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(
		c, nodeUUID, "eth0", "00:11:22:33:44:55", corenetwork.EthernetDevice,
	)
	spaceUUID := corenetwork.AlphaSpaceId.String()
	subnetUUID := s.addSubnet(c, "10.0.0.0/24", spaceUUID)
	s.addIPAddressWithSubnetAndScope(
		c, deviceUUID, nodeUUID, subnetUUID, "10.0.0.20", corenetwork.ScopeCloudLocal,
	)
	secondaryUUID := s.addIPAddressWithSubnetAndScope(
		c, deviceUUID, nodeUUID, subnetUUID, "10.0.0.10", corenetwork.ScopeCloudLocal,
	)
	s.markIPAddressSecondary(c, secondaryUUID)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	info, err := s.state.GetUnitNetworkInfo(c.Context(), string(unitUUID))

	c.Assert(err, tc.ErrorIsNil)
	c.Check(
		info.IngressAddresses,
		tc.DeepEquals,
		[]string{"10.0.0.20", "10.0.0.10"},
	)
}

func (s *infoSuite) TestGetUnitNetworkInfoCaasUnit(c *tc.C) {
	podNodeUUID := s.addNetNode(c)
	svcNodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(
		c, podNodeUUID, "eth0", "00:11:22:33:44:55", corenetwork.EthernetDevice,
	)
	spaceUUID := corenetwork.AlphaSpaceId.String()
	cidr := "10.0.0.0/24"
	subnetUUID := s.addSubnet(c, cidr, spaceUUID)
	s.addIPAddressWithSubnetAndScope(
		c, deviceUUID, podNodeUUID, subnetUUID, "10.0.0.1", corenetwork.ScopeMachineLocal,
	)

	svcVethUUID := s.addLinkLayerDevice(
		c, svcNodeUUID, "veth0", "00:11:22:33:44:66", corenetwork.VirtualEthernetDevice,
	)
	s.addIPAddressWithSubnetAndScope(
		c, svcVethUUID, svcNodeUUID, subnetUUID, "10.0.0.3", corenetwork.ScopeCloudLocal,
	)

	svcEthUUID := s.addLinkLayerDevice(
		c, svcNodeUUID, "eth1", "00:11:22:33:44:77", corenetwork.EthernetDevice,
	)
	s.addIPAddressWithSubnetAndScope(
		c, svcEthUUID, svcNodeUUID, subnetUUID, "10.0.0.2", corenetwork.ScopeCloudLocal,
	)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, podNodeUUID)
	s.addK8sService(c, svcNodeUUID, appUUID)

	info, err := s.state.GetUnitNetworkInfo(c.Context(), string(unitUUID))

	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.IngressAddresses, tc.DeepEquals, []string{"10.0.0.2", "10.0.0.3"})
	c.Check(normalizeUnitAddresses(info.Addresses), tc.SameContents, []networkinternal.UnitAddress{{
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
		MACAddress: "00:11:22:33:44:77",
		DeviceType: corenetwork.EthernetDevice,
	}, {
		SpaceAddress: corenetwork.SpaceAddress{
			MachineAddress: corenetwork.MachineAddress{
				Value: "10.0.0.3",
				CIDR:  cidr,
				Scope: corenetwork.ScopeCloudLocal,
			},
			SpaceID: corenetwork.SpaceUUID(spaceUUID),
		},
		DeviceName: "veth0",
		MACAddress: "00:11:22:33:44:66",
		DeviceType: corenetwork.VirtualEthernetDevice,
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
	s.addRelationEndpoint(c, relationUUID, endpointUUID)

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

func (s *infoSuite) TestGetModelEgressSubnets(c *tc.C) {
	s.query(c, `INSERT INTO model_config VALUES (?, ?)`,
		config.EgressSubnets, "10.0.1.0/24, 10.0.2.0/24")

	cidrs, err := s.state.GetModelEgressSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cidrs, tc.DeepEquals, []string{"10.0.1.0/24", "10.0.2.0/24"})
}

func (s *infoSuite) TestGetModelEgressSubnetsEmpty(c *tc.C) {
	cidrs, err := s.state.GetModelEgressSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cidrs, tc.HasLen, 0)
}

// Helper methods

func normalizeEndpointNetworkInfo(
	addresses []networkinternal.EndpointNetworkInfo,
) []networkinternal.EndpointNetworkInfo {
	return transform.Slice(addresses, func(addr networkinternal.EndpointNetworkInfo) networkinternal.EndpointNetworkInfo {
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

func (s *infoSuite) markIPAddressSecondary(c *tc.C, addressUUID string) {
	s.query(c, `
UPDATE ip_address
SET is_secondary = true
WHERE uuid = ?
`, addressUUID)
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
