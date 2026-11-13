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

// TestGetPublicAddressesCloudLocalPrefersDefaultRouteSource checks that, with
// no public addresses, the cloud-local address on the device carrying the
// default route is preferred over a cloud-local address with a lower value
// on another device.
func (s *unitAddressSuite) TestGetPublicAddressesCloudLocalPrefersDefaultRouteSource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	unitName := unit.Name("foo/0")

	eth1Addr := network.DeviceAddress{
		SpaceAddress: network.SpaceAddress{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.123",
				CIDR:       "10.0.0.0/24",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopeCloudLocal,
			},
		},
		DeviceUUID: "eth1-uuid",
	}
	eth0Addr := network.DeviceAddress{
		SpaceAddress: network.SpaceAddress{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "10.1.0.123",
				CIDR:       "10.1.0.0/24",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopeCloudLocal,
			},
		},
		DeviceUUID:             "eth0-uuid",
		IsDefaultGatewayDevice: true,
		GatewayAddress:         "10.1.0.1",
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil).Times(2)
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).
		Return([]network.DeviceAddress{eth1Addr, eth0Addr}, nil).Times(2)

	// Act
	addrs, err := s.service(c).GetUnitPublicAddresses(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	addr, err := s.service(c).GetUnitPublicAddress(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)

	// Assert
	expectedDefault := eth0Addr.SpaceAddress
	expectedDefault.IsDefaultRouteSource = true
	c.Check(addrs, tc.DeepEquals, network.SpaceAddresses{
		expectedDefault, eth1Addr.SpaceAddress,
	})
	c.Check(addr, tc.DeepEquals, expectedDefault)
}

// TestGetPublicAddressesPublicBeatsDefaultRouteSource checks that a public
// address is returned in preference to the default route source address when
// that is only cloud-local, and that the cloud-local address is not returned
// alongside it. That is, marking the default route source does not change the
// scope tier filter.
func (s *unitAddressSuite) TestGetPublicAddressesPublicBeatsDefaultRouteSource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	unitName := unit.Name("foo/0")

	publicAddr := network.DeviceAddress{
		SpaceAddress: network.SpaceAddress{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "203.0.113.10",
				CIDR:       "203.0.113.0/24",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopePublic,
			},
		},
		DeviceUUID: "eth1-uuid",
	}
	cloudLocalAddr := network.DeviceAddress{
		SpaceAddress: network.SpaceAddress{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "10.1.0.123",
				CIDR:       "10.1.0.0/24",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopeCloudLocal,
			},
		},
		DeviceUUID:             "eth0-uuid",
		IsDefaultGatewayDevice: true,
		GatewayAddress:         "10.1.0.1",
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil).Times(2)
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).
		Return([]network.DeviceAddress{cloudLocalAddr, publicAddr}, nil).Times(2)

	// Act
	addrs, err := s.service(c).GetUnitPublicAddresses(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	addr, err := s.service(c).GetUnitPublicAddress(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)

	// Assert
	c.Check(addrs, tc.DeepEquals, network.SpaceAddresses{publicAddr.SpaceAddress})
	c.Check(addr, tc.DeepEquals, publicAddr.SpaceAddress)
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
