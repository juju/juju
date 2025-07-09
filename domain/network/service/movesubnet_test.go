// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"testing"

	"github.com/juju/collections/transform"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/network"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/network/internal"
	"github.com/juju/juju/internal/errors"
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

// TestMoveSubnetsToSpaceGetAllSpacesError tests that an error is returned when
// getting all spaces fails.
func (s *moveSubnetsSuite) TestMoveSubnetsToSpaceGetAllSpacesError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	subnetUUID := s.newSubnetUUID(c)

	s.st.EXPECT().
		GetAllSpaces(gomock.Any()).
		Return(nil, errors.New("boom"))

	// Act
	result, err := s.service.MoveSubnetsToSpace(
		c.Context(),
		[]domainnetwork.SubnetUUID{subnetUUID},
		"space1",
		false,
	)

	// Assert
	c.Assert(err, tc.ErrorMatches, "getting current topology: boom")
	c.Assert(result, tc.IsNil)
}

// TestMoveSubnetsToSpaceSpaceNotFound tests that an error is returned when
// the destination space is not found.
func (s *moveSubnetsSuite) TestMoveSubnetsToSpaceSpaceNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	subnetUUID := s.newSubnetUUID(c)
	spaces := network.SpaceInfos{{}}

	s.st.EXPECT().
		GetAllSpaces(gomock.Any()).
		Return(spaces, nil)

	// Act
	result, err := s.service.MoveSubnetsToSpace(
		c.Context(),
		[]domainnetwork.SubnetUUID{subnetUUID},
		"some-unknown-space",
		false,
	)

	// Assert
	c.Assert(err, tc.ErrorMatches, `space "some-unknown-space" not found`)
	c.Assert(result, tc.IsNil)
}

// TestMoveSubnetsToSpaceGetSubnetsError tests that an error is returned when
// getting subnets fails.
func (s *moveSubnetsSuite) TestMoveSubnetsToSpaceGetSubnetsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	subnetUUID := s.newSubnetUUID(c)
	spaces := network.SpaceInfos{
		{
			ID:   "space1-id",
			Name: "space1",
		},
	}

	s.st.EXPECT().
		GetAllSpaces(gomock.Any()).
		Return(spaces, nil)

	s.st.EXPECT().
		GetSubnets(gomock.Any(), []string{subnetUUID.String()}).
		Return(nil, errors.New("boom"))

	// Act
	result, err := s.service.MoveSubnetsToSpace(
		c.Context(),
		[]domainnetwork.SubnetUUID{subnetUUID},
		"space1",
		false,
	)

	// Assert
	c.Assert(err, tc.ErrorMatches, "getting moving subnets: boom")
	c.Assert(result, tc.IsNil)
}

// TestMoveSubnetsToSpaceSubnetNotFound tests that an error is returned when
// a subnet is not found in the topology.
func (s *moveSubnetsSuite) TestMoveSubnetsToSpaceSubnetNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	subnetUUID := s.newSubnetUUID(c)
	spaces := network.SpaceInfos{
		{
			ID:   "space1-id",
			Name: "space1",
		},
	}

	subnets := network.SubnetInfos{
		{
			ID: network.Id(subnetUUID),
		},
	}

	s.st.EXPECT().
		GetAllSpaces(gomock.Any()).
		Return(spaces, nil)

	s.st.EXPECT().
		GetSubnets(gomock.Any(), []string{subnetUUID.String()}).
		Return(subnets, nil)

	// Act
	result, err := s.service.MoveSubnetsToSpace(
		c.Context(),
		[]domainnetwork.SubnetUUID{subnetUUID},
		"space1",
		false,
	)

	// Assert
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf(`building new topology: subnet IDs \[%s\] not found`, subnetUUID))
	c.Assert(result, tc.IsNil)
}

// TestMoveSubnetsToSpaceMachinesBoundToSpacesError tests that an error is returned when
// getting machines bound to spaces fails.
func (s *moveSubnetsSuite) TestMoveSubnetsToSpaceMachinesBoundToSpacesError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	subnetUUID := s.newSubnetUUID(c)
	subnetID := network.Id(subnetUUID.String())
	subnet := network.SubnetInfo{
		ID:        subnetID,
		CIDR:      "192.168.2.0/24",
		SpaceID:   "space2-id",
		SpaceName: "space2",
	}
	spaces := network.SpaceInfos{
		{
			ID:   "space1-id",
			Name: "space1",
		},
		{
			ID:   "space2-id",
			Name: "space2",
			Subnets: []network.SubnetInfo{
				subnet,
			},
		},
	}

	s.st.EXPECT().
		GetAllSpaces(gomock.Any()).
		Return(spaces, nil)

	s.st.EXPECT().
		GetSubnets(gomock.Any(), []string{subnetUUID.String()}).
		Return(network.SubnetInfos{subnet}, nil)

	s.st.EXPECT().
		GetMachinesBoundToSpaces(gomock.Any(), []string{"space2-id"}).
		Return(nil, errors.New("boom"))

	// Act
	result, err := s.service.MoveSubnetsToSpace(
		c.Context(),
		[]domainnetwork.SubnetUUID{subnetUUID},
		"space1",
		false,
	)

	// Assert
	c.Assert(err, tc.ErrorMatches, "getting machines bound to the source spaces: boom")
	c.Assert(result, tc.IsNil)
}

// TestMoveSubnetsToSpaceMachinesAllergicToSpaceError tests that an error is returned when
// getting machines allergic to the destination space fails.
func (s *moveSubnetsSuite) TestMoveSubnetsToSpaceMachinesAllergicToSpaceError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	subnetUUID := s.newSubnetUUID(c)
	subnetID := network.Id(subnetUUID.String())
	subnet := network.SubnetInfo{
		ID:        subnetID,
		CIDR:      "192.168.2.0/24",
		SpaceID:   "space2-id",
		SpaceName: "space2",
	}
	spaces := network.SpaceInfos{
		{
			ID:   "space1-id",
			Name: "space1",
		},
		{
			ID:   "space2-id",
			Name: "space2",
			Subnets: []network.SubnetInfo{
				subnet,
			},
		},
	}

	s.st.EXPECT().
		GetAllSpaces(gomock.Any()).
		Return(spaces, nil)

	s.st.EXPECT().
		GetSubnets(gomock.Any(), []string{subnetUUID.String()}).
		Return(network.SubnetInfos{subnet}, nil)

	s.st.EXPECT().
		GetMachinesBoundToSpaces(gomock.Any(), []string{"space2-id"}).
		Return(internal.CheckableMachines{}, nil)

	s.st.EXPECT().
		GetMachinesNotAllowedInSpace(gomock.Any(), "space1-id").
		Return(nil, errors.New("boom"))

	// Act
	result, err := s.service.MoveSubnetsToSpace(
		c.Context(),
		[]domainnetwork.SubnetUUID{subnetUUID},
		"space1",
		false,
	)

	// Assert
	c.Assert(err, tc.ErrorMatches, "getting machines allergic to the destination space: boom")
	c.Assert(result, tc.IsNil)
}

// TestMoveSubnetsToSpaceMachinesRejectTopology tests that an error is returned when
// machines reject the new topology.
func (s *moveSubnetsSuite) TestMoveSubnetsToSpaceMachinesRejectTopology(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Arrange
	subnetUUID := s.newSubnetUUID(c)
	subnetID := network.Id(subnetUUID.String())
	subnet := network.SubnetInfo{
		ID:        subnetID,
		CIDR:      "192.168.2.0/24",
		SpaceID:   "space2-id",
		SpaceName: "space2",
	}
	spaces := network.SpaceInfos{
		{
			ID:   "space1-id",
			Name: "space1",
		},
		{
			ID:   "space2-id",
			Name: "space2",
			Subnets: []network.SubnetInfo{
				subnet,
			},
		},
	}

	newTopology, err := spaces.MoveSubnets(network.MakeIDSet(subnetID), "space1")
	c.Assert(err, tc.IsNil)

	s.st.EXPECT().
		GetAllSpaces(gomock.Any()).
		Return(spaces, nil)

	s.st.EXPECT().
		GetSubnets(gomock.Any(), []string{subnetUUID.String()}).
		Return(network.SubnetInfos{subnet}, nil)

	// Create a mock CheckableMachine that rejects the topology
	mockMachine := NewMockCheckableMachine(ctrl)
	mockMachine.EXPECT().
		Accept(gomock.Any(), newTopology).
		Return(errors.New("topology rejected: error1"))
	mockMachine.EXPECT().
		Accept(gomock.Any(), newTopology).
		Return(errors.New("topology rejected: error2"))

	s.st.EXPECT().
		GetMachinesBoundToSpaces(gomock.Any(), []string{"space2-id"}).
		Return(internal.CheckableMachines{mockMachine}, nil)

	s.st.EXPECT().
		GetMachinesNotAllowedInSpace(gomock.Any(), "space1-id").
		Return(internal.CheckableMachines{mockMachine}, nil)

	// Act
	result, err := s.service.MoveSubnetsToSpace(
		c.Context(),
		[]domainnetwork.SubnetUUID{subnetUUID},
		"space1",
		false,
	)

	// Assert
	c.Assert(err, tc.ErrorMatches, "topology rejected: error1\ntopology rejected: error2")
	c.Assert(result, tc.IsNil)
}

// TestMoveSubnetsToSpaceSuccessForcedWithRejectedTopology tests that subnets are
// successfully moved to the destination space, when forced even if the topology
// has been rejected.
func (s *moveSubnetsSuite) TestMoveSubnetsToSpaceSuccessForcedWithRejectedTopology(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Arrange
	subnetUUID1 := s.newSubnetUUID(c)
	subnetUUID2 := s.newSubnetUUID(c)
	subnetID1 := network.Id(subnetUUID1.String())
	subnetID2 := network.Id(subnetUUID2.String())
	subnets := []network.SubnetInfo{{
		ID:        subnetID1,
		CIDR:      "192.168.2.0/24",
		SpaceID:   "space2-id",
		SpaceName: "space2",
	}, {
		ID:        subnetID2,
		CIDR:      "192.192.2.0/24",
		SpaceID:   "space3-id",
		SpaceName: "space3",
	}}
	spaces := network.SpaceInfos{
		{
			ID:   "space1-id",
			Name: "space1",
		},
		{
			ID:      "space2-id",
			Name:    "space2",
			Subnets: subnets[0:1],
		},
		{
			ID:      "space3-id",
			Name:    "space3",
			Subnets: subnets[1:],
		},
	}

	newTopology, err := spaces.MoveSubnets(network.MakeIDSet(subnetID1, subnetID2), "space1")
	c.Assert(err, tc.IsNil)

	// Create a mock CheckableMachine that accept the topology
	boundMachines := NewMockCheckableMachine(ctrl)
	boundMachines.EXPECT().
		Accept(gomock.Any(), newTopology).
		Return(nil).
		Times(2)
	allergicMachines := NewMockCheckableMachine(ctrl)
	allergicMachines.EXPECT().
		Accept(gomock.Any(), newTopology).
		Return(errors.New("topology rejected: error1")).
		Times(1)

	s.st.EXPECT().
		GetAllSpaces(gomock.Any()).
		Return(spaces, nil)

	s.st.EXPECT().
		GetSubnets(gomock.Any(), []string{subnetUUID1.String(), subnetUUID2.String()}).
		Return(subnets, nil)

	s.st.EXPECT().
		GetMachinesBoundToSpaces(gomock.Any(), []string{"space2-id", "space3-id"}).
		Return(internal.CheckableMachines{boundMachines, boundMachines}, nil)

	s.st.EXPECT().
		GetMachinesNotAllowedInSpace(gomock.Any(), "space1-id").
		Return(internal.CheckableMachines{allergicMachines}, nil)

	// Expect UpdateSubnet to be called for the moved subnet
	upserted := transform.Slice(subnets, func(subnet network.SubnetInfo) network.SubnetInfo {
		subnet.SpaceID = "space1-id"
		subnet.SpaceName = "space1"
		return subnet
	})
	s.st.EXPECT().
		UpsertSubnets(gomock.Any(), upserted).
		Return(nil)

	// Act
	result, err := s.service.MoveSubnetsToSpace(
		c.Context(),
		[]domainnetwork.SubnetUUID{subnetUUID1, subnetUUID2},
		"space1",
		true,
	)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.SameContents, []domainnetwork.MovedSubnets{{
		UUID:      subnetUUID1,
		CIDR:      subnets[0].CIDR,
		FromSpace: "space2",
		ToSpace:   "space1",
	}, {
		UUID:      subnetUUID2,
		CIDR:      subnets[1].CIDR,
		FromSpace: "space3",
		ToSpace:   "space1",
	}})
}

// TestMoveSubnetsToSpaceSuccess tests that subnets are successfully moved to the
// destination space.
func (s *moveSubnetsSuite) TestMoveSubnetsToSpaceSuccess(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Arrange
	subnetUUID1 := s.newSubnetUUID(c)
	subnetUUID2 := s.newSubnetUUID(c)
	subnetID1 := network.Id(subnetUUID1.String())
	subnetID2 := network.Id(subnetUUID2.String())
	subnets := []network.SubnetInfo{{
		ID:        subnetID1,
		CIDR:      "192.168.2.0/24",
		SpaceID:   "space2-id",
		SpaceName: "space2",
	}, {
		ID:        subnetID2,
		CIDR:      "192.192.2.0/24",
		SpaceID:   "space3-id",
		SpaceName: "space3",
	}}
	spaces := network.SpaceInfos{
		{
			ID:   "space1-id",
			Name: "space1",
		},
		{
			ID:      "space2-id",
			Name:    "space2",
			Subnets: subnets[0:1],
		},
		{
			ID:      "space3-id",
			Name:    "space3",
			Subnets: subnets[1:],
		},
	}

	newTopology, err := spaces.MoveSubnets(network.MakeIDSet(subnetID1, subnetID2), "space1")
	c.Assert(err, tc.IsNil)

	// Create a mock CheckableMachine that accept the topology
	boundMachines := NewMockCheckableMachine(ctrl)
	boundMachines.EXPECT().
		Accept(gomock.Any(), newTopology).
		Return(nil).
		Times(2)
	allergicMachines := NewMockCheckableMachine(ctrl)
	allergicMachines.EXPECT().
		Accept(gomock.Any(), newTopology).
		Return(nil).
		Times(1)

	s.st.EXPECT().
		GetAllSpaces(gomock.Any()).
		Return(spaces, nil)

	s.st.EXPECT().
		GetSubnets(gomock.Any(), []string{subnetUUID1.String(), subnetUUID2.String()}).
		Return(subnets, nil)

	s.st.EXPECT().
		GetMachinesBoundToSpaces(gomock.Any(), []string{"space2-id", "space3-id"}).
		Return(internal.CheckableMachines{boundMachines, boundMachines}, nil)

	s.st.EXPECT().
		GetMachinesNotAllowedInSpace(gomock.Any(), "space1-id").
		Return(internal.CheckableMachines{allergicMachines}, nil)

	// Expect UpdateSubnet to be called for the moved subnet
	upserted := transform.Slice(subnets, func(subnet network.SubnetInfo) network.SubnetInfo {
		subnet.SpaceID = "space1-id"
		subnet.SpaceName = "space1"
		return subnet
	})
	s.st.EXPECT().
		UpsertSubnets(gomock.Any(), upserted).
		Return(nil)

	// Act
	result, err := s.service.MoveSubnetsToSpace(
		c.Context(),
		[]domainnetwork.SubnetUUID{subnetUUID1, subnetUUID2},
		"space1",
		false,
	)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.SameContents, []domainnetwork.MovedSubnets{{
		UUID:      subnetUUID1,
		CIDR:      subnets[0].CIDR,
		FromSpace: "space2",
		ToSpace:   "space1",
	}, {
		UUID:      subnetUUID2,
		CIDR:      subnets[1].CIDR,
		FromSpace: "space3",
		ToSpace:   "space1",
	}})
}

// TestMoveSubnetsToSpaceUpdateSubnetError tests that an error is returned when
// updating a subnet fails.
func (s *moveSubnetsSuite) TestMoveSubnetsToSpaceUpdateSubnetError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Arrange
	subnetUUID := s.newSubnetUUID(c)
	subnetID := network.Id(subnetUUID.String())
	subnet := network.SubnetInfo{
		ID:        subnetID,
		CIDR:      "192.168.2.0/24",
		SpaceID:   "space2-id",
		SpaceName: "space2",
	}
	spaces := network.SpaceInfos{
		{
			ID:   "space1-id",
			Name: "space1",
		},
		{
			ID:   "space2-id",
			Name: "space2",
			Subnets: []network.SubnetInfo{
				subnet,
			},
		},
	}
	s.st.EXPECT().
		GetAllSpaces(gomock.Any()).
		Return(spaces, nil)

	s.st.EXPECT().
		GetSubnets(gomock.Any(), []string{subnetUUID.String()}).
		Return(network.SubnetInfos{subnet}, nil)

	s.st.EXPECT().
		GetMachinesBoundToSpaces(gomock.Any(), []string{"space2-id"}).
		Return(internal.CheckableMachines{}, nil)

	s.st.EXPECT().
		GetMachinesNotAllowedInSpace(gomock.Any(), "space1-id").
		Return(internal.CheckableMachines{}, nil)

	// Expect UpsertSubnets to fail
	s.st.EXPECT().
		UpsertSubnets(gomock.Any(), gomock.Any()).
		Return(errors.New("boom"))

	// Act
	result, err := s.service.MoveSubnetsToSpace(
		c.Context(),
		[]domainnetwork.SubnetUUID{subnetUUID},
		"space1",
		false,
	)

	// Assert
	c.Assert(err, tc.ErrorMatches, "upserting subnets: boom")
	c.Assert(result, tc.IsNil)
}

// newSubnetUUID generates a new valid SubnetUUID and asserts that no error
// occurs during its creation.
func (s *moveSubnetsSuite) newSubnetUUID(c *tc.C) domainnetwork.SubnetUUID {
	subnetUUID, err := domainnetwork.NewSubnetUUID()
	c.Assert(err, tc.IsNil)
	return subnetUUID
}
