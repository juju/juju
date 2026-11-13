// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

func TestUnitAddressSuite(t *testing.T) {
	tc.Run(t, &unitAddressSuite{})
}

type unitAddressSuite struct {
	testhelpers.IsolationSuite

	st *MockState
}

func (s *unitAddressSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.st = NewMockState(ctrl)
	c.Cleanup(func() { s.st = nil })
	return ctrl
}

func (s *unitAddressSuite) service(c *tc.C) *Service {
	return NewService(s.st, loggertesting.WrapCheckLog(c))
}

func (s *unitAddressSuite) TestGetPublicAddressUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID(""), errors.New("boom"))

	_, err := s.service(c).GetUnitPublicAddress(c.Context(), unitName)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *unitAddressSuite) TestGetPublicAddressWithCloudServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil)
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).Return(nil, errors.New("boom"))

	_, err := s.service(c).GetUnitPublicAddress(c.Context(), unitName)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *unitAddressSuite) TestGetPublicAddressNonMatchingAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	nonMatchingScopeAddrs := network.SpaceAddresses{
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.1",
				ConfigType: network.ConfigStatic,
				Type:       network.IPv4Address,
				Scope:      network.ScopeMachineLocal,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.2",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopeMachineLocal,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "10.0.1.1",
				ConfigType: network.ConfigStatic,
				Type:       network.IPv4Address,
				Scope:      network.ScopeMachineLocal,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "10.0.1.2",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopeMachineLocal,
			},
		},
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unit.Name("foo/0")).Return(unit.UUID("foo-uuid"), nil)
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo-uuid")).Return(toDeviceAddresses(nonMatchingScopeAddrs), nil)

	_, err := s.service(c).GetUnitPublicAddress(c.Context(), unitName)
	c.Assert(err, tc.ErrorMatches, "no public address.*")
}

func (s *unitAddressSuite) TestGetPublicAddressMatchingAddress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	matchingScopeAddrs := network.SpaceAddresses{
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.1",
				ConfigType: network.ConfigStatic,
				Type:       network.IPv4Address,
				Scope:      network.ScopeMachineLocal,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "54.32.1.2",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopePublic,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "54.32.1.3",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopeCloudLocal,
			},
		},
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil)
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).Return(toDeviceAddresses(matchingScopeAddrs), nil)

	addr, err := s.service(c).GetUnitPublicAddress(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	// Since the second address is higher in hierarchy of scope match, it should
	// be returned.
	c.Check(addr, tc.DeepEquals, matchingScopeAddrs[1])
}

func (s *unitAddressSuite) TestGetPublicAddressMatchingAddressSameOrigin(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	matchingScopeAddrs := network.SpaceAddresses{
		{
			SpaceID: network.AlphaSpaceId,
			Origin:  network.OriginProvider,
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.1",
				ConfigType: network.ConfigStatic,
				Type:       network.IPv4Address,
				Scope:      network.ScopeMachineLocal,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			Origin:  network.OriginProvider,
			MachineAddress: network.MachineAddress{
				Value:      "54.32.1.2",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopePublic,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			Origin:  network.OriginProvider,
			MachineAddress: network.MachineAddress{
				Value:      "54.32.1.3",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopePublic,
			},
		},
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil)
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).Return(toDeviceAddresses(matchingScopeAddrs), nil)

	addr, err := s.service(c).GetUnitPublicAddress(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	// Since the second address is higher in hierarchy of scope match, it should
	// be returned.
	c.Check(addr, tc.DeepEquals, matchingScopeAddrs[1])
}

func (s *unitAddressSuite) TestGetPublicAddressMatchingAddressOneProviderOnly(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	matchingScopeAddrs := network.SpaceAddresses{
		{
			SpaceID: network.AlphaSpaceId,
			Origin:  network.OriginMachine,
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.1",
				ConfigType: network.ConfigStatic,
				Type:       network.IPv4Address,
				Scope:      network.ScopeMachineLocal,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			Origin:  network.OriginMachine,
			MachineAddress: network.MachineAddress{
				Value:      "54.32.1.2",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopePublic,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			Origin:  network.OriginProvider,
			MachineAddress: network.MachineAddress{
				Value:      "54.32.1.3",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopePublic,
			},
		},
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil)
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).Return(toDeviceAddresses(matchingScopeAddrs), nil)

	addr, err := s.service(c).GetUnitPublicAddress(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	// Since the second address is higher in hierarchy of scope match, it should
	// be returned.
	c.Check(addr, tc.DeepEquals, matchingScopeAddrs[2])
}

func (s *unitAddressSuite) TestGetPublicAddressMatchingAddressOneProviderOtherUnknown(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	matchingScopeAddrs := network.SpaceAddresses{
		{
			SpaceID: network.AlphaSpaceId,
			Origin:  network.OriginMachine,
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.1",
				ConfigType: network.ConfigStatic,
				Type:       network.IPv4Address,
				Scope:      network.ScopeMachineLocal,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			Origin:  network.OriginUnknown,
			MachineAddress: network.MachineAddress{
				Value:      "54.32.1.2",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopePublic,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			Origin:  network.OriginProvider,
			MachineAddress: network.MachineAddress{
				Value:      "54.32.1.3",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopePublic,
			},
		},
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil)
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).Return(toDeviceAddresses(matchingScopeAddrs), nil)

	addr, err := s.service(c).GetUnitPublicAddress(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	// Since the second address is higher in hierarchy of scope match, it should
	// be returned.
	c.Check(addr, tc.DeepEquals, matchingScopeAddrs[2])
}

func (s *unitAddressSuite) TestGetPublicAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	unitAddresses := network.SpaceAddresses{
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.1/24",
				ConfigType: network.ConfigStatic,
				Type:       network.IPv4Address,
				Scope:      network.ScopePublic,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.2/24",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopePublic,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "54.32.1.2/24",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopeMachineLocal,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "54.32.1.3/24",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopeCloudLocal,
			},
		},
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil)
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).Return(toDeviceAddresses(unitAddresses), nil)

	addrs, err := s.service(c).GetUnitPublicAddresses(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	// The two public addresses should be returned.
	c.Check(addrs, tc.DeepEquals, unitAddresses[0:2])
}

func (s *unitAddressSuite) TestGetPublicAddressesCloudLocal(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	unitAddresses := network.SpaceAddresses{
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.1/24",
				ConfigType: network.ConfigStatic,
				Type:       network.IPv4Address,
				Scope:      network.ScopeCloudLocal,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.2/24",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopeCloudLocal,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "54.32.1.2/24",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopeMachineLocal,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "54.32.1.3/24",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopeMachineLocal,
			},
		},
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil)
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).Return(toDeviceAddresses(unitAddresses), nil)

	addrs, err := s.service(c).GetUnitPublicAddresses(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	// The two cloud-local addresses should be returned because there are no
	// public ones.
	c.Check(addrs, tc.DeepEquals, unitAddresses[0:2])
}

// TestGetPublicAddressesCloudLocalPrefersDefaultRouteSource checks that
// GetUnitPublicAddresses, with no public addresses, returns the cloud-local
// address on the device carrying the default route before a cloud-local
// address with a lower value on another device.
func (s *unitAddressSuite) TestGetPublicAddressesCloudLocalPrefersDefaultRouteSource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	unitName := unit.Name("foo/0")
	eth1Addr, eth0Addr := cloudLocalDefaultRouteAddresses()

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil)
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).
		Return([]network.DeviceAddress{eth1Addr, eth0Addr}, nil)

	// Act
	addrs, err := s.service(c).GetUnitPublicAddresses(c.Context(), unitName)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	expectedDefault := eth0Addr.SpaceAddress
	expectedDefault.IsDefaultRouteSource = true
	c.Check(addrs, tc.DeepEquals, network.SpaceAddresses{
		expectedDefault, eth1Addr.SpaceAddress,
	})
}

// TestGetPublicAddressCloudLocalPrefersDefaultRouteSource checks that
// GetUnitPublicAddress, with no public addresses, returns the cloud-local
// address on the device carrying the default route rather than a cloud-local
// address with a lower value on another device.
func (s *unitAddressSuite) TestGetPublicAddressCloudLocalPrefersDefaultRouteSource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	unitName := unit.Name("foo/0")
	eth1Addr, eth0Addr := cloudLocalDefaultRouteAddresses()

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil)
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).
		Return([]network.DeviceAddress{eth1Addr, eth0Addr}, nil)

	// Act
	addr, err := s.service(c).GetUnitPublicAddress(c.Context(), unitName)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	expectedDefault := eth0Addr.SpaceAddress
	expectedDefault.IsDefaultRouteSource = true
	c.Check(addr, tc.DeepEquals, expectedDefault)
}

// TestGetPublicAddressesPublicBeatsDefaultRouteSource checks that
// GetUnitPublicAddresses returns only the public address when the default
// route source is cloud-local. That is, marking the default route source does
// not change the scope tier filter.
func (s *unitAddressSuite) TestGetPublicAddressesPublicBeatsDefaultRouteSource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	unitName := unit.Name("foo/0")
	publicAddr, cloudLocalAddr := publicAndCloudLocalDefaultRouteAddresses()

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil)
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).
		Return([]network.DeviceAddress{cloudLocalAddr, publicAddr}, nil)

	// Act
	addrs, err := s.service(c).GetUnitPublicAddresses(c.Context(), unitName)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addrs, tc.DeepEquals, network.SpaceAddresses{publicAddr.SpaceAddress})
}

// TestGetPublicAddressPublicBeatsDefaultRouteSource checks that
// GetUnitPublicAddress returns the public address, not the cloud-local
// default route source. That is, marking the default route source does not
// change the scope tier filter.
func (s *unitAddressSuite) TestGetPublicAddressPublicBeatsDefaultRouteSource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	unitName := unit.Name("foo/0")
	publicAddr, cloudLocalAddr := publicAndCloudLocalDefaultRouteAddresses()

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil)
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).
		Return([]network.DeviceAddress{cloudLocalAddr, publicAddr}, nil)

	// Act
	addr, err := s.service(c).GetUnitPublicAddress(c.Context(), unitName)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addr, tc.DeepEquals, publicAddr.SpaceAddress)
}

// TestGetPublicAddressMarksBeforeScopeFilter checks that the default route
// source is marked before the machine-local addresses are filtered out. The
// only address in the gateway's subnet is machine-local, so it is marked and
// then excluded, and the cloud-local address on the same device is not marked.
// The lower-valued address on the other device is returned. If the marking
// happened after the scope filter, nothing would match the gateway, the
// fallback would mark the cloud-local address on the default route device, and
// that would be returned instead.
func (s *unitAddressSuite) TestGetPublicAddressMarksBeforeScopeFilter(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	unitName := unit.Name("foo/0")
	eth0Addr, eth1MachineLocal, eth1CloudLocal := machineLocalDefaultRouteAddresses()

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil)
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).
		Return([]network.DeviceAddress{
			eth0Addr, eth1MachineLocal, eth1CloudLocal,
		}, nil)

	// Act
	addr, err := s.service(c).GetUnitPublicAddress(c.Context(), unitName)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addr, tc.DeepEquals, eth0Addr.SpaceAddress)
}

func (s *unitAddressSuite) TestGetPublicAddressesNoAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil)
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).Return([]network.DeviceAddress{}, nil)

	_, err := s.service(c).GetUnitPublicAddresses(c.Context(), unitName)
	c.Assert(err, tc.Satisfies, network.IsNoAddressError)
}

func (s *unitAddressSuite) TestGetControllerAPIAddressesPrefersNonVeth(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	unitName := unit.Name("foo/0")
	candidates := domainnetwork.ControllerAPIAddresses{
		newControllerAPIAddress("10.0.0.1", "space-0", domainnetwork.DeviceTypeVeth),
		newControllerAPIAddress("10.0.0.2", "space-0", domainnetwork.DeviceTypeEthernet),
	}

	s.st.EXPECT().GetControllerUnitUUIDByName(gomock.Any(), unitName.String()).Return("foo", nil)
	s.st.EXPECT().GetControllerAPIAddresses(gomock.Any(), "foo").Return(candidates, nil)

	// Act
	addrs, err := s.service(c).GetControllerAPIAddresses(c.Context(), unitName, nil)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addrs, tc.DeepEquals, network.SpaceAddresses{candidates[1].SpaceAddress})
}

func (s *unitAddressSuite) TestGetControllerAPIAddressesFallsBackToVeth(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")
	candidates := domainnetwork.ControllerAPIAddresses{
		newControllerAPIAddress("10.0.0.1", "space-0", domainnetwork.DeviceTypeVeth),
		newControllerAPIAddress("10.0.0.2", "space-1", domainnetwork.DeviceTypeVeth),
	}

	s.st.EXPECT().GetControllerUnitUUIDByName(gomock.Any(), unitName.String()).Return("foo", nil)
	s.st.EXPECT().GetControllerAPIAddresses(gomock.Any(), "foo").Return(candidates, nil)

	addrs, err := s.service(c).GetControllerAPIAddresses(c.Context(), unitName, nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(addrs, tc.DeepEquals, network.SpaceAddresses{
		candidates[0].SpaceAddress,
		candidates[1].SpaceAddress,
	})
}

func (s *unitAddressSuite) TestGetControllerAPIAddressesHonoursManagementSpace(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")
	candidates := domainnetwork.ControllerAPIAddresses{
		newControllerAPIAddress("10.0.0.1", "management", domainnetwork.DeviceTypeVeth),
		newControllerAPIAddress("10.0.0.2", "management", domainnetwork.DeviceTypeEthernet),
		newControllerAPIAddress("10.1.0.1", "other", domainnetwork.DeviceTypeEthernet),
	}
	managementSpace := &network.SpaceInfo{ID: "management"}

	s.st.EXPECT().GetControllerUnitUUIDByName(gomock.Any(), unitName.String()).Return("foo", nil)
	s.st.EXPECT().GetControllerAPIAddresses(gomock.Any(), "foo").Return(candidates, nil)

	addrs, err := s.service(c).GetControllerAPIAddresses(
		c.Context(), unitName, managementSpace,
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(addrs, tc.DeepEquals, network.SpaceAddresses{
		candidates[1].SpaceAddress,
		candidates[2].SpaceAddress,
	})
}

func (s *unitAddressSuite) TestGetControllerAPIAddressesFallsBackToVethInManagementSpace(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")
	candidates := domainnetwork.ControllerAPIAddresses{
		newControllerAPIAddress("10.0.0.1", "management", domainnetwork.DeviceTypeVeth),
		newControllerAPIAddress("10.1.0.1", "other", domainnetwork.DeviceTypeEthernet),
	}
	managementSpace := &network.SpaceInfo{ID: "management"}

	s.st.EXPECT().GetControllerUnitUUIDByName(gomock.Any(), unitName.String()).Return("foo", nil)
	s.st.EXPECT().GetControllerAPIAddresses(gomock.Any(), "foo").Return(candidates, nil)

	addrs, err := s.service(c).GetControllerAPIAddresses(
		c.Context(), unitName, managementSpace,
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(addrs, tc.DeepEquals, network.SpaceAddresses{
		candidates[0].SpaceAddress,
		candidates[1].SpaceAddress,
	})
}

func (s *unitAddressSuite) TestGetControllerAPIAddressesNoAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	unitName := unit.Name("foo/0")
	s.st.EXPECT().GetControllerUnitUUIDByName(gomock.Any(), unitName.String()).Return("foo", nil)
	s.st.EXPECT().GetControllerAPIAddresses(gomock.Any(), "foo").Return(domainnetwork.ControllerAPIAddresses{}, nil)

	// Act
	_, err := s.service(c).GetControllerAPIAddresses(c.Context(), unitName, nil)

	// Assert
	c.Assert(err, tc.Satisfies, network.IsNoAddressError)
}

func (s *unitAddressSuite) TestGetControllerAPIAddressesUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	unitName := unit.Name("foo/0")
	s.st.EXPECT().GetControllerUnitUUIDByName(gomock.Any(), unitName.String()).Return("foo", applicationerrors.UnitNotFound)

	// Act
	_, err := s.service(c).GetControllerAPIAddresses(c.Context(), unitName, nil)

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitAddressSuite) TestGetPrivateAddressUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), errors.New("boom"))

	_, err := s.service(c).GetUnitPrivateAddress(c.Context(), unitName)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *unitAddressSuite) TestGetPrivateAddressError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil)
	s.st.EXPECT().GetUnitAddresses(gomock.Any(), unit.UUID("foo")).Return(nil, errors.New("boom"))

	_, err := s.service(c).GetUnitPrivateAddress(c.Context(), unitName)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *unitAddressSuite) TestGetPrivateAddressNonMatchingAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	// Only machine-local and link-local addresses.
	nonMatchingScopeAddrs := network.SpaceAddresses{
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.1",
				ConfigType: network.ConfigStatic,
				Type:       network.IPv4Address,
				Scope:      network.ScopeMachineLocal,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.2",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopeMachineLocal,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "10.0.1.1",
				ConfigType: network.ConfigStatic,
				Type:       network.IPv4Address,
				Scope:      network.ScopeMachineLocal,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "10.0.1.2",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopeMachineLocal,
			},
		},
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unit.Name("foo/0")).Return(unit.UUID("foo-uuid"), nil)
	s.st.EXPECT().GetUnitAddresses(gomock.Any(), unit.UUID("foo-uuid")).Return(nonMatchingScopeAddrs, nil)

	_, err := s.service(c).GetUnitPrivateAddress(c.Context(), unitName)
	// AllMatchingScope with ScopeMatchCloudLocal returns invalidScope for both
	// scopes, so no address can be selected and NoAddressError must be returned.
	c.Assert(err, tc.Satisfies, network.IsNoAddressError)
}

func (s *unitAddressSuite) TestGetPrivateAddressNonMatchingAddressesSorted(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	nonMatchingScopeAddrs := network.SpaceAddresses{
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.9",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopeCloudLocal,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.2",
				ConfigType: network.ConfigStatic,
				Type:       network.IPv4Address,
				Scope:      network.ScopeCloudLocal,
			},
		},
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo-uuid"), nil)
	s.st.EXPECT().GetUnitAddresses(gomock.Any(), unit.UUID("foo-uuid")).Return(nonMatchingScopeAddrs, nil)

	addr, err := s.service(c).GetUnitPrivateAddress(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr, tc.DeepEquals, nonMatchingScopeAddrs[1])
}

func (s *unitAddressSuite) TestGetPrivateAddressMatchingAddress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	matchingScopeAddrs := network.SpaceAddresses{
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "54.32.1.2",
				ConfigType: network.ConfigStatic,
				Type:       network.IPv4Address,
				Scope:      network.ScopePublic,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "192.168.1.2",
				ConfigType: network.ConfigStatic,
				Type:       network.IPv4Address,
				Scope:      network.ScopeCloudLocal,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.2",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopeMachineLocal,
			},
		},
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unit.Name("foo/0")).Return(unit.UUID("foo-uuid"), nil)
	s.st.EXPECT().GetUnitAddresses(gomock.Any(), unit.UUID("foo-uuid")).Return(matchingScopeAddrs, nil)

	addrs, err := s.service(c).GetUnitPrivateAddress(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	// Since the second address is higher in hierarchy of scope match, it should
	// be returned.
	c.Check(addrs, tc.DeepEquals, matchingScopeAddrs[1])
}

func (s *unitAddressSuite) TestGetUnitPrivateAddressNoAddress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil)
	s.st.EXPECT().GetUnitAddresses(gomock.Any(), unit.UUID("foo")).Return(network.SpaceAddresses{}, nil)

	_, err := s.service(c).GetUnitPrivateAddress(c.Context(), unitName)
	c.Assert(err, tc.Satisfies, network.IsNoAddressError)
}

func newControllerAPIAddress(
	value string,
	spaceUUID network.SpaceUUID,
	deviceType domainnetwork.DeviceType,
) domainnetwork.ControllerAPIAddress {
	return domainnetwork.ControllerAPIAddress{
		SpaceAddress: network.SpaceAddress{
			SpaceID: spaceUUID,
			MachineAddress: network.MachineAddress{
				Value: value,
			},
		},
		DeviceType: deviceType,
	}
}

// toDeviceAddresses wraps the input addresses as device addresses for
// devices that do not carry the default route.
func toDeviceAddresses(addrs network.SpaceAddresses) []network.DeviceAddress {
	res := make([]network.DeviceAddress, len(addrs))
	for i, addr := range addrs {
		res[i] = network.DeviceAddress{SpaceAddress: addr}
	}
	return res
}

// cloudLocalDefaultRouteAddresses returns two cloud-local addresses: the
// lower-valued address on eth1, which does not carry the default route, and
// the higher-valued address on eth0, which does.
func cloudLocalDefaultRouteAddresses() (eth1, eth0 network.DeviceAddress) {
	eth1 = defaultRouteDeviceAddress("10.0.0.123", "10.0.0.0/24", "eth1-uuid")
	eth1.IsDefaultGatewayDevice = false
	eth1.GatewayAddress = ""
	eth0 = defaultRouteDeviceAddress("10.1.0.123", "10.1.0.0/24", "eth0-uuid")
	return eth1, eth0
}

// publicAndCloudLocalDefaultRouteAddresses returns a public address on eth1,
// which does not carry the default route, and a cloud-local address on eth0,
// which does.
func publicAndCloudLocalDefaultRouteAddresses() (public, cloudLocal network.DeviceAddress) {
	public = defaultRouteDeviceAddress("203.0.113.10", "203.0.113.0/24", "eth1-uuid")
	public.Scope = network.ScopePublic
	public.IsDefaultGatewayDevice = false
	public.GatewayAddress = ""
	cloudLocal = defaultRouteDeviceAddress("10.1.0.123", "10.1.0.0/24", "eth0-uuid")
	return public, cloudLocal
}
