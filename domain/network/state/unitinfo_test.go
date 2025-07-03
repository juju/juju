// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"
	"time"

	"github.com/juju/collections/transform"
	"github.com/juju/tc"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/uuid"
)

type infoSuite struct {
	linkLayerBaseSuite
}

func TestInfoSuite(t *testing.T) {
	tc.Run(t, &infoSuite{})
}

// TestGetUnitEndpointNetworks tests the main function that retrieves network information for unit endpoints
func (s *infoSuite) TestGetUnitEndpointNetworks(c *tc.C) {
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
	networks, err := s.state.GetUnitEndpointNetworks(c.Context(), string(unitUUID), []string{endpointName})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(networks, tc.DeepEquals, []network.UnitNetwork{{
		EndpointName: endpointName,
		DeviceInfos: []network.DeviceInfo{{
			Name:       "eth0",
			MACAddress: "00:11:22:33:44:55",
			Addresses: []network.AddressInfo{{
				Hostname: expectedAddr,
				Value:    expectedAddr,
				CIDR:     cidr,
			}},
		}},
		IngressAddresses: []string{expectedAddr},
	}})
}

// TestGetUnitEndpointNetworksMultipleEndpoints tests retrieving network information for multiple endpoints
func (s *infoSuite) TestGetUnitEndpointNetworksMultipleEndpoints(c *tc.C) {
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
	networks, err := s.state.GetUnitEndpointNetworks(c.Context(), string(unitUUID), []string{"endpoint1", "endpoint2"})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(networks, tc.SameContents, []network.UnitNetwork{{
		EndpointName: "endpoint1",
		DeviceInfos: []network.DeviceInfo{{
			Name:       "eth0",
			MACAddress: "00:11:22:33:44:55",
			Addresses: []network.AddressInfo{{
				Hostname: "10.0.0.1",
				Value:    "10.0.0.1",
				CIDR:     "10.0.0.0/24",
			}},
		}},
		IngressAddresses: []string{"10.0.0.1"},
	}, {
		EndpointName: "endpoint2",
		DeviceInfos: []network.DeviceInfo{{
			Name:       "eth0",
			MACAddress: "00:11:22:33:44:55",
			Addresses: []network.AddressInfo{{
				Hostname: "10.0.1.1",
				Value:    "10.0.1.1",
				CIDR:     "10.0.1.0/24",
			}},
		}},
		IngressAddresses: []string{"10.0.1.1"},
	}})
}

// TestGetUnitEndpointNetworksCaasUnit tests retrieving network information for a CAAS unit
func (s *infoSuite) TestGetUnitEndpointNetworksCaasUnit(c *tc.C) {
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
	networks, err := s.state.GetUnitEndpointNetworks(c.Context(), string(unitUUID), []string{endpointName})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(networks, tc.HasLen, 1)
	c.Assert(networks[0].EndpointName, tc.Equals, endpointName)

	// For CAAS units, machine local addresses should be included in device info
	devices := transform.SliceToMap(networks[0].DeviceInfos, func(d network.DeviceInfo) (string, network.DeviceInfo) {
		return d.Name, d
	})
	c.Assert(devices, tc.HasLen, 2)
	c.Assert(devices["eth0"].Addresses, tc.SameContents, []network.AddressInfo{{
		Hostname: "10.0.0.1",
		Value:    "10.0.0.1",
		CIDR:     "10.0.0.0/24",
	}})
	c.Assert(devices["eth0"].MACAddress, tc.Equals, "00:11:22:33:44:55")
	c.Assert(devices["eth1"].Addresses, tc.HasLen, 0)
	c.Assert(devices["eth0"].MACAddress, tc.Equals, "00:11:22:33:44:55")

	// For CAAS units, only non-machine-local addresses should be in ingress addresses
	c.Assert(networks[0].IngressAddresses, tc.DeepEquals, []string{"10.0.0.2"})
}

// TestGetUnitEndpointNetworksNoAddresses tests retrieving network information when no addresses are available
func (s *infoSuite) TestGetUnitEndpointNetworksNoAddresses(c *tc.C) {
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
	networks, err := s.state.GetUnitEndpointNetworks(c.Context(), string(unitUUID), []string{endpointName})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(networks, tc.SameContents, []network.UnitNetwork{{
		EndpointName: endpointName,
		// No devices nor ingresses
	}})
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
	// Clean addresses to only check what we have setted in test
	//(we don't test spaceaddress functionnalities)
	addresses = transform.Slice(addresses, func(addr unitAddress) unitAddress {
		return unitAddress{
			SpaceAddress: corenetwork.SpaceAddress{
				MachineAddress: corenetwork.MachineAddress{
					Value: addr.SpaceAddress.MachineAddress.Value,
					CIDR:  addr.SpaceAddress.MachineAddress.CIDR,
				},
				SpaceID: addr.SpaceAddress.SpaceID,
			},
			Device: addr.Device,
			MAC:    addr.MAC,
		}
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addresses, tc.SameContents, []unitAddress{{
		SpaceAddress: corenetwork.SpaceAddress{
			MachineAddress: corenetwork.MachineAddress{
				Value: "10.0.0.1",
				CIDR:  "10.0.0.0/24",
			},
			SpaceID: corenetwork.SpaceUUID(spaceUUID1),
		},
		Device: "eth0",
		MAC:    "00:11:22:33:44:55",
	}, {
		SpaceAddress: corenetwork.SpaceAddress{
			MachineAddress: corenetwork.MachineAddress{
				Value: "10.1.0.1",
				CIDR:  "10.1.0.0/24",
			},
			SpaceID: corenetwork.SpaceUUID(spaceUUID2),
		},
		Device: "eth0",
		MAC:    "00:11:22:33:44:55",
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
	isCaasIaaS, err := s.state.isCaasUnit(c.Context(), string(iaasUnitUUID))
	c.Assert(err, tc.ErrorIsNil)
	isCaasCaas, err := s.state.isCaasUnit(c.Context(), string(caasUnitUUID))
	c.Assert(err, tc.ErrorIsNil)

	// Assert
	c.Assert(isCaasIaaS, tc.Equals, false)
	c.Assert(isCaasCaas, tc.Equals, true)
}

// Helper methods

// addCharm inserts a new charm record into the database and returns its UUID as a string.
func (s *infoSuite) addCharm(c *tc.C) string {
	charmUUID := uuid.MustNewUUID().String()
	s.query(c, `INSERT INTO charm (uuid, reference_name, create_time) VALUES (?, ?, ?)`,
		charmUUID, charmUUID, time.Now())
	return charmUUID
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
