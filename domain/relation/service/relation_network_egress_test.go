// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/errors"
)

type relationNetworkEgressServiceSuite struct {
	baseServiceSuite
}

func TestRelationNetworkEgressServiceSuite(t *testing.T) {
	tc.Run(t, &relationNetworkEgressServiceSuite{})
}

func (s *relationNetworkEgressServiceSuite) TestAddRelationNetworkEgressSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	cidrs := []string{"192.0.2.0/24", "198.51.100.0/24"}

	ep1 := relation.Endpoint{ApplicationName: "wordpress"}
	ep2 := relation.Endpoint{ApplicationName: "mysql"}

	s.state.EXPECT().AddRelation(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		cidrs[0],
		cidrs[1],
	).Return(ep1, ep2, nil)

	// Act
	_, _, err := s.service.AddRelation(c.Context(), "wordpress:db", "mysql:server", cidrs...)

	// Assert
	c.Assert(err, tc.IsNil)
}

func (s *relationNetworkEgressServiceSuite) TestAddRelationNetworkEgressNoCIDRs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	ep1 := relation.Endpoint{ApplicationName: "wordpress"}
	ep2 := relation.Endpoint{ApplicationName: "mysql"}

	s.state.EXPECT().AddRelation(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(ep1, ep2, nil)

	// Act - passing no CIDRs should work fine
	_, _, err := s.service.AddRelation(c.Context(), "wordpress:db", "mysql:server")

	// Assert
	c.Assert(err, tc.IsNil)
}

func (s *relationNetworkEgressServiceSuite) TestAddRelationNetworkEgressEmptyCIDR(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	cidrs := []string{"192.0.2.0/24", "", "198.51.100.0/24"}

	// Act
	_, _, err := s.service.AddRelation(c.Context(), "wordpress:db", "mysql:server", cidrs...)

	// Assert
	c.Assert(err, tc.ErrorMatches, `CIDR cannot be empty`)
}

func (s *relationNetworkEgressServiceSuite) TestAddRelationNetworkEgressInvalidCIDRFormat(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	cidrs := []string{"not-a-valid-cidr"}

	// Act
	_, _, err := s.service.AddRelation(c.Context(), "wordpress:db", "mysql:server", cidrs...)

	// Assert
	c.Assert(err, tc.ErrorMatches, `CIDR "not-a-valid-cidr" is not valid.*`)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *relationNetworkEgressServiceSuite) TestAddRelationNetworkEgressInvalidCIDRInList(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	cidrs := []string{"192.0.2.0/24", "invalid-cidr", "198.51.100.0/24"}

	// Act
	_, _, err := s.service.AddRelation(c.Context(), "wordpress:db", "mysql:server", cidrs...)

	// Assert
	c.Assert(err, tc.ErrorMatches, `CIDR "invalid-cidr" is not valid.*`)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *relationNetworkEgressServiceSuite) TestAddRelationNetworkEgressIPv4MissingMask(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	cidrs := []string{"192.0.2.0"}

	// Act
	_, _, err := s.service.AddRelation(c.Context(), "wordpress:db", "mysql:server", cidrs...)

	// Assert
	c.Assert(err, tc.ErrorMatches, `CIDR .* is not valid.*`)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *relationNetworkEgressServiceSuite) TestAddRelationNetworkEgressIPv4InvalidIP(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	cidrs := []string{"999.999.999.999/24"}

	// Act
	_, _, err := s.service.AddRelation(c.Context(), "wordpress:db", "mysql:server", cidrs...)

	// Assert
	c.Assert(err, tc.ErrorMatches, `CIDR .* is not valid.*`)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *relationNetworkEgressServiceSuite) TestAddRelationNetworkEgressIPv4InvalidMask(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	cidrs := []string{"192.0.2.0/33"}

	// Act
	_, _, err := s.service.AddRelation(c.Context(), "wordpress:db", "mysql:server", cidrs...)

	// Assert
	c.Assert(err, tc.ErrorMatches, `CIDR .* is not valid.*`)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *relationNetworkEgressServiceSuite) TestAddRelationNetworkEgressIPv4NegativeMask(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	cidrs := []string{"192.0.2.0/-1"}

	// Act
	_, _, err := s.service.AddRelation(c.Context(), "wordpress:db", "mysql:server", cidrs...)

	// Assert
	c.Assert(err, tc.ErrorMatches, `CIDR .* is not valid.*`)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *relationNetworkEgressServiceSuite) TestAddRelationNetworkEgressIPv4NoIP(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	cidrs := []string{"/24"}

	// Act
	_, _, err := s.service.AddRelation(c.Context(), "wordpress:db", "mysql:server", cidrs...)

	// Assert
	c.Assert(err, tc.ErrorMatches, `CIDR .* is not valid.*`)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *relationNetworkEgressServiceSuite) TestAddRelationNetworkEgressIPv6MissingMask(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	cidrs := []string{"2001:db8::1"}

	// Act
	_, _, err := s.service.AddRelation(c.Context(), "wordpress:db", "mysql:server", cidrs...)

	// Assert
	c.Assert(err, tc.ErrorMatches, `CIDR .* is not valid.*`)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *relationNetworkEgressServiceSuite) TestAddRelationNetworkEgressIPv6InvalidIP(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	cidrs := []string{"gggg::/32"}

	// Act
	_, _, err := s.service.AddRelation(c.Context(), "wordpress:db", "mysql:server", cidrs...)

	// Assert
	c.Assert(err, tc.ErrorMatches, `CIDR .* is not valid.*`)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *relationNetworkEgressServiceSuite) TestAddRelationNetworkEgressIPv6InvalidMask(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	cidrs := []string{"2001:db8::/129"}

	// Act
	_, _, err := s.service.AddRelation(c.Context(), "wordpress:db", "mysql:server", cidrs...)

	// Assert
	c.Assert(err, tc.ErrorMatches, `CIDR .* is not valid.*`)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *relationNetworkEgressServiceSuite) TestAddRelationNetworkEgressIPv6NegativeMask(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	cidrs := []string{"2001:db8::/-1"}

	// Act
	_, _, err := s.service.AddRelation(c.Context(), "wordpress:db", "mysql:server", cidrs...)

	// Assert
	c.Assert(err, tc.ErrorMatches, `CIDR .* is not valid.*`)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *relationNetworkEgressServiceSuite) TestAddRelationNetworkEgressSingleCIDR(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	cidrs := []string{"192.0.2.0/24"}

	ep1 := relation.Endpoint{ApplicationName: "wordpress"}
	ep2 := relation.Endpoint{ApplicationName: "mysql"}

	s.state.EXPECT().AddRelation(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		cidrs[0],
	).Return(ep1, ep2, nil)

	// Act
	_, _, err := s.service.AddRelation(c.Context(), "wordpress:db", "mysql:server", cidrs...)

	// Assert
	c.Assert(err, tc.IsNil)
}

func (s *relationNetworkEgressServiceSuite) TestAddRelationNetworkEgressIPv6(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	cidrs := []string{"2001:db8::/32", "2001:db8:1::/48"}

	ep1 := relation.Endpoint{ApplicationName: "wordpress"}
	ep2 := relation.Endpoint{ApplicationName: "mysql"}

	s.state.EXPECT().AddRelation(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		cidrs[0],
		cidrs[1],
	).Return(ep1, ep2, nil)

	// Act
	_, _, err := s.service.AddRelation(c.Context(), "wordpress:db", "mysql:server", cidrs...)

	// Assert
	c.Assert(err, tc.IsNil)
}

func (s *relationNetworkEgressServiceSuite) TestAddRelationNetworkEgressMixedIPVersions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	cidrs := []string{"192.0.2.0/24", "2001:db8::/32", "198.51.100.0/24"}

	ep1 := relation.Endpoint{ApplicationName: "wordpress"}
	ep2 := relation.Endpoint{ApplicationName: "mysql"}

	s.state.EXPECT().AddRelation(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		cidrs[0],
		cidrs[1],
		cidrs[2],
	).Return(ep1, ep2, nil)

	// Act
	_, _, err := s.service.AddRelation(c.Context(), "wordpress:db", "mysql:server", cidrs...)

	// Assert
	c.Assert(err, tc.IsNil)
}

func (s *relationNetworkEgressServiceSuite) TestAddRelationNetworkEgressStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	cidrs := []string{"192.0.2.0/24"}
	stateErr := errors.Errorf("database error")

	ep1 := relation.Endpoint{ApplicationName: "wordpress"}
	ep2 := relation.Endpoint{ApplicationName: "mysql"}

	s.state.EXPECT().AddRelation(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		cidrs[0],
	).Return(ep1, ep2, stateErr)

	// Act
	_, _, err := s.service.AddRelation(c.Context(), "wordpress:db", "mysql:server", cidrs...)

	// Assert
	c.Assert(err, tc.ErrorMatches, `.*database error.*`)
}

func (s *relationNetworkEgressServiceSuite) TestAddRelationNetworkEgressHostBits(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	// CIDR with host bits set (192.168.1.5/24 instead of 192.168.1.0/24)
	cidrs := []string{"192.0.2.5/24"}

	ep1 := relation.Endpoint{ApplicationName: "wordpress"}
	ep2 := relation.Endpoint{ApplicationName: "mysql"}

	s.state.EXPECT().AddRelation(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		cidrs[0],
	).Return(ep1, ep2, nil)

	// Act
	_, _, err := s.service.AddRelation(c.Context(), "wordpress:db", "mysql:server", cidrs...)

	// Assert
	c.Assert(err, tc.IsNil)
}

func (s *relationNetworkEgressServiceSuite) TestAddRelationNetworkEgressSingleHostIPv4(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	cidrs := []string{"192.0.2.5/32"}

	ep1 := relation.Endpoint{ApplicationName: "wordpress"}
	ep2 := relation.Endpoint{ApplicationName: "mysql"}

	s.state.EXPECT().AddRelation(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		cidrs[0],
	).Return(ep1, ep2, nil)

	// Act
	_, _, err := s.service.AddRelation(c.Context(), "wordpress:db", "mysql:server", cidrs...)

	// Assert
	c.Assert(err, tc.IsNil)
}

func (s *relationNetworkEgressServiceSuite) TestAddRelationNetworkEgressSingleHostIPv6(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	cidrs := []string{"2001:db8::1/128"}

	ep1 := relation.Endpoint{ApplicationName: "wordpress"}
	ep2 := relation.Endpoint{ApplicationName: "mysql"}

	s.state.EXPECT().AddRelation(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		cidrs[0],
	).Return(ep1, ep2, nil)

	// Act
	_, _, err := s.service.AddRelation(c.Context(), "wordpress:db", "mysql:server", cidrs...)

	// Assert
	c.Assert(err, tc.IsNil)
}

func (s *relationNetworkEgressServiceSuite) TestAddRelationNetworkEgressMultipleCIDRs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	cidrs := []string{
		"192.0.2.0/24",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"192.0.2.128/25",
		"2001:db8::/32",
	}

	ep1 := relation.Endpoint{ApplicationName: "wordpress"}
	ep2 := relation.Endpoint{ApplicationName: "mysql"}

	s.state.EXPECT().AddRelation(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		cidrs[0],
		cidrs[1],
		cidrs[2],
		cidrs[3],
		cidrs[4],
	).Return(ep1, ep2, nil)

	// Act
	_, _, err := s.service.AddRelation(c.Context(), "wordpress:db", "mysql:server", cidrs...)

	// Assert
	c.Assert(err, tc.IsNil)
}
