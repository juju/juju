// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"
	"time"

	"github.com/juju/tc"

	corenetwork "github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/uuid"
)

type unitAddressSuite struct {
	linkLayerBaseSuite
}

func TestUnitAddressSuite(t *testing.T) {
	tc.Run(t, &unitAddressSuite{})
}

func (s *unitAddressSuite) TestGetUnitPublicAddressesIncludingK8sService(c *tc.C) {
	// Arrange
	podNodeUUID := s.addNetNode(c)
	podDeviceUUID := s.addLinkLayerDevice(c, podNodeUUID)

	svcNodeUUID := s.addNetNode(c)
	svcDeviceUUID := s.addLinkLayerDevice(c, svcNodeUUID)

	spaceUUID := s.addSpace(c)
	subnetUUID, cidr := s.addsubnet(c, spaceUUID)

	s.addKubernetesIPAddress(c, podNodeUUID, podDeviceUUID, subnetUUID, 2, 0)
	svcAddr := s.addKubernetesIPAddress(c, svcNodeUUID, svcDeviceUUID, subnetUUID, 1, 1)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, podNodeUUID)
	s.addK8sService(c, svcNodeUUID, appUUID)

	// Act
	addr, err := s.state.GetUnitPublicAddresses(c.Context(), unitUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addr, tc.SameContents, corenetwork.SpaceAddresses{
		{
			SpaceID: corenetwork.SpaceUUID(spaceUUID),
			Origin:  corenetwork.OriginProvider,
			MachineAddress: corenetwork.MachineAddress{
				Value:      svcAddr,
				CIDR:       cidr,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopePublic,
				ConfigType: corenetwork.ConfigDHCP,
			},
		},
	})
}

func (s *unitAddressSuite) TestGetUnitPublicAddressesWithoutK8sService(c *tc.C) {
	// Arrange
	nodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(c, nodeUUID)
	spaceUUID := s.addSpace(c)
	subnetUUID, cidr := s.addsubnet(c, spaceUUID)
	expectedAddr := s.addIPAddress(c, nodeUUID, deviceUUID, subnetUUID, 2, 1)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	// Act
	addr, err := s.state.GetUnitPublicAddresses(c.Context(), unitUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addr, tc.DeepEquals, corenetwork.SpaceAddresses{
		{
			SpaceID: corenetwork.SpaceUUID(spaceUUID),
			Origin:  corenetwork.OriginProvider,
			MachineAddress: corenetwork.MachineAddress{
				Value:      expectedAddr,
				CIDR:       cidr,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopeCloudLocal,
				ConfigType: corenetwork.ConfigDHCP,
			},
		},
	})
}

// Select a single scope/family group across both net nodes, retaining all
// matching addresses rather than selecting one group independently per node.
func (s *unitAddressSuite) TestGetUnitPublicAddressesCombinesPodAndService(c *tc.C) {
	podNode := s.addNetNode(c)
	podDevice := s.addLinkLayerDevice(c, podNode)
	svcNode := s.addNetNode(c)
	svcDevice := s.addLinkLayerDevice(c, svcNode)
	spaceUUID := s.addSpace(c)
	subnetUUID, _ := s.addsubnet(c, spaceUUID)
	s.addKubernetesIPAddress(c, podNode, podDevice, subnetUUID, 1, 0)
	s.addKubernetesIPAddress(c, svcNode, svcDevice, subnetUUID, 1, 1)
	s.query(c, `UPDATE ip_address SET address_value = '1.1.1.1' WHERE net_node_uuid = ?`, podNode)
	s.query(c, `UPDATE ip_address SET address_value = '8.8.8.8' WHERE net_node_uuid = ?`, svcNode)
	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, podNode)
	s.addK8sService(c, svcNode, appUUID)

	addresses, err := s.state.GetUnitPublicAddresses(c.Context(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addresses.Values(), tc.DeepEquals, []string{"8.8.8.8", "1.1.1.1"})

	// A public IPv6 service address must not be included alongside the
	// preferred public IPv4 family supplied by the pod.
	s.query(c, `
UPDATE ip_address SET address_value = '2001:db8::1', type_id = 1
WHERE net_node_uuid = ?`, svcNode)
	addresses, err = s.state.GetUnitPublicAddresses(c.Context(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addresses.Values(), tc.DeepEquals, []string{"1.1.1.1"})
}

func (s *unitAddressSuite) TestGetUnitPublicAddressesNoAddresses(c *tc.C) {
	// Arrange
	podNodeUUID := s.addNetNode(c)
	svcNodeUUID := s.addNetNode(c)
	spaceUUID := s.addSpace(c)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, podNodeUUID)
	s.addK8sService(c, svcNodeUUID, appUUID)

	// Act
	addr, err := s.state.GetUnitPublicAddresses(c.Context(), unitUUID.String())

	//Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr, tc.DeepEquals, corenetwork.SpaceAddresses{})
}

// TestGetUnitPublicAddressesDefaultRouteOrdering checks that SQL
// returns primary addresses on the default route device before primary
// addresses on other devices, and secondary addresses after all primaries.
func (s *unitAddressSuite) TestGetUnitPublicAddressesDefaultRouteOrdering(c *tc.C) {
	// Arrange
	nodeUUID := s.addNetNode(c)
	eth0UUID := s.linkLayerBaseSuite.addLinkLayerDevice(
		c, nodeUUID, "eth0", "00:11:22:33:44:55", corenetwork.EthernetDevice)
	eth1UUID := s.linkLayerBaseSuite.addLinkLayerDevice(
		c, nodeUUID, "eth1", "00:11:22:33:44:66", corenetwork.EthernetDevice)
	s.setDefaultGateway(c, eth0UUID, "10.1.0.1")

	spaceUUID := s.addSpace(c)
	eth0SubnetUUID := s.addSubnet(c, "10.1.0.0/24", spaceUUID)
	eth1SubnetUUID := s.addSubnet(c, "10.0.0.0/24", spaceUUID)
	s.addIPAddressWithSubnetAndScope(c, eth0UUID, nodeUUID, eth0SubnetUUID,
		"10.1.0.123", corenetwork.ScopeCloudLocal)
	secondaryUUID := s.addIPAddressWithSubnetAndScope(c, eth0UUID, nodeUUID,
		eth0SubnetUUID, "10.1.0.124", corenetwork.ScopeCloudLocal)
	s.query(c, `UPDATE ip_address SET is_secondary = true WHERE uuid = ?`, secondaryUUID)
	s.addIPAddressWithSubnetAndScope(c, eth1UUID, nodeUUID, eth1SubnetUUID,
		"10.0.0.123", corenetwork.ScopeCloudLocal)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	// Act
	addrs, err := s.state.GetUnitPublicAddresses(c.Context(), unitUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addrs.Values(), tc.DeepEquals, []string{
		"10.1.0.123", "10.0.0.123", "10.1.0.124",
	})
}

func (s *unitAddressSuite) TestGetUnitPublicAddressesNotFound(c *tc.C) {
	// Arrange
	svcNodeUUID := s.addNetNode(c)
	spaceUUID := s.addSpace(c)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	s.addK8sService(c, svcNodeUUID, appUUID)

	// Act
	_, err := s.state.GetUnitPublicAddresses(c.Context(), "foo")

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

// Both queries must preserve the scope/family fallback rules that previously
// ran in the service, including returning the whole best group for a unit.
func (s *unitAddressSuite) TestPublicAddressSelection(c *tc.C) {
	nodeUUID := s.addNetNode(c)
	defaultDevice := s.linkLayerBaseSuite.addLinkLayerDevice(
		c, nodeUUID, "eth0", "00:11:22:33:44:55", corenetwork.EthernetDevice)
	otherDevice := s.linkLayerBaseSuite.addLinkLayerDevice(
		c, nodeUUID, "eth1", "00:11:22:33:44:66", corenetwork.EthernetDevice)
	s.setDefaultGateway(c, defaultDevice, "10.1.0.1")
	spaceUUID := s.addSpace(c)
	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	// An unrelated node's public address must not affect either selection.
	unrelatedNode := s.addNetNode(c)
	unrelatedDevice := s.addLinkLayerDevice(c, unrelatedNode)
	s.query(c, `
INSERT INTO ip_address (
    uuid, net_node_uuid, device_uuid, address_value, type_id, scope_id,
    origin_id, config_type_id
) VALUES (?, ?, ?, '9.9.9.9', 0, 1, 1, 0)`,
		uuid.MustNewUUID().String(), unrelatedNode, unrelatedDevice)

	type candidate struct {
		value        string
		scope        corenetwork.Scope
		provider     bool
		secondary    bool
		defaultRoute bool
	}
	for _, test := range []struct {
		name       string
		candidates []candidate
		expected   []string
	}{
		{
			name: "no addresses",
		}, {
			name: "only unusable addresses",
			candidates: []candidate{
				{value: "127.0.0.1", scope: corenetwork.ScopeMachineLocal},
				{value: "169.254.1.1", scope: corenetwork.ScopeLinkLocal},
			},
		}, {
			name: "public scope and IPv4 family",
			candidates: []candidate{
				{value: "8.8.8.8", scope: corenetwork.ScopePublic},
				{value: "1.1.1.1", scope: corenetwork.ScopePublic},
				{value: "2001:db8::1", scope: corenetwork.ScopePublic},
				{value: "10.0.0.1", scope: corenetwork.ScopeCloudLocal, defaultRoute: true},
				{value: "127.0.0.1", scope: corenetwork.ScopeMachineLocal},
			},
			expected: []string{"1.1.1.1", "8.8.8.8"},
		}, {
			name: "public IPv6 precedes cloud-local IPv4",
			candidates: []candidate{
				{value: "2001:db8::2", scope: corenetwork.ScopePublic, defaultRoute: true},
				{value: "2001:db8::1", scope: corenetwork.ScopePublic},
				{value: "10.0.0.1", scope: corenetwork.ScopeCloudLocal, defaultRoute: true},
			},
			expected: []string{"2001:db8::1", "2001:db8::2"},
		}, {
			name: "cloud-local IPv4 fallback",
			candidates: []candidate{
				{value: "10.0.0.2", scope: corenetwork.ScopeCloudLocal},
				{value: "10.0.0.1", scope: corenetwork.ScopeCloudLocal},
				{value: "fd00::1", scope: corenetwork.ScopeCloudLocal},
				{value: "1.1.1.1", scope: corenetwork.ScopeUnknown},
				{value: "169.254.1.1", scope: corenetwork.ScopeLinkLocal},
			},
			expected: []string{"10.0.0.1", "10.0.0.2"},
		}, {
			name: "cloud-local IPv6 precedes unknown IPv4",
			candidates: []candidate{
				{value: "fd00::2", scope: corenetwork.ScopeCloudLocal, defaultRoute: true},
				{value: "fd00::1", scope: corenetwork.ScopeCloudLocal},
				{value: "1.1.1.1", scope: corenetwork.ScopeUnknown},
			},
			expected: []string{"fd00::1", "fd00::2"},
		}, {
			name: "unknown IPv4 scope is excluded",
			candidates: []candidate{
				{value: "8.8.8.8", scope: corenetwork.ScopeUnknown},
				{value: "1.1.1.1", scope: corenetwork.ScopeUnknown},
				{value: "2001:db8::1", scope: corenetwork.ScopeUnknown},
				{value: "127.0.0.1", scope: corenetwork.ScopeMachineLocal},
			},
		}, {
			name: "unknown IPv6 scope is excluded",
			candidates: []candidate{
				{value: "2001:db8::2", scope: corenetwork.ScopeUnknown},
				{value: "2001:db8::1", scope: corenetwork.ScopeUnknown},
			},
		}, {
			name: "default route precedes origin and value",
			candidates: []candidate{
				{value: "8.8.8.8", scope: corenetwork.ScopePublic, defaultRoute: true},
				{value: "1.1.1.1", scope: corenetwork.ScopePublic, provider: true},
			},
			expected: []string{"8.8.8.8", "1.1.1.1"},
		}, {
			name: "primary precedes secondary on default device",
			candidates: []candidate{
				{value: "1.1.1.1", scope: corenetwork.ScopePublic, secondary: true, defaultRoute: true},
				{value: "8.8.8.8", scope: corenetwork.ScopePublic},
			},
			expected: []string{"8.8.8.8", "1.1.1.1"},
		}, {
			name: "secondary addresses ignore default device",
			candidates: []candidate{
				{value: "8.8.8.8", scope: corenetwork.ScopePublic, secondary: true, defaultRoute: true},
				{value: "1.1.1.1", scope: corenetwork.ScopePublic, secondary: true},
			},
			expected: []string{"1.1.1.1", "8.8.8.8"},
		}, {
			name: "scope precedes primary status",
			candidates: []candidate{
				{value: "8.8.8.8", scope: corenetwork.ScopePublic, secondary: true},
				{value: "10.0.0.1", scope: corenetwork.ScopeCloudLocal, defaultRoute: true},
			},
			expected: []string{"8.8.8.8"},
		}, {
			name: "provider origin precedes value",
			candidates: []candidate{
				{value: "8.8.8.8", scope: corenetwork.ScopePublic, provider: true},
				{value: "1.1.1.1", scope: corenetwork.ScopePublic},
			},
			expected: []string{"8.8.8.8", "1.1.1.1"},
		},
	} {
		c.Logf("%s", test.name)
		s.query(c, `DELETE FROM ip_address WHERE net_node_uuid = ?`, nodeUUID)
		var allValues []string
		for _, addr := range test.candidates {
			deviceUUID := otherDevice
			if addr.defaultRoute {
				deviceUUID = defaultDevice
			}
			typeID := 0
			if corenetwork.NewMachineAddress(addr.value).Type == corenetwork.IPv6Address {
				typeID = 1
			}
			scope := addr.scope.String()
			if addr.scope == corenetwork.ScopeUnknown {
				// The database uses "unknown", which the existing address
				// decoder does not map to the core empty-string scope.
				scope = "unknown"
			}
			s.query(c, `
INSERT INTO ip_address (
    uuid, net_node_uuid, device_uuid, address_value, type_id, scope_id,
    origin_id, config_type_id, is_secondary
)
SELECT ?, ?, ?, ?, ?, scope.id, ?, 0, ?
FROM ip_address_scope AS scope
WHERE scope.name = ?`, uuid.MustNewUUID().String(), nodeUUID, deviceUUID,
				addr.value, typeID, addr.provider, addr.secondary, scope)
			allValues = append(allValues, addr.value)
		}

		addresses, err := s.state.GetUnitPublicAddresses(c.Context(), unitUUID.String())
		c.Assert(err, tc.ErrorIsNil)
		c.Check(addresses.Values(), tc.DeepEquals, test.expected)
		address, err := s.state.GetNetNodePublicAddress(c.Context(), nodeUUID)
		c.Assert(err, tc.ErrorIsNil)
		if len(test.expected) == 0 {
			c.Check(address, tc.DeepEquals, corenetwork.SpaceAddress{})
		} else {
			c.Check(address.Value, tc.Equals, test.expected[0])
		}

		// SSH's all-address query must retain every scope and family.
		all, err := s.state.GetNetNodeAddresses(c.Context(), nodeUUID, true)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(all.Values(), tc.SameContents, allValues)
	}
}

func (s *unitAddressSuite) TestGetControllerAPIAddressesReturnsDeviceTypes(c *tc.C) {
	// Arrange
	nodeUUID := s.addNetNode(c)
	ethDeviceUUID := s.linkLayerBaseSuite.addLinkLayerDevice(
		c, nodeUUID, "eth0", "00:11:22:33:44:55",
		corenetwork.EthernetDevice,
	)
	vethDeviceUUID := s.linkLayerBaseSuite.addLinkLayerDevice(
		c, nodeUUID, "veth0", "00:11:22:33:44:66",
		corenetwork.VirtualEthernetDevice,
	)
	localDeviceUUID := s.linkLayerBaseSuite.addLinkLayerDevice(
		c, nodeUUID, "eth1", "00:11:22:33:44:77",
		corenetwork.EthernetDevice,
	)

	spaceUUID := s.addSpace(c)
	subnetUUID, cidr := s.addsubnet(c, spaceUUID)
	s.addIPAddressWithSubnetAndScope(
		c, ethDeviceUUID, nodeUUID, subnetUUID, "10.0.0.1",
		corenetwork.ScopeCloudLocal,
	)
	s.addIPAddressWithSubnetAndScope(
		c, vethDeviceUUID, nodeUUID, subnetUUID, "10.0.0.2",
		corenetwork.ScopeCloudLocal,
	)
	s.addIPAddressWithSubnetAndScope(
		c, localDeviceUUID, nodeUUID, subnetUUID, "10.0.0.3",
		corenetwork.ScopeMachineLocal,
	)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	// Act
	addr, err := s.state.GetControllerAPIAddresses(c.Context(), unitUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addr, tc.SameContents, domainnetwork.ControllerAPIAddresses{
		{
			SpaceAddress: corenetwork.SpaceAddress{
				SpaceID: corenetwork.SpaceUUID(spaceUUID),
				Origin:  corenetwork.OriginProvider,
				MachineAddress: corenetwork.MachineAddress{
					Value:      "10.0.0.1",
					CIDR:       cidr,
					Type:       corenetwork.IPv4Address,
					Scope:      corenetwork.ScopeCloudLocal,
					ConfigType: corenetwork.ConfigStatic,
				},
			},
			DeviceType: domainnetwork.DeviceTypeEthernet,
		},
		{
			SpaceAddress: corenetwork.SpaceAddress{
				SpaceID: corenetwork.SpaceUUID(spaceUUID),
				Origin:  corenetwork.OriginProvider,
				MachineAddress: corenetwork.MachineAddress{
					Value:      "10.0.0.2",
					CIDR:       cidr,
					Type:       corenetwork.IPv4Address,
					Scope:      corenetwork.ScopeCloudLocal,
					ConfigType: corenetwork.ConfigStatic,
				},
			},
			DeviceType: domainnetwork.DeviceTypeVeth,
		},
	})
}

func (s *unitAddressSuite) TestGetControllerAPIAddressesUsesK8sPodAddress(c *tc.C) {
	// Arrange
	podNodeUUID := s.addNetNode(c)
	podDeviceUUID := s.linkLayerBaseSuite.addLinkLayerDevice(
		c, podNodeUUID, "eth0", "00:11:22:33:44:55",
		corenetwork.EthernetDevice,
	)

	svcNodeUUID := s.addNetNode(c)
	svcDeviceUUID := s.linkLayerBaseSuite.addLinkLayerDevice(
		c, svcNodeUUID, "placeholder", "00:11:22:33:44:66",
		corenetwork.EthernetDevice,
	)
	// Kubernetes service devices are placeholders without a discovered type.
	s.query(c, `UPDATE link_layer_device SET device_type_id = 0 WHERE uuid = ?`,
		svcDeviceUUID)

	spaceUUID := s.addSpace(c)
	subnetUUID, cidr := s.addsubnet(c, spaceUUID)
	s.addIPAddressWithSubnetAndScope(
		c, podDeviceUUID, podNodeUUID, subnetUUID, "10.0.0.1",
		corenetwork.ScopeMachineLocal,
	)
	s.addIPAddressWithSubnetAndScope(
		c, svcDeviceUUID, svcNodeUUID, subnetUUID, "10.0.0.2",
		corenetwork.ScopeCloudLocal,
	)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, podNodeUUID)
	s.addK8sService(c, svcNodeUUID, appUUID)

	// Act
	addr, err := s.state.GetControllerAPIAddresses(c.Context(), unitUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addr, tc.DeepEquals, domainnetwork.ControllerAPIAddresses{{
		SpaceAddress: corenetwork.SpaceAddress{
			SpaceID: corenetwork.SpaceUUID(spaceUUID),
			Origin:  corenetwork.OriginProvider,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "10.0.0.1",
				CIDR:       cidr,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopeCloudLocal,
				ConfigType: corenetwork.ConfigStatic,
			},
		},
		DeviceType: domainnetwork.DeviceTypeEthernet,
	}})
}

func (s *unitAddressSuite) TestGetControllerAPIAddressesExcludesLoopback(c *tc.C) {
	nodeUUID := s.addNetNode(c)
	ethDeviceUUID := s.linkLayerBaseSuite.addLinkLayerDevice(
		c, nodeUUID, "eth0", "00:11:22:33:44:55",
		corenetwork.EthernetDevice,
	)
	loDeviceUUID := s.linkLayerBaseSuite.addLinkLayerDevice(
		c, nodeUUID, "lo", "00:00:00:00:00:00",
		corenetwork.LoopbackDevice,
	)

	spaceUUID := s.addSpace(c)
	subnetUUID, cidr := s.addsubnet(c, spaceUUID)
	s.addIPAddressWithSubnetAndScope(
		c, ethDeviceUUID, nodeUUID, subnetUUID, "10.0.0.1",
		corenetwork.ScopeCloudLocal,
	)
	s.addIPAddressWithSubnetAndScope(
		c, loDeviceUUID, nodeUUID, subnetUUID, "127.0.0.1",
		corenetwork.ScopeCloudLocal,
	)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	addr, err := s.state.GetControllerAPIAddresses(c.Context(), unitUUID.String())

	c.Assert(err, tc.ErrorIsNil)
	c.Check(addr, tc.DeepEquals, domainnetwork.ControllerAPIAddresses{{
		SpaceAddress: corenetwork.SpaceAddress{
			SpaceID: corenetwork.SpaceUUID(spaceUUID),
			Origin:  corenetwork.OriginProvider,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "10.0.0.1",
				CIDR:       cidr,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopeCloudLocal,
				ConfigType: corenetwork.ConfigStatic,
			},
		},
		DeviceType: domainnetwork.DeviceTypeEthernet,
	}})
}

func (s *unitAddressSuite) TestGetControllerAPIAddressesExcludesAllMachineLocal(c *tc.C) {
	nodeUUID := s.addNetNode(c)
	deviceUUID := s.linkLayerBaseSuite.addLinkLayerDevice(
		c, nodeUUID, "eth0", "00:11:22:33:44:55",
		corenetwork.EthernetDevice,
	)

	spaceUUID := s.addSpace(c)
	subnetUUID, _ := s.addsubnet(c, spaceUUID)
	s.addIPAddressWithSubnetAndScope(
		c, deviceUUID, nodeUUID, subnetUUID, "10.0.0.1",
		corenetwork.ScopeMachineLocal,
	)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	addr, err := s.state.GetControllerAPIAddresses(c.Context(), unitUUID.String())

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr, tc.DeepEquals, domainnetwork.ControllerAPIAddresses{})
}

func (s *unitAddressSuite) TestGetControllerAPIAddressesCaasMachineLocalPodIncluded(c *tc.C) {
	podNodeUUID := s.addNetNode(c)
	podDeviceUUID := s.linkLayerBaseSuite.addLinkLayerDevice(
		c, podNodeUUID, "eth0", "00:11:22:33:44:55",
		corenetwork.EthernetDevice,
	)

	spaceUUID := s.addSpace(c)
	subnetUUID, cidr := s.addsubnet(c, spaceUUID)
	s.addIPAddressWithSubnetAndScope(
		c, podDeviceUUID, podNodeUUID, subnetUUID, "10.0.0.1",
		corenetwork.ScopeMachineLocal,
	)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, podNodeUUID)
	s.addK8sService(c, s.addNetNode(c), appUUID)

	addr, err := s.state.GetControllerAPIAddresses(c.Context(), unitUUID.String())

	c.Assert(err, tc.ErrorIsNil)
	c.Check(addr, tc.DeepEquals, domainnetwork.ControllerAPIAddresses{{
		SpaceAddress: corenetwork.SpaceAddress{
			SpaceID: corenetwork.SpaceUUID(spaceUUID),
			Origin:  corenetwork.OriginProvider,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "10.0.0.1",
				CIDR:       cidr,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.Scope("local-cloud"),
				ConfigType: corenetwork.ConfigStatic,
			},
		},
		DeviceType: domainnetwork.DeviceTypeEthernet,
	}})
}

func (s *unitAddressSuite) TestGetControllerAPIAddressesCaasServiceAddressOmitted(c *tc.C) {
	svcNodeUUID := s.addNetNode(c)
	svcDeviceUUID := s.linkLayerBaseSuite.addLinkLayerDevice(
		c, svcNodeUUID, "placeholder", "00:11:22:33:44:66",
		corenetwork.EthernetDevice,
	)

	podNodeUUID := s.addNetNode(c)

	spaceUUID := s.addSpace(c)
	subnetUUID, _ := s.addsubnet(c, spaceUUID)
	s.addIPAddressWithSubnetAndScope(
		c, svcDeviceUUID, svcNodeUUID, subnetUUID, "10.0.0.1",
		corenetwork.ScopeCloudLocal,
	)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, podNodeUUID)
	s.addK8sService(c, svcNodeUUID, appUUID)

	addr, err := s.state.GetControllerAPIAddresses(c.Context(), unitUUID.String())

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr, tc.DeepEquals, domainnetwork.ControllerAPIAddresses{})
}

func (s *unitAddressSuite) TestGetUnitAddresses(c *tc.C) {
	// Arrange
	nodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(c, nodeUUID)
	spaceUUID := s.addSpace(c)
	subnetUUID, cidr := s.addsubnet(c, spaceUUID)
	expectedAddr := s.addIPAddress(c, nodeUUID, deviceUUID, subnetUUID, 3, 1)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	// Act
	addr, err := s.state.GetUnitAddresses(c.Context(), unitUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addr, tc.DeepEquals, corenetwork.SpaceAddresses{
		{
			SpaceID: corenetwork.SpaceUUID(spaceUUID),
			Origin:  corenetwork.OriginProvider,
			MachineAddress: corenetwork.MachineAddress{
				Value:      expectedAddr,
				CIDR:       cidr,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopeMachineLocal,
				ConfigType: corenetwork.ConfigDHCP,
			},
		},
	})
}

func (s *unitAddressSuite) TestGetUnitAddressesNoAddresses(c *tc.C) {
	// Arrange
	nodeUUID := s.addNetNode(c)
	spaceUUID := s.addSpace(c)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	// Act
	addr, err := s.state.GetUnitAddresses(c.Context(), unitUUID)

	// Arrange
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr, tc.DeepEquals, corenetwork.SpaceAddresses{})
}

func (s *unitAddressSuite) TestGetControllerUnitUUIDByName(c *tc.C) {
	// Arrange
	nodeUUID := s.addNetNode(c)
	spaceUUID := s.addSpace(c)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	s.addControllerApplication(c, appUUID)
	// The unit uuid and name are the same in addUnit.
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	// Act:
	uuid, err := s.state.GetControllerUnitUUIDByName(c.Context(), unitUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid, tc.Equals, unitUUID.String())
}

func (s *unitAddressSuite) TestGetControllerUnitUUIDByNameNotFound(c *tc.C) {
	_, err := s.state.GetControllerUnitUUIDByName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitAddressSuite) TestGetControllerUnitUUIDByNameNotController(c *tc.C) {
	// Arrange: add a unit but do NOT add application to
	// application_controller table.
	nodeUUID := s.addNetNode(c)
	spaceUUID := s.addSpace(c)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	s.addUnit(c, appUUID, charmUUID, nodeUUID)

	// Act
	_, err := s.state.GetControllerUnitUUIDByName(c.Context(), "foo")

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitAddressSuite) TestGetUnitUUIDByName(c *tc.C) {
	// Arrange
	nodeUUID := s.addNetNode(c)
	spaceUUID := s.addSpace(c)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	// Act
	uuid, err := s.state.GetUnitUUIDByName(c.Context(), coreunit.Name(unitUUID))

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid, tc.Equals, unitUUID)
}

func (s *unitAddressSuite) TestGetUnitUUIDByNameNotFound(c *tc.C) {
	_, err := s.state.GetUnitUUIDByName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitAddressSuite) TestGetUnitAddressesNotFound(c *tc.C) {
	_, err := s.state.GetUnitAddresses(c.Context(), coreunit.UUID("foo"))
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitAddressSuite) addIPAddress(c *tc.C, nodeUUID, deviceUUID, subnetUUID string, scopeID, originID int) string {
	ipAddrUUID := uuid.MustNewUUID().String()
	addr := "10.0.0.1"
	s.query(c, `INSERT INTO ip_address (uuid, net_node_uuid, device_uuid, address_value, type_id, scope_id, origin_id, config_type_id, subnet_uuid) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ipAddrUUID, nodeUUID, deviceUUID, addr+"/24", 0, scopeID, originID, 1, subnetUUID)
	return addr
}

func (s *unitAddressSuite) addKubernetesIPAddress(c *tc.C, nodeUUID, deviceUUID, subnetUUID string, scopeID, originID int) string {
	ipAddrUUID := uuid.MustNewUUID().String()
	addr := "10.0.0.1"
	s.query(c, `INSERT INTO ip_address (uuid, net_node_uuid, device_uuid, address_value, type_id, scope_id, origin_id, config_type_id, subnet_uuid) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ipAddrUUID, nodeUUID, deviceUUID, addr, 0, scopeID, originID, 1, subnetUUID)
	return addr
}

func (s *unitAddressSuite) addCharm(c *tc.C) string {
	charmUUID := uuid.MustNewUUID().String()
	s.query(c, `INSERT INTO charm (uuid, reference_name, create_time) VALUES (?, ?, ?)`,
		charmUUID, charmUUID, time.Now())
	return charmUUID
}

func (s *unitAddressSuite) addControllerApplication(c *tc.C, applicationUUID string) {
	s.query(c, `INSERT INTO application_controller (application_uuid) VALUES ( ?)`,
		applicationUUID)
}

func (s *unitAddressSuite) addsubnet(c *tc.C, spaceUUID string) (string, string) {
	subnetUUID := uuid.MustNewUUID().String()
	cidr := "10.0.0.0/24"
	s.query(c, `INSERT INTO subnet (uuid, cidr, space_uuid) VALUES (?, ?, ?)`,
		subnetUUID, cidr, spaceUUID)
	return subnetUUID, cidr
}

func (s *unitAddressSuite) addLinkLayerDevice(c *tc.C, netNodeUUID string) string {
	name := uuid.MustNewUUID().String()
	return s.linkLayerBaseSuite.addLinkLayerDevice(c, netNodeUUID, name, "00:11:22:33:44:55",
		corenetwork.EthernetDevice)
}
