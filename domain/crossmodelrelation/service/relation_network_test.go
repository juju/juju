// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
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
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	cidrs := []string{"192.0.2.0/24", "198.51.100.0/24"}

	// The service will cast modelState to ModelRelationNetworkState
	s.modelState.EXPECT().AddRelationNetworkIngress(gomock.Any(), relationUUID, cidrs[0], cidrs[1]).Return(nil)

	// Act
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID, cidrs...)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressSingleCIDR(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	cidr := "192.0.2.0/24"

	s.modelState.EXPECT().AddRelationNetworkIngress(gomock.Any(), relationUUID, cidr).Return(nil)

	// Act
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID, cidr)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressEmptyRelationUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	cidrs := []string{"192.0.2.0/24"}

	// Act - No mock expectations needed as validation happens before state call
	err := s.service(c).AddRelationNetworkIngress(c.Context(), "", cidrs...)

	// Assert
	c.Assert(err, tc.ErrorMatches, "relation UUID cannot be empty")
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressNoCIDRs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, uuid.NewUUID).String()

	// Act - No mock expectations needed as validation happens before state call
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID)

	// Assert
	c.Assert(err, tc.ErrorMatches, "at least one CIDR must be provided")
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressEmptyCIDR(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	cidrs := []string{"192.0.2.0/24", "", "198.51.100.0/24"}

	// Act - No mock expectations needed as validation happens before state call
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID, cidrs...)

	// Assert
	c.Assert(err, tc.ErrorMatches, "CIDR cannot be empty")
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	cidrs := []string{"192.0.2.0/24"}
	expectedErr := errors.Errorf("state error")

	s.modelState.EXPECT().AddRelationNetworkIngress(gomock.Any(), relationUUID, cidrs[0]).Return(expectedErr)

	// Act
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID, cidrs...)

	// Assert
	c.Assert(err, tc.ErrorMatches, "state error")
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressMultipleCIDRs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	cidrs := []string{
		"192.0.2.0/24",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"2001:db8::/32",
	}

	s.modelState.EXPECT().AddRelationNetworkIngress(
		gomock.Any(),
		relationUUID,
		cidrs[0], cidrs[1], cidrs[2], cidrs[3],
	).Return(nil)

	// Act
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID, cidrs...)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressInvalidUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	invalidUUID := "not-a-uuid"
	cidrs := []string{"192.0.2.0/24"}

	// Act - No mock expectations needed as validation happens before state call
	err := s.service(c).AddRelationNetworkIngress(c.Context(), invalidUUID, cidrs...)

	// Assert
	c.Assert(err, tc.ErrorMatches, `relation UUID "not-a-uuid" is not a valid UUID`)
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressInvalidCIDRv4(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	cidrs := []string{"192.0.2.0/24", "not-a-cidr", "198.51.100.0/24"}

	// Act - No mock expectations needed as validation happens before state call
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID, cidrs...)

	// Assert
	c.Assert(err, tc.ErrorMatches, `CIDR "not-a-cidr" is not valid:.*`)
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressInvalidCIDRFormat(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	cidrs := []string{"192.0.2.256/24"} // Invalid IP address

	// Act - No mock expectations needed as validation happens before state call
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID, cidrs...)

	// Assert
	c.Assert(err, tc.ErrorMatches, `CIDR "192.0.2.256/24" is not valid:.*`)
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressIPv6CIDR(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	cidrs := []string{"2001:db8::/32", "2001:db8:1::/48"}

	s.modelState.EXPECT().AddRelationNetworkIngress(gomock.Any(), relationUUID, cidrs[0], cidrs[1]).Return(nil)

	// Act
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID, cidrs...)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationNetworkServiceSuite) TestAddRelationNetworkIngressMixedIPv4IPv6(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	cidrs := []string{"192.0.2.0/24", "2001:db8::/32"}

	s.modelState.EXPECT().AddRelationNetworkIngress(gomock.Any(), relationUUID, cidrs[0], cidrs[1]).Return(nil)

	// Act
	err := s.service(c).AddRelationNetworkIngress(c.Context(), relationUUID, cidrs...)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationNetworkServiceSuite) TestGetRelationNetworkIngress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	expectedCIDRs := []string{"192.0.2.0/24", "198.51.100.0/24"}

	s.modelState.EXPECT().GetRelationNetworkIngress(gomock.Any(), relationUUID).Return(expectedCIDRs, nil)

	// Act
	obtainedCIDRs, err := s.service(c).GetRelationNetworkIngress(c.Context(), relationUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedCIDRs, tc.DeepEquals, expectedCIDRs)
}

func (s *relationNetworkServiceSuite) TestGetRelationNetworkIngressSingleCIDR(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	expectedCIDRs := []string{"192.0.2.0/24"}

	s.modelState.EXPECT().GetRelationNetworkIngress(gomock.Any(), relationUUID).Return(expectedCIDRs, nil)

	// Act
	obtainedCIDRs, err := s.service(c).GetRelationNetworkIngress(c.Context(), relationUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedCIDRs, tc.DeepEquals, expectedCIDRs)
}

func (s *relationNetworkServiceSuite) TestGetRelationNetworkIngressEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	expectedCIDRs := []string{}

	s.modelState.EXPECT().GetRelationNetworkIngress(gomock.Any(), relationUUID).Return(expectedCIDRs, nil)

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
	c.Assert(err, tc.ErrorMatches, "relation UUID cannot be empty")
	c.Check(obtainedCIDRs, tc.IsNil)
}

func (s *relationNetworkServiceSuite) TestGetRelationNetworkIngressInvalidUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	invalidUUID := "not-a-uuid"

	// Act - No mock expectations needed as validation happens before state call
	obtainedCIDRs, err := s.service(c).GetRelationNetworkIngress(c.Context(), invalidUUID)

	// Assert
	c.Assert(err, tc.ErrorMatches, `relation UUID "not-a-uuid" is not a valid UUID`)
	c.Check(obtainedCIDRs, tc.IsNil)
}

func (s *relationNetworkServiceSuite) TestGetRelationNetworkIngressStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	expectedErr := errors.Errorf("state error")

	s.modelState.EXPECT().GetRelationNetworkIngress(gomock.Any(), relationUUID).Return(nil, expectedErr)

	// Act
	obtainedCIDRs, err := s.service(c).GetRelationNetworkIngress(c.Context(), relationUUID)

	// Assert
	c.Assert(err, tc.ErrorMatches, "state error")
	c.Check(obtainedCIDRs, tc.IsNil)
}

func (s *relationNetworkServiceSuite) TestGetRelationNetworkIngressMultipleCIDRs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	expectedCIDRs := []string{
		"192.0.2.0/24",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"2001:db8::/32",
	}

	s.modelState.EXPECT().GetRelationNetworkIngress(gomock.Any(), relationUUID).Return(expectedCIDRs, nil)

	// Act
	obtainedCIDRs, err := s.service(c).GetRelationNetworkIngress(c.Context(), relationUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedCIDRs, tc.DeepEquals, expectedCIDRs)
}

func (s *relationNetworkServiceSuite) TestGetRelationNetworkIngressIPv6CIDR(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	expectedCIDRs := []string{"2001:db8::/32", "2001:db8:1::/48"}

	s.modelState.EXPECT().GetRelationNetworkIngress(gomock.Any(), relationUUID).Return(expectedCIDRs, nil)

	// Act
	obtainedCIDRs, err := s.service(c).GetRelationNetworkIngress(c.Context(), relationUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedCIDRs, tc.DeepEquals, expectedCIDRs)
}

func (s *relationNetworkServiceSuite) TestGetRelationNetworkIngressMixedIPv4IPv6(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	expectedCIDRs := []string{"192.0.2.0/24", "2001:db8::/32"}

	s.modelState.EXPECT().GetRelationNetworkIngress(gomock.Any(), relationUUID).Return(expectedCIDRs, nil)

	// Act
	obtainedCIDRs, err := s.service(c).GetRelationNetworkIngress(c.Context(), relationUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedCIDRs, tc.DeepEquals, expectedCIDRs)
}
