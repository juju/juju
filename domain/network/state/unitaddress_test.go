// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	corenetwork "github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
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

	podAddr := s.addIPAddress(c, podNodeUUID, podDeviceUUID, subnetUUID, corenetwork.ScopeMachineLocal, corenetwork.OriginMachine)
	svcAddr := s.addIPAddress(c, svcNodeUUID, svcDeviceUUID, subnetUUID, corenetwork.ScopePublic, corenetwork.OriginProvider)

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
	expectedAddr := s.addIPAddress(c, nodeUUID, deviceUUID, subnetUUID, corenetwork.ScopeMachineLocal, corenetwork.OriginProvider)

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
	expectedAddr := s.addIPAddress(c, nodeUUID, deviceUUID, subnetUUID, corenetwork.ScopeMachineLocal, corenetwork.OriginProvider)

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
