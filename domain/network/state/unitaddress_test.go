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

func (s *unitAddressSuite) TestGetUnitAndK8sServiceAddressesIncludingK8sService(c *tc.C) {
	// Arrange
	podNodeUUID := s.addNetNode(c)
	podDeviceUUID := s.addLinkLayerDevice(c, podNodeUUID)

	svcNodeUUID := s.addNetNode(c)
	svcDeviceUUID := s.addLinkLayerDevice(c, svcNodeUUID)

	spaceUUID := s.addSpace(c)
	subnetUUID, cidr := s.addsubnet(c, spaceUUID)

	podAddr := s.addKubernetesIPAddress(c, podNodeUUID, podDeviceUUID, subnetUUID, 3, 0)
	svcAddr := s.addKubernetesIPAddress(c, svcNodeUUID, svcDeviceUUID, subnetUUID, 1, 1)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, podNodeUUID)
	s.addK8sService(c, svcNodeUUID, appUUID)

	// Act
	addr, err := s.state.GetUnitAndK8sServiceAddresses(c.Context(), unitUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addr, tc.SameContents, []corenetwork.DeviceAddress{
		{
			SpaceAddress: corenetwork.SpaceAddress{
				SpaceID: corenetwork.SpaceUUID(spaceUUID),
				Origin:  corenetwork.OriginMachine,
				MachineAddress: corenetwork.MachineAddress{
					Value:      podAddr,
					CIDR:       cidr,
					Type:       corenetwork.IPv4Address,
					Scope:      corenetwork.ScopeMachineLocal,
					ConfigType: corenetwork.ConfigDHCP,
				},
			},
			DeviceUUID: podDeviceUUID,
		},
		{
			SpaceAddress: corenetwork.SpaceAddress{
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
			DeviceUUID: svcDeviceUUID,
		},
	})
}

func (s *unitAddressSuite) TestGetUnitAndK8sServiceAddressesWithoutK8sService(c *tc.C) {
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
	addr, err := s.state.GetUnitAndK8sServiceAddresses(c.Context(), unitUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addr, tc.DeepEquals, []corenetwork.DeviceAddress{
		{
			SpaceAddress: corenetwork.SpaceAddress{
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
			DeviceUUID: deviceUUID,
		},
	})
}

func (s *unitAddressSuite) TestGetUnitAndK8sServiceAddressesNoAddresses(c *tc.C) {
	// Arrange
	podNodeUUID := s.addNetNode(c)
	svcNodeUUID := s.addNetNode(c)
	spaceUUID := s.addSpace(c)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, podNodeUUID)
	s.addK8sService(c, svcNodeUUID, appUUID)

	// Act
	addr, err := s.state.GetUnitAndK8sServiceAddresses(c.Context(), unitUUID)

	//Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr, tc.DeepEquals, []corenetwork.DeviceAddress{})
}

// TestGetUnitAndK8sServiceAddressesDefaultGateway checks that the device
// facts needed to identify the default route source are returned with each
// address, and that the state does not itself flag the source address.
func (s *unitAddressSuite) TestGetUnitAndK8sServiceAddressesDefaultGateway(c *tc.C) {
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
	s.addUnitDeviceIPAddress(c, nodeUUID, eth0UUID, eth0SubnetUUID,
		"10.1.0.123/24", 0, 2, 0, false)
	s.addUnitDeviceIPAddress(c, nodeUUID, eth0UUID, eth0SubnetUUID,
		"10.1.0.124/24", 0, 2, 0, true)
	s.addUnitDeviceIPAddress(c, nodeUUID, eth1UUID, eth1SubnetUUID,
		"10.0.0.123/24", 0, 2, 0, false)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, nodeUUID)

	// Act
	addrs, err := s.state.GetUnitAndK8sServiceAddresses(c.Context(), unitUUID)

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
				Origin:  corenetwork.OriginMachine,
				MachineAddress: corenetwork.MachineAddress{
					Value:      value,
					CIDR:       cidr,
					Type:       corenetwork.IPv4Address,
					Scope:      corenetwork.ScopeCloudLocal,
					ConfigType: corenetwork.ConfigDHCP,
				},
			},
			DeviceUUID:             deviceUUID,
			IsSecondary:            isSecondary,
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

// TestGetUnitAndK8sServiceAddressesExcludesOtherEntities checks that only the
// addresses of the unit and of the K8s service of its own application are
// returned. The addresses of another unit of the same application, and of
// another application's unit and K8s service, are not.
func (s *unitAddressSuite) TestGetUnitAndK8sServiceAddressesExcludesOtherEntities(c *tc.C) {
	// Arrange
	spaceUUID := s.addSpace(c)
	subnetUUID, cidr := s.addsubnet(c, spaceUUID)
	charmUUID := s.addCharm(c)

	addAddress := func(nodeUUID, value string) string {
		deviceUUID := s.addLinkLayerDevice(c, nodeUUID)
		s.addUnitDeviceIPAddress(c, nodeUUID, deviceUUID, subnetUUID,
			value+"/24", 0, 2, 0, false)
		return deviceUUID
	}

	// The unit and the K8s service of its application.
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitNodeUUID := s.addNetNode(c)
	unitUUID := s.addUnit(c, appUUID, charmUUID, unitNodeUUID)
	unitDeviceUUID := addAddress(unitNodeUUID, "10.0.0.11")
	svcNodeUUID := s.addNetNode(c)
	s.addK8sServiceWithProviderID(c, svcNodeUUID, appUUID, "svc-1")
	svcDeviceUUID := addAddress(svcNodeUUID, "10.0.0.12")

	// Another unit of the same application, on its own net node.
	otherUnitNodeUUID := s.addNetNode(c)
	s.addUnit(c, appUUID, charmUUID, otherUnitNodeUUID)
	addAddress(otherUnitNodeUUID, "10.0.0.21")

	// Another application, with its own unit and K8s service.
	otherAppUUID := s.addApplication(c, charmUUID, spaceUUID)
	otherAppUnitNodeUUID := s.addNetNode(c)
	s.addUnit(c, otherAppUUID, charmUUID, otherAppUnitNodeUUID)
	addAddress(otherAppUnitNodeUUID, "10.0.0.31")
	otherSvcNodeUUID := s.addNetNode(c)
	s.addK8sServiceWithProviderID(c, otherSvcNodeUUID, otherAppUUID, "svc-2")
	addAddress(otherSvcNodeUUID, "10.0.0.32")

	// Act
	addrs, err := s.state.GetUnitAndK8sServiceAddresses(c.Context(), unitUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	expected := func(value, deviceUUID string) corenetwork.DeviceAddress {
		return corenetwork.DeviceAddress{
			SpaceAddress: corenetwork.SpaceAddress{
				SpaceID: corenetwork.SpaceUUID(spaceUUID),
				Origin:  corenetwork.OriginMachine,
				MachineAddress: corenetwork.MachineAddress{
					Value:      value,
					CIDR:       cidr,
					Type:       corenetwork.IPv4Address,
					Scope:      corenetwork.ScopeCloudLocal,
					ConfigType: corenetwork.ConfigDHCP,
				},
			},
			DeviceUUID: deviceUUID,
		}
	}
	c.Check(addrs, tc.SameContents, []corenetwork.DeviceAddress{
		expected("10.0.0.11", unitDeviceUUID),
		expected("10.0.0.12", svcDeviceUUID),
	})
}

func (s *unitAddressSuite) TestGetUnitAndK8sServiceAddressesNotFound(c *tc.C) {
	// Arrange
	svcNodeUUID := s.addNetNode(c)
	spaceUUID := s.addSpace(c)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	s.addK8sService(c, svcNodeUUID, appUUID)

	// Act
	_, err := s.state.GetUnitAndK8sServiceAddresses(c.Context(), "foo")

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
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
		}, {
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

func (s *unitAddressSuite) TestGetControllerAPIAddressesIncludesK8sServicePlaceholder(c *tc.C) {
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
	c.Check(addr, tc.SameContents, domainnetwork.ControllerAPIAddresses{{
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
		DeviceType: domainnetwork.DeviceTypeUnknown,
	}})
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

// addUnitDeviceIPAddress adds an IP address with the input value, which
// should include the prefix length, and returns the address UUID.
// The type, scope and origin arguments are lookup table IDs.
func (s *unitAddressSuite) addUnitDeviceIPAddress(
	c *tc.C, nodeUUID, deviceUUID, subnetUUID, value string,
	typeID, scopeID, originID int, isSecondary bool,
) string {
	ipAddrUUID := uuid.MustNewUUID().String()
	s.query(c, `
INSERT INTO ip_address (
    uuid, net_node_uuid, device_uuid, address_value, type_id, scope_id,
    origin_id, config_type_id, subnet_uuid, is_secondary)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ipAddrUUID, nodeUUID, deviceUUID, value, typeID, scopeID, originID,
		1, subnetUUID, isSecondary)
	return ipAddrUUID
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
