// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
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
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo-uuid")).Return(nonMatchingScopeAddrs, nil)

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
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).Return(matchingScopeAddrs, nil)

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
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).Return(matchingScopeAddrs, nil)

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
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).Return(matchingScopeAddrs, nil)

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
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).Return(matchingScopeAddrs, nil)

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
				Value:      "10.0.0.1",
				ConfigType: network.ConfigStatic,
				Type:       network.IPv4Address,
				Scope:      network.ScopePublic,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.2",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopePublic,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "54.32.1.2",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopeMachineLocal,
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
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).Return(unitAddresses, nil)

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
				Value:      "10.0.0.1",
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
				Scope:      network.ScopeCloudLocal,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "54.32.1.2",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopeMachineLocal,
			},
		},
		{
			SpaceID: network.AlphaSpaceId,
			MachineAddress: network.MachineAddress{
				Value:      "54.32.1.3",
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopeMachineLocal,
			},
		},
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil)
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).Return(unitAddresses, nil)

	addrs, err := s.service(c).GetUnitPublicAddresses(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	// The two cloud-local addresses should be returned because there are no
	// public ones.
	c.Check(addrs, tc.DeepEquals, unitAddresses[0:2])
}

func (s *unitAddressSuite) TestGetPublicAddressesNoAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil)
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).Return(network.SpaceAddresses{}, nil)

	_, err := s.service(c).GetUnitPublicAddresses(c.Context(), unitName)
	c.Assert(err, tc.Satisfies, network.IsNoAddressError)
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

	addr, err := s.service(c).GetUnitPrivateAddress(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	// We always return the (first) container address even if it doesn't match
	// the scope.
	c.Assert(addr, tc.DeepEquals, nonMatchingScopeAddrs[0])
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
