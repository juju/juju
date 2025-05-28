// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces_test

import (
	"fmt"
	"sort"
	stdtesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/facades/client/spaces"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

// APISuite tests API calls using mocked model operations.
// TODO (manadart 2020-03-24): This should be broken up into separate
// suites for each command. See move_tests.go.
type APISuite struct {
	spaces.APISuite
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

	s.expectDefaultSpace(ctrl, "default", nil)
	s.expectEndpointBindings(ctrl, "1")
	s.expectMachines(ctrl, s.getDefaultSpaces(), nil, nil)

	s.NetworkService.EXPECT().GetAllSpaces(gomock.Any())
	s.NetworkService.EXPECT().GetAllSubnets(gomock.Any())

	expectedApplications := []string{"mysql", "mediawiki"}
	sort.Strings(expectedApplications)
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

	s.NetworkService.EXPECT().GetAllSpaces(gomock.Any())
	s.NetworkService.EXPECT().GetAllSubnets(gomock.Any())

	res, err := s.API.ShowSpace(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	expectedErr := fmt.Sprintf("fetching space %q: %v", args.Entities[0].Tag, bamErr.Error())
	c.Assert(res.Results[0].Error, tc.ErrorMatches, expectedErr)
}

func (s *APISuite) TestShowSpaceErrorGettingSubnets(c *tc.C) {
	ctrl := s.SetupMocks(c, true, false)
	defer ctrl.Finish()

	bamErr := errors.New("bam")
	s.expectDefaultSpace(ctrl, "default", bamErr)
	args := s.getShowSpaceArg("default")

	s.NetworkService.EXPECT().GetAllSpaces(gomock.Any())
	s.NetworkService.EXPECT().GetAllSubnets(gomock.Any())

	res, err := s.API.ShowSpace(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	expectedErr := fmt.Sprintf("fetching space \"space-default\": %v", bamErr.Error())
	c.Assert(res.Results[0].Error, tc.ErrorMatches, expectedErr)
}

func (s *APISuite) TestShowSpaceErrorGettingApplications(c *tc.C) {
	ctrl := s.SetupMocks(c, true, false)
	defer ctrl.Finish()

	expErr := errors.New("bam")
	s.expectDefaultSpace(ctrl, "default", nil)
	s.Backing.EXPECT().AllEndpointBindings().Return(nil, expErr)
	s.NetworkService.EXPECT().GetAllSpaces(gomock.Any())
	s.NetworkService.EXPECT().GetAllSubnets(gomock.Any())

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
	s.expectEndpointBindings(ctrl, "1")
	s.expectMachines(ctrl, s.getDefaultSpaces(), bamErr, nil)

	s.NetworkService.EXPECT().GetAllSpaces(gomock.Any())
	s.NetworkService.EXPECT().GetAllSubnets(gomock.Any())

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

	from, to := "bla", "blub"
	args := s.getRenameArgs(from, to)

	res, err := s.API.RenameSpace(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	expectedErr := fmt.Sprintf("space %q already exists", to)
	c.Assert(res.Results[0].Error, tc.ErrorMatches, expectedErr)
}

func (s *APISuite) TestRenameSpaceErrorUnexpectedError(c *tc.C) {
	ctrl := s.SetupMocks(c, true, false)
	defer ctrl.Finish()

	from, to := "bla", "blub"

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

	from, to := "bla", "blub"

	bamErr := errors.New("bam")
	s.expectDefaultSpace(ctrl, to, networkerrors.SpaceNotFound)
	args := s.getRenameArgs(from, to)
	s.NetworkService.EXPECT().SpaceByName(gomock.Any(), from).Return(
		&network.SpaceInfo{
			ID: "bla-space-id",
		},
		nil,
	)
	s.NetworkService.EXPECT().UpdateSpace(gomock.Any(), "bla-space-id", "blub").Return(bamErr)

	res, err := s.API.RenameSpace(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results[0].Error, tc.ErrorMatches, ".*"+bamErr.Error())
}

func (s *APISuite) TestRenameAlphaSpaceError(c *tc.C) {
	ctrl := s.SetupMocks(c, true, false)
	defer ctrl.Finish()

	from, to := "alpha", "blub"

	args := s.getRenameArgs(from, to)

	res, err := s.API.RenameSpace(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results[0].Error, tc.ErrorMatches, `the "alpha" space cannot be renamed`)
}

func (s *APISuite) TestRenameSpaceSuccess(c *tc.C) {
	ctrl := s.SetupMocks(c, true, false)
	defer ctrl.Finish()

	from, to := "bla", "blub"

	s.expectDefaultSpace(ctrl, to, networkerrors.SpaceNotFound)
	args := s.getRenameArgs(from, to)

	fromSpace := &network.SpaceInfo{
		ID: "space-bla",
	}
	s.NetworkService.EXPECT().SpaceByName(gomock.Any(), "bla").Return(fromSpace, nil)
	s.NetworkService.EXPECT().UpdateSpace(gomock.Any(), fromSpace.ID, "blub").Return(nil)

	res, err := s.API.RenameSpace(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results[0].Error, tc.IsNil)
}

func (s *APISuite) TestRenameSpaceErrorProviderSpacesSupport(c *tc.C) {
	ctrl := s.SetupMocks(c, true, true)
	defer ctrl.Finish()

	from, to := "bla", "blub"

	args := s.getRenameArgs(from, to)

	res, err := s.API.RenameSpace(c.Context(), args)
	c.Assert(err, tc.ErrorMatches, "modifying provider-sourced spaces not supported")
	c.Assert(res, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult(nil)})
}

func (s *APISuite) TestRemoveSpaceSuccessNoControllerConfig(c *tc.C) {
	ctrl := s.SetupMocks(c, true, false)
	defer ctrl.Finish()

	spaceTag := "myspace"
	args, tag := s.getRemoveArgs(spaceTag, false)

	s.expectDefaultSpace(ctrl, spaceTag, nil)
	s.expectEndpointBindings(ctrl, "2")
	s.Backing.EXPECT().ConstraintsBySpaceName(spaceTag).Return(nil, nil)
	s.Backing.EXPECT().IsController().Return(false)

	s.NetworkService.EXPECT().GetAllSpaces(gomock.Any())
	space := &network.SpaceInfo{
		ID:   "my-space-id",
		Name: "myspace",
	}
	s.NetworkService.EXPECT().SpaceByName(gomock.Any(), tag.Id()).Return(space, nil)
	s.NetworkService.EXPECT().RemoveSpace(gomock.Any(), space.ID)

	res, err := s.API.RemoveSpace(c.Context(), args)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.RemoveSpaceResults{Results: []params.RemoveSpaceResult{{}}})
}

func (s *APISuite) TestRemoveSpaceSuccessControllerConfig(c *tc.C) {
	ctrl := s.SetupMocks(c, true, false)
	defer ctrl.Finish()

	spaceTag := "myspace"
	args, tag := s.getRemoveArgs(spaceTag, false)

	s.expectDefaultSpace(ctrl, spaceTag, nil)
	s.expectEndpointBindings(ctrl, "2")

	s.ControllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(nil, nil)

	s.Backing.EXPECT().IsController().Return(true)
	s.Backing.EXPECT().ConstraintsBySpaceName(spaceTag).Return(nil, nil)

	s.NetworkService.EXPECT().GetAllSpaces(gomock.Any())
	space := &network.SpaceInfo{
		ID:   "my-space-id",
		Name: "myspace",
	}
	s.NetworkService.EXPECT().SpaceByName(gomock.Any(), tag.Id()).Return(space, nil)
	s.NetworkService.EXPECT().RemoveSpace(gomock.Any(), space.ID)

	res, err := s.API.RemoveSpace(c.Context(), args)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.RemoveSpaceResults{Results: []params.RemoveSpaceResult{{}}})
}

func (s *APISuite) TestRemoveSpaceErrorFoundApplications(c *tc.C) {
	ctrl := s.SetupMocks(c, true, false)
	defer ctrl.Finish()

	space := "myspace"
	args, _ := s.getRemoveArgs(space, false)

	s.expectDefaultSpace(ctrl, space, nil)
	s.expectEndpointBindings(ctrl, "1")
	s.Backing.EXPECT().IsController().Return(false)
	s.Backing.EXPECT().ConstraintsBySpaceName(space).Return(nil, nil)
	expected := params.RemoveSpaceResults{Results: []params.RemoveSpaceResult{{
		Bindings: []params.Entity{
			{
				Tag: names.NewApplicationTag("mediawiki").String(),
			},
			{
				Tag: names.NewApplicationTag("mysql").String(),
			},
		},
		Constraints:        nil,
		ControllerSettings: nil,
		Error:              nil,
	}}}
	s.NetworkService.EXPECT().GetAllSpaces(gomock.Any())

	res, err := s.API.RemoveSpace(c.Context(), args)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, expected)
}

func (s *APISuite) TestRemoveSpaceErrorFoundController(c *tc.C) {
	ctrl := s.SetupMocks(c, true, false)
	defer ctrl.Finish()

	spaceName := "myspace"
	args, _ := s.getRemoveArgs(spaceName, false)

	s.expectDefaultSpace(ctrl, spaceName, nil)
	s.expectEndpointBindings(ctrl, "2")
	s.Backing.EXPECT().IsController().Return(true)

	currentConfig := s.getDefaultControllerConfig(c, map[string]interface{}{controller.JujuHASpace: "nothing", controller.JujuManagementSpace: spaceName})
	s.ControllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(currentConfig, nil)
	s.Backing.EXPECT().ConstraintsBySpaceName(spaceName).Return(nil, nil)
	expected := params.RemoveSpaceResults{Results: []params.RemoveSpaceResult{{
		Bindings:           nil,
		Constraints:        nil,
		ControllerSettings: []string{controller.JujuManagementSpace},
		Error:              nil,
	}}}

	s.NetworkService.EXPECT().GetAllSpaces(gomock.Any())

	res, err := s.API.RemoveSpace(c.Context(), args)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, expected)
}

func (s *APISuite) TestRemoveSpaceErrorFoundConstraints(c *tc.C) {
	ctrl := s.SetupMocks(c, true, false)
	defer ctrl.Finish()

	space := "myspace"
	args, _ := s.getRemoveArgs(space, false)

	s.expectDefaultSpace(ctrl, space, nil)
	s.expectEndpointBindings(ctrl, "2")
	s.Backing.EXPECT().IsController().Return(false)

	cApp, cModel := s.expectAllTags(space)

	expected := params.RemoveSpaceResults{Results: []params.RemoveSpaceResult{{
		Bindings: nil,
		Constraints: []params.Entity{
			{
				Tag: cApp.String(),
			},
			{
				Tag: cModel.String(),
			},
		},
		ControllerSettings: nil,
		Error:              nil,
	}}}

	s.NetworkService.EXPECT().GetAllSpaces(gomock.Any())

	res, err := s.API.RemoveSpace(c.Context(), args)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results[0].Constraints, tc.SameContents, expected.Results[0].Constraints)
	c.Assert(res.Results[0].Bindings, tc.IsNil)
	c.Assert(res.Results[0].ControllerSettings, tc.IsNil)
	c.Assert(res.Results[0].Error, tc.IsNil)
}

func (s *APISuite) TestRemoveSpaceErrorFoundAll(c *tc.C) {
	ctrl := s.SetupMocks(c, true, false)
	defer ctrl.Finish()

	space := "myspace"
	args, _ := s.getRemoveArgs(space, false)

	s.expectDefaultSpace(ctrl, space, nil)
	s.expectEndpointBindings(ctrl, "1")
	s.Backing.EXPECT().IsController().Return(true)

	currentConfig := s.getDefaultControllerConfig(c, map[string]interface{}{controller.JujuHASpace: "nothing", controller.JujuManagementSpace: space})
	s.ControllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(currentConfig, nil)

	cApp, cModel := s.expectAllTags(space)

	expected := params.RemoveSpaceResults{Results: []params.RemoveSpaceResult{{
		Bindings: []params.Entity{
			{
				Tag: names.NewApplicationTag("mediawiki").String(),
			},
			{
				Tag: names.NewApplicationTag("mysql").String(),
			},
		},
		Constraints: []params.Entity{
			{
				Tag: cApp.String(),
			},
			{
				Tag: cModel.String(),
			},
		},
		ControllerSettings: []string{controller.JujuManagementSpace},
		Error:              nil,
	}}}
	s.NetworkService.EXPECT().GetAllSpaces(gomock.Any())

	res, err := s.API.RemoveSpace(c.Context(), args)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results[0].Constraints, tc.SameContents, expected.Results[0].Constraints)
	c.Assert(res.Results[0].Bindings, tc.SameContents, expected.Results[0].Bindings)
	c.Assert(res.Results[0].ControllerSettings, tc.SameContents, expected.Results[0].ControllerSettings)
	c.Assert(res.Results[0].Error, tc.IsNil)
}

func (s *APISuite) TestRemoveSpaceFoundAllWithForce(c *tc.C) {
	ctrl := s.SetupMocks(c, true, false)
	defer ctrl.Finish()

	spaceTag := "myspace"
	args, tag := s.getRemoveArgs(spaceTag, true)

	s.expectDefaultSpace(ctrl, spaceTag, nil)
	s.expectEndpointBindings(ctrl, "1")
	s.Backing.EXPECT().IsController().Return(true)

	currentConfig := s.getDefaultControllerConfig(c, map[string]interface{}{controller.JujuHASpace: "nothing", controller.JujuManagementSpace: spaceTag})
	s.ControllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(currentConfig, nil)

	s.NetworkService.EXPECT().GetAllSpaces(gomock.Any())
	space := &network.SpaceInfo{
		ID:   "my-space-id",
		Name: "myspace",
	}
	s.NetworkService.EXPECT().SpaceByName(gomock.Any(), tag.Id()).Return(space, nil)
	s.NetworkService.EXPECT().RemoveSpace(gomock.Any(), space.ID)

	_, _ = s.expectAllTags(spaceTag)

	expected := params.RemoveSpaceResults{Results: []params.RemoveSpaceResult{{}}}

	res, err := s.API.RemoveSpace(c.Context(), args)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, expected)
}

func (s *APISuite) TestRemoveSpaceErrorProviderSpacesSupport(c *tc.C) {
	ctrl := s.SetupMocks(c, true, true)
	defer ctrl.Finish()

	space := "myspace"

	args, _ := s.getRemoveArgs(space, false)

	_, err := s.API.RemoveSpace(c.Context(), args)
	c.Assert(err, tc.ErrorMatches, "modifying provider-sourced spaces not supported")
}

func (s *APISuite) expectAllTags(spaceName string) (names.ApplicationTag, names.ModelTag) {
	model := "42c4f770-86ed-4fcc-8e39-697063d082bc:e"
	machine := "42c4f770-86ed-4fcc-8e39-697063d082bc:m#0"
	application := "c9741ea1-0c2a-444d-82f5-787583a48557:a#mysql"
	unit := "c9741ea1-0c2a-444d-82f5-787583a48557:u#mysql/0"
	s.Constraints.EXPECT().ID().Return(model)
	s.Constraints.EXPECT().ID().Return(machine)
	s.Constraints.EXPECT().ID().Return(application)
	s.Constraints.EXPECT().ID().Return(unit)

	s.Backing.EXPECT().ConstraintsBySpaceName(spaceName).Return(
		[]spaces.Constraints{s.Constraints, s.Constraints, s.Constraints, s.Constraints}, nil)
	return names.NewApplicationTag("mysql"), names.NewModelTag(model)
}

func (s *APISuite) getDefaultControllerConfig(c *tc.C, attr map[string]interface{}) controller.Config {
	cfg, err := controller.NewConfig(testing.ControllerTag.Id(), testing.CACert, attr)
	c.Assert(err, tc.ErrorIsNil)
	return cfg
}

func (s *APISuite) getShowSpaceArg(name string) params.Entities {
	spaceTag := names.NewSpaceTag(name)
	args := params.Entities{
		Entities: []params.Entity{{Tag: spaceTag.String()}},
	}
	return args
}

func (s *APISuite) getDefaultSpaces() set.Strings {
	strings := set.NewStrings("1", "2")
	return strings
}

func (s *APISuite) expectEndpointBindings(ctrl *gomock.Controller, spaceID string) {
	b1 := spaces.NewMockBindings(ctrl)
	b1.EXPECT().Map().Return(map[string]string{
		"db":    spaceID,
		"slave": network.AlphaSpaceName,
	})

	b2 := spaces.NewMockBindings(ctrl)
	b2.EXPECT().Map().Return(map[string]string{
		"db":   spaceID,
		"back": network.AlphaSpaceName,
	})

	s.Backing.EXPECT().AllEndpointBindings().Return(map[string]spaces.Bindings{
		"mysql":     b1,
		"mediawiki": b2,
	}, nil)
}

// expectDefaultSpace configures a default space mock with default subnet settings
func (s *APISuite) expectDefaultSpace(ctrl *gomock.Controller, name string, spacesErr error) {
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
		Name:    network.SpaceName(name),
		Subnets: backingSubnets,
	}
	if spacesErr != nil {
		s.NetworkService.EXPECT().SpaceByName(gomock.Any(), name).Return(nil, spacesErr)
	} else {
		s.NetworkService.EXPECT().SpaceByName(gomock.Any(), name).Return(backingSpaceInfo, nil)
	}
}

func (s *APISuite) expectMachines(ctrl *gomock.Controller, addresses set.Strings, machErr, addressesErr error) {
	mockMachine := spaces.NewMockMachine(ctrl)
	// With this we can ensure that the function correctly adds up multiple machines.
	anotherMockMachine := spaces.NewMockMachine(ctrl)
	if machErr != nil {
		mockMachine.EXPECT().AllSpaces(gomock.Any()).Return(addresses, addressesErr).AnyTimes()
		anotherMockMachine.EXPECT().AllSpaces(gomock.Any()).Return(addresses, addressesErr).AnyTimes()
	} else {
		mockMachine.EXPECT().AllSpaces(gomock.Any()).Return(addresses, addressesErr)
		anotherMockMachine.EXPECT().AllSpaces(gomock.Any()).Return(addresses, addressesErr)
	}
	mockMachines := []spaces.Machine{mockMachine, anotherMockMachine}
	s.Backing.EXPECT().AllMachines().Return(mockMachines, machErr)
}

func (s *APISuite) getRenameArgs(from, to string) params.RenameSpacesParams {
	spaceTagFrom := names.NewSpaceTag(from)
	spaceTagTo := names.NewSpaceTag(to)
	args := params.RenameSpacesParams{
		Changes: []params.RenameSpaceParams{{
			FromSpaceTag: spaceTagFrom.String(),
			ToSpaceTag:   spaceTagTo.String(),
		}},
	}
	return args
}

func (s *APISuite) getRemoveArgs(name string, force bool) (params.RemoveSpaceParams, names.SpaceTag) {
	spaceTag := names.NewSpaceTag(name)
	args := params.RemoveSpaceParams{SpaceParams: []params.RemoveSpaceParam{
		{
			Space: params.Entity{Tag: spaceTag.String()},
			Force: force,
		},
	},
	}
	return args, spaceTag
}

func (s *APISuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- Authoriser should not permit calls from agents.
- An error returned from the call to SupportsSpaces for each of List,Create.
- A NotSupported error from CreateSpaces.
- A successful call to ListSpaces.
- CreateSpaces error when the block checker is active.
  ".
`)
}
