// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	domainnetwork "github.com/juju/juju/domain/network"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type moveSubnetsSuite struct {
	testhelpers.IsolationSuite

	service *Service
	st      *MockState
}

func TestMoveSubnetsSuite(t *testing.T) {
	tc.Run(t, &moveSubnetsSuite{})
}

func (s *moveSubnetsSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.st = NewMockState(ctrl)
	s.service = NewService(s.st, loggertesting.WrapCheckLog(c))
	return ctrl
}

// TestMoveSubnetsToSpaceInvalidSubnetUUIDs tests that an error is returned when
// invalid subnet UUIDs are provided.
func (s *moveSubnetsSuite) TestMoveSubnetsToSpaceInvalidSubnetUUIDs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Invalid UUID format
	result, err := s.service.MoveSubnetsToSpace(
		c.Context(),
		[]domainnetwork.SubnetUUID{"invalid-uuid"},
		"space1",
		false,
	)

	c.Assert(err, tc.ErrorMatches, "invalid subnet UUIDs:.*")
	c.Assert(result, tc.IsNil)
}

func (s *moveSubnetsSuite) TestMoveSubnetsToSpaceStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	subnetUUID := s.newSubnetUUID(c)
	expectedErr := fmt.Errorf("state error")

	s.st.EXPECT().MoveSubnetsToSpace(gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any()).Return(nil, expectedErr)

	// Act
	result, err := s.service.MoveSubnetsToSpace(
		c.Context(),
		[]domainnetwork.SubnetUUID{subnetUUID},
		"space1",
		false,
	)

	// Assert
	c.Assert(err, tc.ErrorIs, expectedErr)
	c.Assert(result, tc.IsNil)
}

func (s *moveSubnetsSuite) TestMoveSubnetsToSpaceSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	subnetUUID := s.newSubnetUUID(c)
	expectedResult := []domainnetwork.MovedSubnets{{
		UUID:      subnetUUID,
		FromSpace: "space",
	}}

	s.st.EXPECT().MoveSubnetsToSpace(gomock.Any(),
		[]string{subnetUUID.String()},
		"space1",
		false).Return(expectedResult, nil)

	// Act
	result, err := s.service.MoveSubnetsToSpace(
		c.Context(),
		[]domainnetwork.SubnetUUID{subnetUUID},
		"space1",
		false,
	)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, expectedResult)
}

// newSubnetUUID generates a new valid SubnetUUID and asserts that no error
// occurs during its creation.
func (s *moveSubnetsSuite) newSubnetUUID(c *tc.C) domainnetwork.SubnetUUID {
	subnetUUID, err := domainnetwork.NewSubnetUUID()
	c.Assert(err, tc.IsNil)
	return subnetUUID
}
