// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces_test

import (
	"fmt"
	"sort"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/facades/client/spaces"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/rpc/params"
)

// APIBaseSuite tests API calls using mocked model operations.
// TODO (manadart 2020-03-24): This should be broken up into separate
// suites for each command. See move_tests.go.
type APISuite struct {
	spaces.APIBaseSuite
}

func TestAPISuite(t *stdtesting.T) {
	tc.Run(t, &APISuite{})
}

func (s *APISuite) TestCreateSpacesFailInvalidTag(c *tc.C) {
	ctrl := s.SetupMocks(c, true, false)
	defer ctrl.Finish()

	args := params.CreateSpacesParams{
		Spaces: []params.CreateSpaceParams{
			{
				CIDRs:      []string{"10.0.0.0/24", "192.168.0.0/24"},
				SpaceTag:   "space0",
				ProviderId: "space-0",
			},
		},
	}

	expected := params.ErrorResults{
		Results: []params.ErrorResult{
			{
				Error: &params.Error{
					Message: "\"space0\" is not a valid tag",
				},
			},
		},
	}

	res, err := s.API.CreateSpaces(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, expected)
}

func (s *APISuite) TestCreateSpacesFailInvalidCIDR(c *tc.C) {
	ctrl := s.SetupMocks(c, true, false)
	defer ctrl.Finish()

	args := params.CreateSpacesParams{
		Spaces: []params.CreateSpaceParams{
			{
				CIDRs:      []string{"256.0.0.0/24"},
				SpaceTag:   "space-0",
				ProviderId: "space-0",
			},
		},
	}

	expected := params.ErrorResults{
		Results: []params.ErrorResult{
			{
				Error: &params.Error{
					Message: "\"256.0.0.0/24\" is not a valid CIDR",
				},
			},
		},
	}

	res, err := s.API.CreateSpaces(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, expected)
}

func (s *APISuite) TestCreateSpacesSuccess(c *tc.C) {
	ctrl := s.SetupMocks(c, true, false)
	defer ctrl.Finish()

	args := params.CreateSpacesParams{
		Spaces: []params.CreateSpaceParams{
			{
				CIDRs:      []string{"10.0.0.0/24", "192.168.0.0/24"},
				SpaceTag:   "space-0",
				ProviderId: "prov-space-0",
			},
			{
				CIDRs:      []string{"10.0.1.0/24", "192.168.1.0/24"},
				SpaceTag:   "space-1",
				ProviderId: "prov-space-1",
			},
		},
	}

	s.NetworkService.EXPECT().SubnetsByCIDR(gomock.Any(), []string{"10.0.0.0/24", "192.168.0.0/24"}).
		Return([]network.SubnetInfo{
			{
				ID: "subnet-1",
			},
			{
				ID: "subnet-2",
			},
		}, nil)
	s.NetworkService.EXPECT().SubnetsByCIDR(gomock.Any(), []string{"10.0.1.0/24", "192.168.1.0/24"}).
		Return([]network.SubnetInfo{
			{
				ID: "subnet-3",
			},
			{
				ID: "subnet-4",
			},
		}, nil)

	space0 := network.SpaceInfo{
		Name:       "0",
		ProviderId: network.Id("prov-space-0"),
		Subnets: []network.SubnetInfo{
			{
				ID: "subnet-1",
			},
			{
				ID: "subnet-2",
			},
		},
	}
	space1 := network.SpaceInfo{
		Name:       "1",
		ProviderId: network.Id("prov-space-1"),
		Subnets: []network.SubnetInfo{
			{
				ID: "subnet-3",
			},
			{
				ID: "subnet-4",
			},
		},
	}
	s.NetworkService.EXPECT().AddSpace(gomock.Any(), space0)
	s.NetworkService.EXPECT().AddSpace(gomock.Any(), space1)

	expected := params.ErrorResults{Results: []params.ErrorResult{{}, {}}}
	res, err := s.API.CreateSpaces(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, expected)
}

func (s *APISuite) TestShowSpaceDefault(c *tc.C) {
	ctrl := s.SetupMocks(c, true, false)
	defer ctrl.Finish()

	expectedApplications := []string{"mysql", "mediawiki"}
	sort.Strings(expectedApplications)

	s.expectDefaultSpace(ctrl, "default", nil)
	s.ApplicationService.EXPECT().GetApplicationsBoundToSpace(gomock.Any(), network.SpaceUUID("1")).Return(expectedApplications, nil)
	s.MachineService.EXPECT().CountMachinesInSpace(gomock.Any(), network.SpaceUUID("1")).Return(int64(2), nil)

	args := s.getShowSpaceArg("default")

	expected := params.ShowSpaceResults{Results: []params.ShowSpaceResult{
		{
			Space: params.Space{Id: "1", Name: "default", Subnets: []params.Subnet{{
				CIDR:              "192.168.0.0/24",
				ProviderId:        "0",
				ProviderNetworkId: "1",
				ProviderSpaceId:   "",
				VLANTag:           0,
				Life:              "alive",
				SpaceTag:          args.Entities[0].Tag,
				Zones:             []string{"bar", "bam"},
			}}},
			Error: nil,
			// Applications = 2, as 2 applications are having a bind on that space.
			Applications: expectedApplications,
			// MachineCount = 2, as two machines has constraints on the space.
			MachineCount: 2,
		},
	}}

	res, err := s.API.ShowSpace(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, expected)
}

func (s *APISuite) TestShowSpaceErrorGettingSpace(c *tc.C) {
	ctrl := s.SetupMocks(c, true, false)
	defer ctrl.Finish()

	bamErr := errors.New("bam")
	s.expectDefaultSpace(ctrl, "default", bamErr)
	args := s.getShowSpaceArg("default")

	res, err := s.API.ShowSpace(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	expectedErr := fmt.Sprintf("fetching space %q: %v", "default", bamErr.Error())
	c.Assert(res.Results[0].Error, tc.ErrorMatches, expectedErr)
}

func (s *APISuite) TestShowSpaceErrorGettingSubnets(c *tc.C) {
	ctrl := s.SetupMocks(c, true, false)
	defer ctrl.Finish()

	bamErr := errors.New("bam")
	s.expectDefaultSpace(ctrl, "default", bamErr)
	args := s.getShowSpaceArg("default")

	res, err := s.API.ShowSpace(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	expectedErr := fmt.Sprintf("fetching space \"default\": %v", bamErr.Error())
	c.Assert(res.Results[0].Error, tc.ErrorMatches, expectedErr)
}

func (s *APISuite) TestShowSpaceErrorGettingApplications(c *tc.C) {
	ctrl := s.SetupMocks(c, true, false)
	defer ctrl.Finish()

	expErr := errors.New("bam")
	s.expectDefaultSpace(ctrl, "default", nil)
	s.ApplicationService.EXPECT().GetApplicationsBoundToSpace(gomock.Any(), network.SpaceUUID("1")).Return(nil, expErr)

	args := s.getShowSpaceArg("default")

	res, err := s.API.ShowSpace(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	expectedErr := fmt.Sprintf("fetching applications: %v", expErr.Error())
	c.Assert(res.Results[0].Error, tc.ErrorMatches, expectedErr)
}

func (s *APISuite) TestShowSpaceErrorGettingMachines(c *tc.C) {
	ctrl := s.SetupMocks(c, true, false)
	defer ctrl.Finish()

	bamErr := errors.New("bam")
	s.expectDefaultSpace(ctrl, "default", nil)
	s.ApplicationService.EXPECT().GetApplicationsBoundToSpace(gomock.Any(), network.SpaceUUID("1")).Return([]string{}, nil)
	s.MachineService.EXPECT().CountMachinesInSpace(gomock.Any(), network.SpaceUUID("1")).Return(int64(0), bamErr)

	args := s.getShowSpaceArg("default")
	res, err := s.API.ShowSpace(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	expectedErr := fmt.Sprintf("fetching machine count: %v", bamErr.Error())
	c.Assert(res.Results[0].Error, tc.ErrorMatches, expectedErr)
}

func (s *APISuite) TestRenameSpaceErrorToAlreadyExist(c *tc.C) {
	ctrl := s.SetupMocks(c, true, false)
	defer ctrl.Finish()

	s.expectDefaultSpace(ctrl, "blub", nil)

	var from, to network.SpaceName = "bla", "blub"
	args := s.getRenameArgs(from, to)

	res, err := s.API.RenameSpace(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	expectedErr := fmt.Sprintf("space %q already exists", to)
	c.Assert(res.Results[0].Error, tc.ErrorMatches, expectedErr)
}

func (s *APISuite) TestRenameSpaceErrorUnexpectedError(c *tc.C) {
	ctrl := s.SetupMocks(c, true, false)
	defer ctrl.Finish()

	var from, to network.SpaceName = "bla", "blub"

	bamErr := errors.New("bam")
	s.expectDefaultSpace(ctrl, to, bamErr)

	args := s.getRenameArgs(from, to)

	res, err := s.API.RenameSpace(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	expectedErr := fmt.Sprintf("retrieving space %q: %v", to, bamErr.Error())
	c.Assert(res.Results[0].Error, tc.ErrorMatches, expectedErr)
}

func (s *APISuite) TestRenameSpaceErrorRename(c *tc.C) {
	ctrl := s.SetupMocks(c, true, false)
	defer ctrl.Finish()

	var from, to network.SpaceName = "bla", "blub"

	bamErr := errors.New("bam")
	s.expectDefaultSpace(ctrl, to, networkerrors.SpaceNotFound)
	args := s.getRenameArgs(from, to)
	s.NetworkService.EXPECT().SpaceByName(gomock.Any(), from).Return(
		&network.SpaceInfo{
			ID: "bla-space-id",
		},
		nil,
	)
	s.NetworkService.EXPECT().UpdateSpace(gomock.Any(), network.SpaceUUID("bla-space-id"), to).Return(bamErr)

	res, err := s.API.RenameSpace(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results[0].Error, tc.ErrorMatches, ".*"+bamErr.Error())
}

func (s *APISuite) TestRenameAlphaSpaceError(c *tc.C) {
	ctrl := s.SetupMocks(c, true, false)
	defer ctrl.Finish()

	var from, to network.SpaceName = network.AlphaSpaceName, "blub"

	args := s.getRenameArgs(from, to)

	res, err := s.API.RenameSpace(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results[0].Error, tc.ErrorMatches, `the "alpha" space cannot be renamed`)
}

func (s *APISuite) TestRenameSpaceSuccess(c *tc.C) {
	ctrl := s.SetupMocks(c, true, false)
	defer ctrl.Finish()

	var from, to network.SpaceName = "bla", "blub"

	s.expectDefaultSpace(ctrl, to, networkerrors.SpaceNotFound)
	args := s.getRenameArgs(from, to)

	fromSpace := &network.SpaceInfo{
		ID: "space-bla",
	}
	s.NetworkService.EXPECT().SpaceByName(gomock.Any(), from).Return(fromSpace, nil)
	s.NetworkService.EXPECT().UpdateSpace(gomock.Any(), fromSpace.ID, to).Return(nil)

	res, err := s.API.RenameSpace(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results[0].Error, tc.IsNil)
}

func (s *APISuite) TestRenameSpaceErrorProviderSpacesSupport(c *tc.C) {
	ctrl := s.SetupMocks(c, true, true)
	defer ctrl.Finish()

	var from, to network.SpaceName = "bla", "blub"

	args := s.getRenameArgs(from, to)

	res, err := s.API.RenameSpace(c.Context(), args)
	c.Assert(err, tc.ErrorMatches, "modifying provider-sourced spaces not supported")
	c.Assert(res, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult(nil)})
}

func (s *APISuite) getShowSpaceArg(name string) params.Entities {
	spaceTag := names.NewSpaceTag(name)
	args := params.Entities{
		Entities: []params.Entity{{Tag: spaceTag.String()}},
	}
	return args
}

// expectDefaultSpace configures a default space mock with default subnet settings
func (s *APISuite) expectDefaultSpace(ctrl *gomock.Controller, name network.SpaceName, spacesErr error) {
	backingSubnets := network.SubnetInfos{{
		ProviderId:        network.Id("0"),
		ProviderNetworkId: network.Id("1"),
		CIDR:              "192.168.0.0/24",
		VLANTag:           0,
		AvailabilityZones: []string{"bar", "bam"},
		SpaceName:         name,
		SpaceID:           "1",
		Life:              life.Value("alive"),
	}}

	backingSpaceInfo := &network.SpaceInfo{
		ID:      "1",
		Name:    name,
		Subnets: backingSubnets,
	}
	if spacesErr != nil {
		s.NetworkService.EXPECT().SpaceByName(gomock.Any(), name).Return(nil, spacesErr)
	} else {
		s.NetworkService.EXPECT().SpaceByName(gomock.Any(), name).Return(backingSpaceInfo, nil)
	}
}

func (s *APISuite) getRenameArgs(from, to network.SpaceName) params.RenameSpacesParams {
	spaceTagFrom := names.NewSpaceTag(from.String())
	spaceTagTo := names.NewSpaceTag(to.String())
	args := params.RenameSpacesParams{
		Changes: []params.RenameSpaceParams{{
			FromSpaceTag: spaceTagFrom.String(),
			ToSpaceTag:   spaceTagTo.String(),
		}},
	}
	return args
}

func (s *APISuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- Authoriser should not permit calls from agents.
- An error returned from the call to SupportsSpaces for each of List,Create.
- A NotSupported error from CreateSpaces.
- A successful call to ListSpaces.
- CreateSpaces error when the block checker is active.
`)
}
