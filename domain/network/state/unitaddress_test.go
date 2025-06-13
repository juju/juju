// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"
	"time"

	"github.com/juju/tc"

	corenetwork "github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
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
	s.addk8sService(c, svcNodeUUID, appUUID)

	// Act
	addr, err := s.state.GetUnitAndK8sServiceAddresses(c.Context(), unitUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addr, tc.DeepEquals, corenetwork.SpaceAddresses{
		{
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

func (s *unitAddressSuite) TestGetUnitAndK8sServiceAddressesNoAddresses(c *tc.C) {
	// Arrange
	podNodeUUID := s.addNetNode(c)
	svcNodeUUID := s.addNetNode(c)
	spaceUUID := s.addSpace(c)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	unitUUID := s.addUnit(c, appUUID, charmUUID, podNodeUUID)
	s.addk8sService(c, svcNodeUUID, appUUID)

	// Act
	addr, err := s.state.GetUnitAndK8sServiceAddresses(c.Context(), unitUUID)

	//Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr, tc.DeepEquals, corenetwork.SpaceAddresses{})
}

func (s *unitAddressSuite) TestGetUnitAndK8sServiceAddressesNotFound(c *tc.C) {
	// Arrange
	svcNodeUUID := s.addNetNode(c)
	spaceUUID := s.addSpace(c)

	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, spaceUUID)
	s.addk8sService(c, svcNodeUUID, appUUID)

	// Act
	_, err := s.state.GetUnitAndK8sServiceAddresses(c.Context(), "foo")

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
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

func (s *unitAddressSuite) addApplication(c *tc.C, charmUUID, spaceUUID string) string {
	appUUID := uuid.MustNewUUID().String()
	s.query(c, `INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, ?, ?, ?)`,
		appUUID, appUUID, life.Alive, charmUUID, spaceUUID)
	return appUUID
}

func (s *unitAddressSuite) addUnit(c *tc.C, appUUID, charmUUID, nodeUUID string) coreunit.UUID {
	unitUUID := unittesting.GenUnitUUID(c)
	s.query(c, `INSERT INTO unit (uuid, name, life_id, application_uuid, charm_uuid, net_node_uuid) VALUES (?, ?, ?, ?, ?, ?)`,
		unitUUID, unitUUID, life.Alive, appUUID, charmUUID, nodeUUID)
	return unitUUID
}

func (s *unitAddressSuite) addk8sService(c *tc.C, nodeUUID, appUUID string) {
	svcUUID := uuid.MustNewUUID().String()
	s.query(c, `INSERT INTO k8s_service (uuid, net_node_uuid, application_uuid, provider_id) VALUES (?, ?, ?, ?)`,
		svcUUID, nodeUUID, appUUID, "provider-id")
}
