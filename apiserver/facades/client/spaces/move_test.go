// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces_test

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/facades/client/spaces"
	"github.com/juju/juju/core/network"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

type moveSubnetsAPISuite struct {
	spaces.APIBaseSuite
}

func TestMoveSubnetsAPISuite(t *testing.T) {
	tc.Run(t, &moveSubnetsAPISuite{})
}

// TestMoveSubnetsSpaceNotMutable ensures that the MoveSubnets function
// returns an error if ensureSpacesAreMutable fails.
func (s *moveSubnetsAPISuite) TestMoveSubnetsSpaceNotMutable(c *tc.C) {
	// Test that if ensureSpacesAreMutable fails, the MoveSubnets
	// function returns an error
	// Set providerSpaces to true to make ensureSpacesNotProviderSourced fail
	defer s.SetupMocks(c, false, true).Finish()

	// Arrange
	args := params.MoveSubnetsParams{
		Args: []params.MoveSubnetsParam{{
			SpaceTag:   "space-dmz",
			SubnetTags: []string{"subnet-10.0.0.0/24"},
		}},
	}

	// Act
	results, err := s.API.MoveSubnets(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorMatches, ".*modifying provider-sourced spaces.*")
	c.Assert(results.Results, tc.HasLen, 0)
}

// TestMoveSubnetsInvalidSpaceTag ensures that the MoveSubnets function returns
// an error when an invalid space tag is provided.
func (s *moveSubnetsAPISuite) TestMoveSubnetsInvalidSpaceTag(c *tc.C) {
	defer s.SetupMocks(c, false, false).Finish()

	// Arrange: invalid space tag
	args := params.MoveSubnetsParams{
		Args: []params.MoveSubnetsParam{{
			SpaceTag:   "invalid-space-tag", // Invalid space tag format
			SubnetTags: []string{"subnet-10.0.0.0/24"},
		}},
	}

	// Act
	results, err := s.API.MoveSubnets(c.Context(), args)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, `.*"invalid-space-tag" is not a valid tag.*`)
}

// TestMoveSubnetsNoSubnets validates that the MoveSubnets function
// returns an error when no subnets are specified.
func (s *moveSubnetsAPISuite) TestMoveSubnetsNoSubnets(c *tc.C) {
	defer s.SetupMocks(c, false, false).Finish()

	// Arrange: no subnets
	args := params.MoveSubnetsParams{
		Args: []params.MoveSubnetsParam{{
			SpaceTag:   "space-dmz",
			SubnetTags: []string{}, // Empty subnet tags
		}},
	}

	// Act
	results, err := s.API.MoveSubnets(c.Context(), args)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, ".*no subnets specified.*")
}

// TestMoveSubnetsInvalidSubnetTag validates that the MoveSubnets function
// correctly returns an error for invalid subnet tags.
func (s *moveSubnetsAPISuite) TestMoveSubnetsInvalidSubnetTag(c *tc.C) {
	defer s.SetupMocks(c, false, false).Finish()

	// Arrange: invalid subnet tag
	args := params.MoveSubnetsParams{
		Args: []params.MoveSubnetsParam{{
			SpaceTag:   "space-dmz",
			SubnetTags: []string{"invalid-subnet-tag"}, // Invalid subnet tag format
		}},
	}

	// Act
	results, err := s.API.MoveSubnets(c.Context(), args)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, `.*"invalid-subnet-tag" is not a valid tag.*`)
}

// TestMoveSubnetsServiceFailure validates that the MoveSubnets function
// correctly handles service failures by returning errors.
func (s *moveSubnetsAPISuite) TestMoveSubnetsGetAllSubnetsFailure(c *tc.C) {
	defer s.SetupMocks(c, false, false).Finish()

	// Arrange: Set up the NetworkService mock to return an error
	s.NetworkService.EXPECT().GetAllSubnets(gomock.Any()).Return(nil, errors.New("service failure"))

	args := params.MoveSubnetsParams{
		Args: []params.MoveSubnetsParam{{
			SpaceTag:   "space-dmz",
			SubnetTags: []string{names.NewSubnetTag(uuid.MustNewUUID().String()).String()},
		}},
	}

	// Act
	results, err := s.API.MoveSubnets(c.Context(), args)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, ".*service failure.*")
}

// TestMoveSubnetsServiceFailure validates that the MoveSubnets function
// correctly handles service failures by returning errors.
func (s *moveSubnetsAPISuite) TestMoveSubnetsServiceFailure(c *tc.C) {
	defer s.SetupMocks(c, false, false).Finish()

	// Arrange: Set up the NetworkService mock to return an error
	s.NetworkService.EXPECT().GetAllSubnets(gomock.Any()).Return(nil, nil)
	s.NetworkService.EXPECT().MoveSubnetsToSpace(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(nil, errors.New("service failure"))

	args := params.MoveSubnetsParams{
		Args: []params.MoveSubnetsParam{{
			SpaceTag:   "space-dmz",
			SubnetTags: []string{names.NewSubnetTag(uuid.MustNewUUID().String()).String()},
		}},
	}

	// Act
	results, err := s.API.MoveSubnets(c.Context(), args)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, ".*service failure.*")
}

// TestMoveSubnetsSuccessForced validates that the MoveSubnets function
// successfully moves subnets when forced mode is enabled.
func (s *moveSubnetsAPISuite) TestMoveSubnetsSuccessForced(c *tc.C) {
	defer s.SetupMocks(c, false, false).Finish()

	// Arrange: a subnet moved from default to dmz
	subnetUUID := domainnetwork.SubnetUUID(uuid.MustNewUUID().String())
	subnetTag := names.NewSubnetTag(subnetUUID.String()).String()
	spaceName := network.SpaceName("dmz")
	spaceTag := names.NewSpaceTag(spaceName.String()).String()
	movedSubnets := []domainnetwork.MovedSubnets{
		{
			UUID:      subnetUUID,
			FromSpace: network.SpaceName("default"),
		},
	}
	s.NetworkService.EXPECT().GetAllSubnets(gomock.Any()).Return(network.SubnetInfos{
		network.SubnetInfo{
			ID:   network.Id(subnetUUID),
			CIDR: "10.0.0.0/24",
		},
	}, nil)
	s.NetworkService.EXPECT().MoveSubnetsToSpace(
		gomock.Any(),
		[]domainnetwork.SubnetUUID{subnetUUID},
		spaceName,
		true, // forced
	).Return(movedSubnets, nil)

	args := params.MoveSubnetsParams{
		Args: []params.MoveSubnetsParam{{
			SpaceTag:   spaceTag,
			SubnetTags: []string{subnetTag},
			Force:      true,
		}},
	}

	// Act
	results, err := s.API.MoveSubnets(c.Context(), args)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
	c.Assert(results.Results[0].NewSpaceTag, tc.Equals, "space-dmz")
	c.Assert(results.Results[0].MovedSubnets, tc.SameContents, []params.MovedSubnet{
		{
			SubnetTag:   subnetTag,
			OldSpaceTag: names.NewSpaceTag("default").String(),
			CIDR:        "10.0.0.0/24",
		},
	})
}

// TestMoveSubnetsSuccessSeveralSubnets validates the successful movement of
// multiple subnets to a new space (not forced)
func (s *moveSubnetsAPISuite) TestMoveSubnetsSuccessSeveralSubnets(c *tc.C) {
	defer s.SetupMocks(c, false, false).Finish()

	// Arrange: two subnet, from either default-1 and default-2 spaces, moved
	// to dmz
	subnetUUID1 := domainnetwork.SubnetUUID(uuid.MustNewUUID().String())
	subnetUUID2 := domainnetwork.SubnetUUID(uuid.MustNewUUID().String())
	subnetTag1 := names.NewSubnetTag(subnetUUID1.String()).String()
	subnetTag2 := names.NewSubnetTag(subnetUUID2.String()).String()
	spaceName := network.SpaceName("dmz")
	spaceTag := names.NewSpaceTag(spaceName.String()).String()
	movedSubnets := []domainnetwork.MovedSubnets{
		{
			UUID:      subnetUUID1,
			FromSpace: network.SpaceName("default-1"),
		},
		{
			UUID:      subnetUUID2,
			FromSpace: network.SpaceName("default-2"),
		},
	}
	s.NetworkService.EXPECT().GetAllSubnets(gomock.Any()).Return(network.SubnetInfos{
		network.SubnetInfo{
			ID:   network.Id(subnetUUID1),
			CIDR: "10.0.0.0/24",
		},
		network.SubnetInfo{
			ID:   network.Id(subnetUUID2),
			CIDR: "198.0.0.0/24",
		},
	}, nil)
	s.NetworkService.EXPECT().MoveSubnetsToSpace(
		gomock.Any(),
		[]domainnetwork.SubnetUUID{subnetUUID1, subnetUUID2},
		spaceName,
		false, // not forced
	).Return(movedSubnets, nil)

	args := params.MoveSubnetsParams{
		Args: []params.MoveSubnetsParam{{
			SpaceTag:   spaceTag,
			SubnetTags: []string{subnetTag1, subnetTag2},
			Force:      false,
		}},
	}

	// Act
	results, err := s.API.MoveSubnets(c.Context(), args)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
	c.Assert(results.Results[0].NewSpaceTag, tc.Equals, "space-dmz")
	c.Assert(results.Results[0].MovedSubnets, tc.SameContents, []params.MovedSubnet{
		{
			SubnetTag:   subnetTag1,
			OldSpaceTag: names.NewSpaceTag("default-1").String(),
			CIDR:        "10.0.0.0/24",
		}, {
			SubnetTag:   subnetTag2,
			OldSpaceTag: names.NewSpaceTag("default-2").String(),
			CIDR:        "198.0.0.0/24",
		},
	})
}
