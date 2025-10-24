// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	corerelation "github.com/juju/juju/core/relation"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	"github.com/juju/juju/internal/errors"
)

type relationNetworkServiceSuite struct {
	baseSuite
}

func TestRelationNetworkServiceSuite(t *testing.T) {
	tc.Run(t, &relationNetworkServiceSuite{})
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, corerelation.NewUUID)
	cidrs := []string{"192.0.2.0/24", "198.51.100.0/24"}
	saasIngressAllow := []string{"0.0.0.0/0", "::/0"}

	// The service will cast modelState to ModelRelationNetworkState
	s.modelState.EXPECT().AddRelationNetworkIngress(gomock.Any(), relationUUID.String(), cidrs).Return(nil)

	// Act
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID, saasIngressAllow, cidrs)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressSingleCIDR(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, corerelation.NewUUID)
	cidr := []string{"192.0.2.0/24"}
	saasIngressAllow := []string{"0.0.0.0/0", "::/0"}

	s.modelState.EXPECT().AddRelationNetworkIngress(gomock.Any(), relationUUID.String(), cidr).Return(nil)

	// Act
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID, saasIngressAllow, cidr)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressEmptyRelationUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	cidrs := []string{"192.0.2.0/24"}
	saasIngressAllow := []string{"0.0.0.0/0", "::/0"}

	// Act - No mock expectations needed as validation happens before state call
	err := s.service(c).AddRelationNetworkIngress(c.Context(), "", saasIngressAllow, cidrs)

	// Assert
	c.Assert(err, tc.ErrorMatches, `relation uuid cannot be empty`)
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressNoCIDRs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, corerelation.NewUUID)
	saasIngressAllow := []string{"0.0.0.0/0", "::/0"}

	s.modelState.EXPECT().AddRelationNetworkIngress(gomock.Any(), relationUUID.String(), []string{}).Return(nil)

	// Act
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID, saasIngressAllow, []string{})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, corerelation.NewUUID)
	cidrs := []string{"192.0.2.0/24"}
	saasIngressAllow := []string{"0.0.0.0/0", "::/0"}
	expectedErr := errors.Errorf("state error")

	s.modelState.EXPECT().AddRelationNetworkIngress(gomock.Any(), relationUUID.String(), cidrs).Return(expectedErr)

	// Act
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID, saasIngressAllow, cidrs)

	// Assert
	c.Assert(err, tc.ErrorMatches, "state error")
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressMultipleCIDRs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, corerelation.NewUUID)
	cidrs := []string{
		"192.0.2.0/24",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"2001:db8::/32",
	}
	saasIngressAllow := []string{"0.0.0.0/0", "::/0"}

	s.modelState.EXPECT().AddRelationNetworkIngress(gomock.Any(), relationUUID.String(), cidrs).Return(nil)

	// Act
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID, saasIngressAllow, cidrs)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressInvalidUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	invalidUUID := corerelation.UUID("not-a-uuid")
	cidrs := []string{"192.0.2.0/24"}
	saasIngressAllow := []string{"0.0.0.0/0", "::/0"}

	// Act - No mock expectations needed as validation happens before state call
	err := s.service(c).AddRelationNetworkIngress(c.Context(), invalidUUID, saasIngressAllow, cidrs)

	// Assert
	c.Assert(err, tc.ErrorMatches, `relation uuid "not-a-uuid": not valid`)
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressInvalidCIDRv4(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, corerelation.NewUUID)
	cidrs := []string{"192.0.2.0/24", "not-a-cidr", "198.51.100.0/24"}
	saasIngressAllow := []string{"0.0.0.0/0", "::/0"}

	// Act - No mock expectations needed as validation happens before state call
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID, saasIngressAllow, cidrs)

	// Assert
	c.Assert(err, tc.ErrorMatches, `invalid CIDR address: not-a-cidr`)
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressInvalidCIDRFormat(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, corerelation.NewUUID)
	cidrs := []string{"192.0.2.256/24"} // Invalid IP address
	saasIngressAllow := []string{"0.0.0.0/0", "::/0"}

	// Act - No mock expectations needed as validation happens before state call
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID, saasIngressAllow, cidrs)

	// Assert
	c.Assert(err, tc.ErrorMatches, `invalid CIDR address: 192.0.2.256/24`)
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressIPv6CIDR(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, corerelation.NewUUID)
	cidrs := []string{"2001:db8::/32", "2001:db8:1::/48"}
	saasIngressAllow := []string{"0.0.0.0/0", "::/0"}

	s.modelState.EXPECT().AddRelationNetworkIngress(gomock.Any(), relationUUID.String(), cidrs).Return(nil)

	// Act
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID, saasIngressAllow, cidrs)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressMixedIPv4IPv6(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, corerelation.NewUUID)
	cidrs := []string{"192.0.2.0/24", "2001:db8::/32"}
	saasIngressAllow := []string{"0.0.0.0/0", "::/0"}

	s.modelState.EXPECT().AddRelationNetworkIngress(gomock.Any(), relationUUID.String(), cidrs).Return(nil)

	// Act
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID, saasIngressAllow, cidrs)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressSubnetNotInWhitelist(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, corerelation.NewUUID)
	cidrs := []string{"203.0.113.0/24"}
	saasIngressAllow := []string{"192.0.2.0/24"}

	// Act - No mock expectations needed as validation happens before state call
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID, saasIngressAllow, cidrs)

	// Assert
	c.Assert(err, tc.ErrorMatches, `subnet 203.0.113.0/24 not in firewall whitelist`)
	c.Assert(errors.Is(err, crossmodelrelationerrors.SubnetNotInWhitelist), tc.Equals, true)
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressSubnetInWhitelist(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, corerelation.NewUUID)
	cidrs := []string{"192.0.2.128/25"}
	saasIngressAllow := []string{"192.0.2.0/24"}

	s.modelState.EXPECT().AddRelationNetworkIngress(gomock.Any(), relationUUID.String(), cidrs).Return(nil)

	// Act
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID, saasIngressAllow, cidrs)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressMultipleSubnetsPartialWhitelist(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, corerelation.NewUUID)
	cidrs := []string{"192.0.2.128/25", "203.0.113.0/24"}
	saasIngressAllow := []string{"192.0.2.0/24"}

	// Act - No mock expectations needed as validation happens before state call
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID, saasIngressAllow, cidrs)

	// Assert
	c.Assert(err, tc.ErrorMatches, `subnet 203.0.113.0/24 not in firewall whitelist`)
	c.Assert(errors.Is(err, crossmodelrelationerrors.SubnetNotInWhitelist), tc.Equals, true)
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressEmptyWhitelistAllowsAll(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, corerelation.NewUUID)
	cidrs := []string{"192.0.2.0/24", "198.51.100.0/24"}
	saasIngressAllow := []string{}

	s.modelState.EXPECT().AddRelationNetworkIngress(gomock.Any(), relationUUID.String(), cidrs).Return(nil)

	// Act
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID, saasIngressAllow, cidrs)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressInvalidWhitelistCIDR(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, corerelation.NewUUID)
	cidrs := []string{"192.0.2.0/24"}
	saasIngressAllow := []string{"not-a-cidr"}

	// Act - No mock expectations needed as validation happens before state call
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID, saasIngressAllow, cidrs)

	// Assert
	c.Assert(err, tc.ErrorMatches, `invalid CIDR address: not-a-cidr`)
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressMultipleWhitelistRanges(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, corerelation.NewUUID)
	cidrs := []string{"192.0.2.128/25", "198.51.100.128/25"}
	saasIngressAllow := []string{"192.0.2.0/24", "198.51.100.0/24"}

	s.modelState.EXPECT().AddRelationNetworkIngress(gomock.Any(), relationUUID.String(), cidrs).Return(nil)

	// Act
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID, saasIngressAllow, cidrs)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressIPv6Whitelist(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, corerelation.NewUUID)
	cidrs := []string{"2001:db8:1::/48"}
	saasIngressAllow := []string{"2001:db8::/32"}

	s.modelState.EXPECT().AddRelationNetworkIngress(gomock.Any(), relationUUID.String(), cidrs).Return(nil)

	// Act
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID, saasIngressAllow, cidrs)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressIPv6NotInWhitelist(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, corerelation.NewUUID)
	cidrs := []string{"2001:db9::/32"}
	saasIngressAllow := []string{"2001:db8::/32"}

	// Act - No mock expectations needed as validation happens before state call
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID, saasIngressAllow, cidrs)

	// Assert
	c.Assert(err, tc.ErrorMatches, `subnet 2001:db9::/32 not in firewall whitelist`)
	c.Assert(errors.Is(err, crossmodelrelationerrors.SubnetNotInWhitelist), tc.Equals, true)
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressMixedIPv4IPv6Whitelist(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, corerelation.NewUUID)
	cidrs := []string{"192.0.2.0/25", "2001:db8:1::/48"}
	saasIngressAllow := []string{"192.0.2.0/24", "2001:db8::/32"}

	s.modelState.EXPECT().AddRelationNetworkIngress(gomock.Any(), relationUUID.String(), cidrs).Return(nil)

	// Act
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID, saasIngressAllow, cidrs)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationNetworkServiceSuite) TestGetRelationNetworkIngress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, corerelation.NewUUID)
	expectedCIDRs := []string{"192.0.2.0/24", "198.51.100.0/24"}

	s.modelState.EXPECT().GetRelationNetworkIngress(gomock.Any(), relationUUID.String()).Return(expectedCIDRs, nil)

	// Act
	obtainedCIDRs, err := s.service(c).GetRelationNetworkIngress(c.Context(), relationUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedCIDRs, tc.DeepEquals, expectedCIDRs)
}

func (s *relationNetworkServiceSuite) TestGetRelationNetworkIngressSingleCIDR(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, corerelation.NewUUID)
	expectedCIDRs := []string{"192.0.2.0/24"}

	s.modelState.EXPECT().GetRelationNetworkIngress(gomock.Any(), relationUUID.String()).Return(expectedCIDRs, nil)

	// Act
	obtainedCIDRs, err := s.service(c).GetRelationNetworkIngress(c.Context(), relationUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedCIDRs, tc.DeepEquals, expectedCIDRs)
}

func (s *relationNetworkServiceSuite) TestGetRelationNetworkIngressEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, corerelation.NewUUID)
	expectedCIDRs := []string{}

	s.modelState.EXPECT().GetRelationNetworkIngress(gomock.Any(), relationUUID.String()).Return(expectedCIDRs, nil)

	// Act
	obtainedCIDRs, err := s.service(c).GetRelationNetworkIngress(c.Context(), relationUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedCIDRs, tc.HasLen, 0)
}

func (s *relationNetworkServiceSuite) TestGetRelationNetworkIngressEmptyRelationUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Act - No mock expectations needed as validation happens before state call
	obtainedCIDRs, err := s.service(c).GetRelationNetworkIngress(c.Context(), "")

	// Assert
	c.Assert(err, tc.ErrorMatches, "relation uuid cannot be empty")
	c.Check(obtainedCIDRs, tc.IsNil)
}

func (s *relationNetworkServiceSuite) TestGetRelationNetworkIngressInvalidUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	invalidUUID := corerelation.UUID("not-a-uuid")

	// Act - No mock expectations needed as validation happens before state call
	obtainedCIDRs, err := s.service(c).GetRelationNetworkIngress(c.Context(), invalidUUID)

	// Assert
	c.Assert(err, tc.ErrorMatches, `relation uuid "not-a-uuid": not valid`)
	c.Check(obtainedCIDRs, tc.IsNil)
}

func (s *relationNetworkServiceSuite) TestGetRelationNetworkIngressStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, corerelation.NewUUID)
	expectedErr := errors.Errorf("state error")

	s.modelState.EXPECT().GetRelationNetworkIngress(gomock.Any(), relationUUID.String()).Return(nil, expectedErr)

	// Act
	obtainedCIDRs, err := s.service(c).GetRelationNetworkIngress(c.Context(), relationUUID)

	// Assert
	c.Assert(err, tc.ErrorMatches, "state error")
	c.Check(obtainedCIDRs, tc.IsNil)
}

func (s *relationNetworkServiceSuite) TestGetRelationNetworkIngressMultipleCIDRs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, corerelation.NewUUID)
	expectedCIDRs := []string{
		"192.0.2.0/24",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"2001:db8::/32",
	}

	s.modelState.EXPECT().GetRelationNetworkIngress(gomock.Any(), relationUUID.String()).Return(expectedCIDRs, nil)

	// Act
	obtainedCIDRs, err := s.service(c).GetRelationNetworkIngress(c.Context(), relationUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedCIDRs, tc.DeepEquals, expectedCIDRs)
}

func (s *relationNetworkServiceSuite) TestGetRelationNetworkIngressIPv6CIDR(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, corerelation.NewUUID)
	expectedCIDRs := []string{"2001:db8::/32", "2001:db8:1::/48"}

	s.modelState.EXPECT().GetRelationNetworkIngress(gomock.Any(), relationUUID.String()).Return(expectedCIDRs, nil)

	// Act
	obtainedCIDRs, err := s.service(c).GetRelationNetworkIngress(c.Context(), relationUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedCIDRs, tc.DeepEquals, expectedCIDRs)
}

func (s *relationNetworkServiceSuite) TestGetRelationNetworkIngressMixedIPv4IPv6(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, corerelation.NewUUID)
	expectedCIDRs := []string{"192.0.2.0/24", "2001:db8::/32"}

	s.modelState.EXPECT().GetRelationNetworkIngress(gomock.Any(), relationUUID.String()).Return(expectedCIDRs, nil)

	// Act
	obtainedCIDRs, err := s.service(c).GetRelationNetworkIngress(c.Context(), relationUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedCIDRs, tc.DeepEquals, expectedCIDRs)
}
