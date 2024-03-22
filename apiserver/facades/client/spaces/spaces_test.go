// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces_test

import (
	stdcontext "context"
	"fmt"
	"sort"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/client/spaces"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statemocks "github.com/juju/juju/state/mocks"
	"github.com/juju/juju/testing"
)

// APISuite tests API calls using mocked model operations.
// TODO (manadart 2020-03-24): This should be broken up into separate
// suites for each command. See move_tests.go.
type APISuite struct {
	spaces.APISuite

	renameSpaceOp *statemocks.MockModelOperation
}

var _ = gc.Suite(&APISuite{})

func (s *APISuite) TestCreateSpacesFailInvalidTag(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()

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

	res, err := s.API.CreateSpaces(stdcontext.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, jc.DeepEquals, expected)
}

func (s *APISuite) TestCreateSpacesFailInvalidCIDR(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()

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

	res, err := s.API.CreateSpaces(stdcontext.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, jc.DeepEquals, expected)
}

func (s *APISuite) TestCreateSpacesSuccess(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()

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

	s.SubnetService.EXPECT().SubnetsByCIDR(gomock.Any(), []string{"10.0.0.0/24", "192.168.0.0/24"}).
		Return([]network.SubnetInfo{
			{
				ID: "subnet-1",
			},
			{
				ID: "subnet-2",
			},
		}, nil)
	s.SubnetService.EXPECT().SubnetsByCIDR(gomock.Any(), []string{"10.0.1.0/24", "192.168.1.0/24"}).
		Return([]network.SubnetInfo{
			{
				ID: "subnet-3",
			},
			{
				ID: "subnet-4",
			},
		}, nil)
	s.SpaceService.EXPECT().AddSpace(gomock.Any(), "0", network.Id("prov-space-0"), []string{"subnet-1", "subnet-2"})
	s.SpaceService.EXPECT().AddSpace(gomock.Any(), "1", network.Id("prov-space-1"), []string{"subnet-3", "subnet-4"})

	expected := params.ErrorResults{Results: []params.ErrorResult{{}, {}}}
	res, err := s.API.CreateSpaces(stdcontext.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, jc.DeepEquals, expected)
}

func (s *APISuite) TestShowSpaceDefault(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()

	s.expectDefaultSpace(ctrl, "default", nil)
	s.expectEndpointBindings(ctrl, "1")
	s.expectMachines(ctrl, s.getDefaultSpaces(), nil, nil)

	s.SpaceService.EXPECT().GetAllSpaces(gomock.Any())

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

	res, err := s.API.ShowSpace(stdcontext.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, jc.DeepEquals, expected)
}

func (s *APISuite) TestShowSpaceErrorGettingSpace(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()

	bamErr := errors.New("bam")
	s.expectDefaultSpace(ctrl, "default", bamErr)
	args := s.getShowSpaceArg("default")

	s.SpaceService.EXPECT().GetAllSpaces(gomock.Any())

	res, err := s.API.ShowSpace(stdcontext.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := fmt.Sprintf("fetching space %q: %v", args.Entities[0].Tag, bamErr.Error())
	c.Assert(res.Results[0].Error, gc.ErrorMatches, expectedErr)
}

func (s *APISuite) TestShowSpaceErrorGettingSubnets(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()

	bamErr := errors.New("bam")
	s.expectDefaultSpace(ctrl, "default", bamErr)
	args := s.getShowSpaceArg("default")

	s.SpaceService.EXPECT().GetAllSpaces(gomock.Any())

	res, err := s.API.ShowSpace(stdcontext.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := fmt.Sprintf("fetching space \"space-default\": %v", bamErr.Error())
	c.Assert(res.Results[0].Error, gc.ErrorMatches, expectedErr)
}

func (s *APISuite) TestShowSpaceErrorGettingApplications(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()

	expErr := errors.New("bam")
	s.expectDefaultSpace(ctrl, "default", nil)
	s.Backing.EXPECT().AllEndpointBindings(nil).Return(nil, expErr)
	s.SpaceService.EXPECT().GetAllSpaces(gomock.Any())

	args := s.getShowSpaceArg("default")

	res, err := s.API.ShowSpace(stdcontext.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := fmt.Sprintf("fetching applications: %v", expErr.Error())
	c.Assert(res.Results[0].Error, gc.ErrorMatches, expectedErr)
}

func (s *APISuite) TestShowSpaceErrorGettingMachines(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()

	bamErr := errors.New("bam")
	s.expectDefaultSpace(ctrl, "default", nil)
	s.expectEndpointBindings(ctrl, "1")
	s.expectMachines(ctrl, s.getDefaultSpaces(), bamErr, nil)

	s.SpaceService.EXPECT().GetAllSpaces(gomock.Any())

	args := s.getShowSpaceArg("default")
	res, err := s.API.ShowSpace(stdcontext.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := fmt.Sprintf("fetching machine count: %v", bamErr.Error())
	c.Assert(res.Results[0].Error, gc.ErrorMatches, expectedErr)
}

func (s *APISuite) TestRenameSpaceErrorToAlreadyExist(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()

	s.expectDefaultSpace(ctrl, "blub", nil)

	from, to := "bla", "blub"
	args := s.getRenameArgs(from, to)

	res, err := s.API.RenameSpace(stdcontext.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := fmt.Sprintf("space %q already exists", to)
	c.Assert(res.Results[0].Error, gc.ErrorMatches, expectedErr)
}

func (s *APISuite) TestRenameSpaceErrorUnexpectedError(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()
	from, to := "bla", "blub"

	bamErr := errors.New("bam")
	s.expectDefaultSpace(ctrl, to, bamErr)

	args := s.getRenameArgs(from, to)

	res, err := s.API.RenameSpace(stdcontext.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := fmt.Sprintf("retrieving space %q: %v", to, bamErr.Error())
	c.Assert(res.Results[0].Error, gc.ErrorMatches, expectedErr)
}

func (s *APISuite) TestRenameSpaceErrorRename(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()
	from, to := "bla", "blub"

	bamErr := errors.New("bam")
	s.expectDefaultSpace(ctrl, to, errors.NotFoundf(""))
	args := s.getRenameArgs(from, to)

	fromSpace := &network.SpaceInfo{
		ID: "space-bla",
	}
	s.SpaceService.EXPECT().SpaceByName(gomock.Any(), "bla").Return(fromSpace, nil)
	s.SpaceService.EXPECT().UpdateSpace(gomock.Any(), fromSpace.ID, "blub").Return(nil)

	s.OpFactory.EXPECT().NewRenameSpaceOp(from, to).Return(nil, bamErr)

	res, err := s.API.RenameSpace(stdcontext.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results[0].Error, gc.ErrorMatches, bamErr.Error())
}

func (s *APISuite) TestRenameAlphaSpaceError(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()
	from, to := "alpha", "blub"

	args := s.getRenameArgs(from, to)

	res, err := s.API.RenameSpace(stdcontext.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results[0].Error, gc.ErrorMatches, `the "alpha" space cannot be renamed`)
}

func (s *APISuite) TestRenameSpaceSuccess(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()
	from, to := "bla", "blub"

	s.OpFactory.EXPECT().NewRenameSpaceOp(from, to).Return(s.renameSpaceOp, nil)
	s.expectDefaultSpace(ctrl, to, errors.NotFoundf("abc"))
	s.Backing.EXPECT().ApplyOperation(s.renameSpaceOp).Return(nil)
	args := s.getRenameArgs(from, to)

	fromSpace := &network.SpaceInfo{
		ID: "space-bla",
	}
	s.SpaceService.EXPECT().SpaceByName(gomock.Any(), "bla").Return(fromSpace, nil)
	s.SpaceService.EXPECT().UpdateSpace(gomock.Any(), fromSpace.ID, "blub").Return(nil)

	res, err := s.API.RenameSpace(stdcontext.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results[0].Error, gc.IsNil)
}

func (s *APISuite) TestRenameSpaceErrorProviderSpacesSupport(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, true)
	defer ctrl.Finish()
	defer unreg()
	from, to := "bla", "blub"

	args := s.getRenameArgs(from, to)

	res, err := s.API.RenameSpace(stdcontext.Background(), args)
	c.Assert(err, gc.ErrorMatches, "modifying provider-sourced spaces not supported")
	c.Assert(res, gc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult(nil)})
}

func (s *APISuite) TestRemoveSpaceSuccessNoControllerConfig(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()
	spaceTag := "myspace"
	args, tag := s.getRemoveArgs(spaceTag, false)

	s.expectDefaultSpace(ctrl, spaceTag, nil)
	s.expectEndpointBindings(ctrl, "2")
	s.Backing.EXPECT().ConstraintsBySpaceName(spaceTag).Return(nil, nil)
	s.Backing.EXPECT().IsController().Return(false)

	s.SpaceService.EXPECT().GetAllSpaces(gomock.Any())
	space := &network.SpaceInfo{
		ID:   "my-space-id",
		Name: "myspace",
	}
	s.SpaceService.EXPECT().SpaceByName(gomock.Any(), tag.Id()).Return(space, nil)
	s.SpaceService.EXPECT().Remove(gomock.Any(), space.ID)

	res, err := s.API.RemoveSpace(stdcontext.Background(), args)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.RemoveSpaceResults{Results: []params.RemoveSpaceResult{{}}})
}

func (s *APISuite) TestRemoveSpaceSuccessControllerConfig(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()
	spaceTag := "myspace"
	args, tag := s.getRemoveArgs(spaceTag, false)

	s.expectDefaultSpace(ctrl, spaceTag, nil)
	s.expectEndpointBindings(ctrl, "2")
	s.Backing.EXPECT().ConstraintsBySpaceName(spaceTag).Return(nil, nil)
	s.Backing.EXPECT().IsController().Return(true)
	s.Backing.EXPECT().ControllerConfig().Return(nil, nil)

	s.SpaceService.EXPECT().GetAllSpaces(gomock.Any())
	space := &network.SpaceInfo{
		ID:   "my-space-id",
		Name: "myspace",
	}
	s.SpaceService.EXPECT().SpaceByName(gomock.Any(), tag.Id()).Return(space, nil)
	s.SpaceService.EXPECT().Remove(gomock.Any(), space.ID)

	res, err := s.API.RemoveSpace(stdcontext.Background(), args)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.RemoveSpaceResults{Results: []params.RemoveSpaceResult{{}}})
}

func (s *APISuite) TestRemoveSpaceErrorFoundApplications(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()
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
	s.SpaceService.EXPECT().GetAllSpaces(gomock.Any())

	res, err := s.API.RemoveSpace(stdcontext.Background(), args)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, expected)
}

func (s *APISuite) TestRemoveSpaceErrorFoundController(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()
	spaceName := "myspace"
	args, _ := s.getRemoveArgs(spaceName, false)

	s.expectDefaultSpace(ctrl, spaceName, nil)
	s.expectEndpointBindings(ctrl, "2")
	s.Backing.EXPECT().IsController().Return(true)

	currentConfig := s.getDefaultControllerConfig(c, map[string]interface{}{controller.JujuHASpace: "nothing", controller.JujuManagementSpace: spaceName})
	s.Backing.EXPECT().ControllerConfig().Return(currentConfig, nil)
	s.Backing.EXPECT().ConstraintsBySpaceName(spaceName).Return(nil, nil)
	expected := params.RemoveSpaceResults{Results: []params.RemoveSpaceResult{{
		Bindings:           nil,
		Constraints:        nil,
		ControllerSettings: []string{controller.JujuManagementSpace},
		Error:              nil,
	}}}

	s.SpaceService.EXPECT().GetAllSpaces(gomock.Any())

	res, err := s.API.RemoveSpace(stdcontext.Background(), args)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, expected)
}

func (s *APISuite) TestRemoveSpaceErrorFoundConstraints(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()
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

	s.SpaceService.EXPECT().GetAllSpaces(gomock.Any())

	res, err := s.API.RemoveSpace(stdcontext.Background(), args)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results[0].Constraints, jc.SameContents, expected.Results[0].Constraints)
	c.Assert(res.Results[0].Bindings, gc.IsNil)
	c.Assert(res.Results[0].ControllerSettings, gc.IsNil)
	c.Assert(res.Results[0].Error, gc.IsNil)
}

func (s *APISuite) TestRemoveSpaceErrorFoundAll(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()
	space := "myspace"
	args, _ := s.getRemoveArgs(space, false)

	s.expectDefaultSpace(ctrl, space, nil)
	s.expectEndpointBindings(ctrl, "1")
	s.Backing.EXPECT().IsController().Return(true)

	currentConfig := s.getDefaultControllerConfig(c, map[string]interface{}{controller.JujuHASpace: "nothing", controller.JujuManagementSpace: space})
	s.Backing.EXPECT().ControllerConfig().Return(currentConfig, nil)

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
	s.SpaceService.EXPECT().GetAllSpaces(gomock.Any())

	res, err := s.API.RemoveSpace(stdcontext.Background(), args)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results[0].Constraints, jc.SameContents, expected.Results[0].Constraints)
	c.Assert(res.Results[0].Bindings, jc.SameContents, expected.Results[0].Bindings)
	c.Assert(res.Results[0].ControllerSettings, jc.SameContents, expected.Results[0].ControllerSettings)
	c.Assert(res.Results[0].Error, gc.IsNil)
}

func (s *APISuite) TestRemoveSpaceFoundAllWithForce(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()
	spaceTag := "myspace"
	args, tag := s.getRemoveArgs(spaceTag, true)

	s.expectDefaultSpace(ctrl, spaceTag, nil)
	s.expectEndpointBindings(ctrl, "1")
	s.Backing.EXPECT().IsController().Return(true)

	currentConfig := s.getDefaultControllerConfig(c, map[string]interface{}{controller.JujuHASpace: "nothing", controller.JujuManagementSpace: spaceTag})
	s.Backing.EXPECT().ControllerConfig().Return(currentConfig, nil)

	s.SpaceService.EXPECT().GetAllSpaces(gomock.Any())
	space := &network.SpaceInfo{
		ID:   "my-space-id",
		Name: "myspace",
	}
	s.SpaceService.EXPECT().SpaceByName(gomock.Any(), tag.Id()).Return(space, nil)
	s.SpaceService.EXPECT().Remove(gomock.Any(), space.ID)

	_, _ = s.expectAllTags(spaceTag)

	expected := params.RemoveSpaceResults{Results: []params.RemoveSpaceResult{{}}}

	res, err := s.API.RemoveSpace(stdcontext.Background(), args)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, expected)
}

func (s *APISuite) TestRemoveSpaceErrorProviderSpacesSupport(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, true)
	defer ctrl.Finish()
	defer unreg()
	space := "myspace"

	args, _ := s.getRemoveArgs(space, false)

	_, err := s.API.RemoveSpace(stdcontext.Background(), args)
	c.Assert(err, gc.ErrorMatches, "modifying provider-sourced spaces not supported")
}

func (s *APISuite) setupMocks(c *gc.C, supportSpaces bool, providerSpaces bool) (*gomock.Controller, func()) {
	ctrl, unReg := s.APISuite.SetupMocks(c, supportSpaces, providerSpaces)

	s.renameSpaceOp = statemocks.NewMockModelOperation(ctrl)

	return ctrl, unReg
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

func (s *APISuite) getDefaultControllerConfig(c *gc.C, attr map[string]interface{}) controller.Config {
	cfg, err := controller.NewConfig(testing.ControllerTag.Id(), testing.CACert, attr)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

func (s *APISuite) getShowSpaceArg(name string) params.Entities {
	spaceTag := names.NewSpaceTag(name)
	args := params.Entities{
		Entities: []params.Entity{{spaceTag.String()}},
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

	s.Backing.EXPECT().AllEndpointBindings(gomock.Any()).Return(map[string]spaces.Bindings{
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
		s.SpaceService.EXPECT().SpaceByName(gomock.Any(), name).Return(nil, spacesErr)
	} else {
		s.SpaceService.EXPECT().SpaceByName(gomock.Any(), name).Return(backingSpaceInfo, nil)
	}
}

func (s *APISuite) expectMachines(ctrl *gomock.Controller, addresses set.Strings, machErr, addressesErr error) {
	mockMachine := spaces.NewMockMachine(ctrl)
	// With this we can ensure that the function correctly adds up multiple machines.
	anotherMockMachine := spaces.NewMockMachine(ctrl)
	if machErr != nil {
		mockMachine.EXPECT().AllSpaces().Return(addresses, addressesErr).AnyTimes()
		anotherMockMachine.EXPECT().AllSpaces().Return(addresses, addressesErr).AnyTimes()
	} else {
		mockMachine.EXPECT().AllSpaces().Return(addresses, addressesErr)
		anotherMockMachine.EXPECT().AllSpaces().Return(addresses, addressesErr)
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

type stubBacking struct {
	*apiservertesting.StubBacking
}

func (sb *stubBacking) IsController() bool {
	panic("should not be called")
}

func (sb *stubBacking) ConstraintsBySpaceName(_ string) ([]spaces.Constraints, error) {
	panic("should not be called")
}

func (sb *stubBacking) ApplyOperation(state.ModelOperation) error {
	panic("should not be called")
}

func (sb *stubBacking) ControllerConfig() (controller.Config, error) {
	panic("should not be called")
}

func (sb *stubBacking) AllEndpointBindings(network.SpaceInfos) (map[string]spaces.Bindings, error) {
	panic("should not be called")
}

func (sb *stubBacking) AllMachines() ([]spaces.Machine, error) {
	panic("should not be called")
}

func (sb *stubBacking) AllConstraints() ([]spaces.Constraints, error) {
	panic("should not be called")
}

// LegacySuite is deprecated testing suite that uses stubs.
// TODO (manadart 2020-03-24): These should be phased out in favour of the
// mock-based tests.
type LegacySuite struct {
	testing.BaseSuite
	apiservertesting.StubNetwork

	resources *common.Resources
	auth      apiservertesting.FakeAuthorizer
	facade    *spaces.API

	blockChecker  mockBlockChecker
	SpaceService  *spaces.MockSpaceService
	SubnetService *spaces.MockSubnetService
}

var _ = gc.Suite(&LegacySuite{})

func (s *LegacySuite) SetUpSuite(c *gc.C) {
	s.StubNetwork.SetUpSuite(c)
	s.BaseSuite.SetUpSuite(c)

	ctrl := gomock.NewController(c)
	s.SpaceService = spaces.NewMockSpaceService(ctrl)
	s.SubnetService = spaces.NewMockSubnetService(ctrl)
}

func (s *LegacySuite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
}

func (s *LegacySuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	apiservertesting.BackingInstance.SetUp(
		c,
		apiservertesting.StubZonedNetworkingEnvironName,
		apiservertesting.WithZones,
		apiservertesting.WithSpaces,
		apiservertesting.WithSubnets,
	)

	s.resources = common.NewResources()
	s.auth = apiservertesting.FakeAuthorizer{
		Tag:        names.NewUserTag("admin"),
		Controller: false,
	}

	s.blockChecker = mockBlockChecker{}
	var err error
	s.facade, err = spaces.NewAPIWithBacking(spaces.APIConfig{
		Backing:                     &stubBacking{apiservertesting.BackingInstance},
		Check:                       &s.blockChecker,
		CredentialInvalidatorGetter: apiservertesting.NoopModelCredentialInvalidatorGetter,
		Resources:                   s.resources,
		Authorizer:                  s.auth,
		SpaceService:                s.SpaceService,
		SubnetService:               s.SubnetService,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.facade, gc.NotNil)
}

func (s *LegacySuite) TearDownTest(c *gc.C) {
	if s.resources != nil {
		s.resources.StopAll()
	}
	s.BaseSuite.TearDownTest(c)
}

func (s *LegacySuite) TestNewAPIWithBacking(c *gc.C) {
	// Clients are allowed.
	facade, err := spaces.NewAPIWithBacking(spaces.APIConfig{
		Backing:                     &stubBacking{apiservertesting.BackingInstance},
		Check:                       &s.blockChecker,
		CredentialInvalidatorGetter: apiservertesting.NoopModelCredentialInvalidatorGetter,
		Resources:                   s.resources,
		Authorizer:                  s.auth,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(facade, gc.NotNil)
	// No calls so far.
	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub)

	// Agents are not allowed
	agentAuthorizer := s.auth
	agentAuthorizer.Tag = names.NewMachineTag("42")
	facade, err = spaces.NewAPIWithBacking(spaces.APIConfig{
		Backing:                     &stubBacking{apiservertesting.BackingInstance},
		Check:                       &s.blockChecker,
		CredentialInvalidatorGetter: apiservertesting.NoopModelCredentialInvalidatorGetter,
		Resources:                   s.resources,
		Authorizer:                  agentAuthorizer,
	})
	c.Assert(err, jc.DeepEquals, apiservererrors.ErrPerm)
	c.Assert(facade, gc.IsNil)
	// No calls so far.
	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub)
}

func (s *LegacySuite) TestShowSpaceError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		errors.New("boom"), // Backing.ModelConfig()
	)

	entities := params.Entities{}
	_, err := s.facade.ShowSpace(stdcontext.Background(), entities)
	c.Assert(err, gc.ErrorMatches, "getting environ: retrieving model config: boom")
}

func (s *LegacySuite) TestCreateSpacesModelConfigError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		errors.New("boom"), // Backing.ModelConfig()
	)

	args := params.CreateSpacesParams{}
	_, err := s.facade.CreateSpaces(stdcontext.Background(), args)
	c.Assert(err, gc.ErrorMatches, "getting environ: retrieving model config: boom")
}

func (s *LegacySuite) TestCreateSpacesProviderOpenError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		nil,                // Backing.ModelConfig()
		nil,                // Backing.CloudSpec()
		errors.New("boom"), // Provider.Open()
	)

	args := params.CreateSpacesParams{}
	_, err := s.facade.CreateSpaces(stdcontext.Background(), args)
	c.Assert(err, gc.ErrorMatches,
		`getting environ: creating environ for model \"stub-zoned-networking-environ\" \(.*\): boom`)
}

func (s *LegacySuite) TestCreateSpacesNotSupportedError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		nil,                            // Backing.ModelConfig()
		nil,                            // Backing.CloudSpec()
		nil,                            // Provider.Open()
		errors.NotSupportedf("spaces"), // ZonedNetworkingEnviron.SupportsSpaces()
	)

	args := params.CreateSpacesParams{}
	_, err := s.facade.CreateSpaces(stdcontext.Background(), args)
	c.Assert(err, gc.ErrorMatches, "spaces not supported")
}

func (s *LegacySuite) TestListSpacesDefault(c *gc.C) {
	expected := []params.Space{{
		Id:   "1",
		Name: "default",
		Subnets: []params.Subnet{{
			CIDR:       "192.168.0.0/24",
			ProviderId: "provider-192.168.0.0/24",
			Zones:      []string{"foo"},
			SpaceTag:   "space-default",
		}, {
			CIDR:       "192.168.3.0/24",
			ProviderId: "provider-192.168.3.0/24",
			VLANTag:    23,
			Zones:      []string{"bar", "bam"},
			SpaceTag:   "space-default",
		}},
	}, {
		Id:   "2",
		Name: "dmz",
		Subnets: []params.Subnet{{
			CIDR:       "192.168.1.0/24",
			ProviderId: "provider-192.168.1.0/24",
			VLANTag:    23,
			Zones:      []string{"bar", "bam"},
			SpaceTag:   "space-dmz",
		}},
	}, {
		Id:   "3",
		Name: "private",
		Subnets: []params.Subnet{{
			CIDR:       "192.168.2.0/24",
			ProviderId: "provider-192.168.2.0/24",
			Zones:      []string{"foo"},
			SpaceTag:   "space-private",
		}},
	}}

	result, err := s.facade.ListSpaces(stdcontext.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, jc.DeepEquals, expected)
}

func (s *LegacySuite) TestListSpacesAllSpacesError(c *gc.C) {
	boom := errors.New("backing boom")
	apiservertesting.BackingInstance.SetErrors(boom)
	_, err := s.facade.ListSpaces(stdcontext.Background())
	c.Assert(err, gc.ErrorMatches, "getting environ: retrieving model config: backing boom")
}

func (s *LegacySuite) TestListSpacesSubnetsError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		nil,                                 // Backing.ModelConfig()
		nil,                                 // Backing.CloudSpec()
		nil,                                 // Provider.Open()
		nil,                                 // ZonedNetworkingEnviron.supportsSpaces()
		nil,                                 // Backing.AllSpaces()
		errors.New("space0 subnets failed"), // Space.Subnets()
		errors.New("space1 subnets failed"), // Space.Subnets()
		errors.New("space2 subnets failed"), // Space.Subnets()
	)

	results, err := s.facade.ListSpaces(stdcontext.Background())
	c.Assert(err, jc.ErrorIsNil)
	for i, space := range results.Results {
		errmsg := fmt.Sprintf("fetching subnets: space%d subnets failed", i)
		c.Assert(space.Error, gc.ErrorMatches, errmsg)
	}
}

func (s *LegacySuite) TestListSpacesSubnetsSingleSubnetError(c *gc.C) {
	boom := errors.New("boom")
	apiservertesting.SharedStub.SetErrors(
		nil,  // Backing.ModelConfig()
		nil,  // Backing.CloudSpec()
		nil,  // Provider.Open()
		nil,  // ZonedNetworkingEnviron.supportsSpaces()
		nil,  // Backing.AllSpaces()
		nil,  // Space.Subnets() (1st no error)
		boom, // Space.Subnets() (2nd with error)
	)

	results, err := s.facade.ListSpaces(stdcontext.Background())
	c.Assert(err, jc.ErrorIsNil)
	for i, space := range results.Results {
		if i == 1 {
			c.Assert(space.Error, gc.ErrorMatches, "fetching subnets: boom")
		} else {
			c.Assert(space.Error, gc.IsNil)
		}
	}
}

func (s *LegacySuite) TestListSpacesNotSupportedError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		nil,                            // Backing.ModelConfig()
		nil,                            // Backing.CloudSpec()
		nil,                            // Provider.Open
		errors.NotSupportedf("spaces"), // ZonedNetworkingEnviron.supportsSpaces()
	)

	_, err := s.facade.ListSpaces(stdcontext.Background())
	c.Assert(err, gc.ErrorMatches, "spaces not supported")
}

func (s *LegacySuite) TestCreateSpacesBlocked(c *gc.C) {
	s.blockChecker.SetErrors(apiservererrors.ServerError(apiservererrors.OperationBlockedError("test block")))
	_, err := s.facade.CreateSpaces(stdcontext.Background(), params.CreateSpacesParams{})
	c.Assert(err, gc.ErrorMatches, "test block")
	c.Assert(err, jc.Satisfies, params.IsCodeOperationBlocked)
}

func (s *LegacySuite) TestSupportsSpacesModelConfigError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		errors.New("boom"), // Backing.ModelConfig()
	)

	err := spaces.SupportsSpaces(&stubBacking{apiservertesting.BackingInstance})
	c.Assert(err, gc.ErrorMatches, "getting environ: retrieving model config: boom")
}

func (s *LegacySuite) TestSupportsSpacesEnvironNewError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		nil,                // Backing.ModelConfig()
		nil,                // Backing.CloudSpec()
		errors.New("boom"), // environs.New()
	)

	err := spaces.SupportsSpaces(&stubBacking{apiservertesting.BackingInstance})
	c.Assert(err, gc.ErrorMatches,
		`getting environ: creating environ for model \"stub-zoned-networking-environ\" \(.*\): boom`)
}

func (s *LegacySuite) TestSupportsSpacesWithoutNetworking(c *gc.C) {
	apiservertesting.BackingInstance.SetUp(
		c,
		apiservertesting.StubEnvironName,
		apiservertesting.WithoutZones,
		apiservertesting.WithoutSpaces,
		apiservertesting.WithoutSubnets)

	err := spaces.SupportsSpaces(&stubBacking{apiservertesting.BackingInstance})
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
}

func (s *LegacySuite) TestSupportsSpacesWithoutSpaces(c *gc.C) {
	apiservertesting.BackingInstance.SetUp(
		c,
		apiservertesting.StubNetworkingEnvironName,
		apiservertesting.WithoutZones,
		apiservertesting.WithoutSpaces,
		apiservertesting.WithoutSubnets)

	apiservertesting.SharedStub.SetErrors(
		nil,                // Backing.ModelConfig()
		nil,                // Backing.CloudSpec()
		nil,                // environs.New()
		errors.New("boom"), // Backing.supportsSpaces()
	)

	err := spaces.SupportsSpaces(&stubBacking{apiservertesting.BackingInstance})
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
}

func (s *LegacySuite) TestSupportsSpaces(c *gc.C) {
	err := spaces.SupportsSpaces(&stubBacking{apiservertesting.BackingInstance})
	c.Assert(err, jc.ErrorIsNil)
}

type mockBlockChecker struct {
	jtesting.Stub
}

func (c *mockBlockChecker) ChangeAllowed(ctx stdcontext.Context) error {
	c.MethodCall(c, "ChangeAllowed")
	return c.NextErr()
}

func (c *mockBlockChecker) RemoveAllowed(ctx stdcontext.Context) error {
	c.MethodCall(c, "RemoveAllowed")
	return c.NextErr()
}
