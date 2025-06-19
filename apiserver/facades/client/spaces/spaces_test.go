// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces_test

import (
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
	"github.com/juju/juju/apiserver/common/networkingcommon"
	netmocks "github.com/juju/juju/apiserver/common/networkingcommon/mocks"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/client/spaces"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/context"
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

func (s *APISuite) TestShowSpaceDefault(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()

	s.expectDefaultSpace(ctrl, "default", nil, nil)
	s.expectEndpointBindings(ctrl, "1")
	s.expectMachines(ctrl, s.getDefaultSpaces(), nil, nil)
	s.Backing.EXPECT().ControllerConfig().Return(controller.Config{
		"controller-uuid": testing.ControllerTag.Id(),
	}, nil)

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

	res, err := s.API.ShowSpace(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, jc.DeepEquals, expected)
}

func (s *APISuite) TestCheckSupportsSpacesControllerConfigFail(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()

	s.Backing.EXPECT().ControllerConfig().Return(controller.Config{}, errors.New("broken controller"))

	expectedApplications := []string{"mysql", "mediawiki"}
	sort.Strings(expectedApplications)
	args := s.getShowSpaceArg("default")

	// checkSupportsSpaces is a private method, so use ShowSpace() as the top level method
	// because it invokes checkSupportsSpaces
	res, err := s.API.ShowSpace(args)
	c.Assert(err, gc.ErrorMatches, "getting controller config: broken controller")
	c.Assert(res, jc.DeepEquals, params.ShowSpaceResults{})
}

func (s *APISuite) TestShowSpaceErrorGettingSpace(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()

	bamErr := errors.New("bam")
	s.expectDefaultSpace(ctrl, "default", bamErr, nil)
	args := s.getShowSpaceArg("default")
	s.Backing.EXPECT().ControllerConfig().Return(controller.Config{
		"controller-uuid": testing.ControllerTag.Id(),
	}, nil)

	res, err := s.API.ShowSpace(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := fmt.Sprintf("fetching space %q: %v", args.Entities[0].Tag, bamErr.Error())
	c.Assert(res.Results[0].Error, gc.ErrorMatches, expectedErr)
}

func (s *APISuite) TestShowSpaceErrorGettingSubnets(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()

	bamErr := errors.New("bam")
	s.expectDefaultSpace(ctrl, "default", nil, bamErr)
	s.Backing.EXPECT().ControllerConfig().Return(controller.Config{
		"controller-uuid": testing.ControllerTag.Id(),
	}, nil)
	args := s.getShowSpaceArg("default")

	res, err := s.API.ShowSpace(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := fmt.Sprintf("fetching subnets: %v", bamErr.Error())
	c.Assert(res.Results[0].Error, gc.ErrorMatches, expectedErr)
}

func (s *APISuite) TestShowSpaceErrorGettingApplications(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()

	expErr := errors.New("bam")
	s.expectDefaultSpace(ctrl, "default", nil, nil)
	s.Backing.EXPECT().AllEndpointBindings().Return(nil, expErr)
	s.Backing.EXPECT().ControllerConfig().Return(controller.Config{
		"controller-uuid": testing.ControllerTag.Id(),
	}, nil)

	args := s.getShowSpaceArg("default")

	res, err := s.API.ShowSpace(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := fmt.Sprintf("fetching applications: %v", expErr.Error())
	c.Assert(res.Results[0].Error, gc.ErrorMatches, expectedErr)
}

func (s *APISuite) TestShowSpaceErrorGettingMachines(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()

	bamErr := errors.New("bam")
	s.expectDefaultSpace(ctrl, "default", nil, nil)
	s.expectEndpointBindings(ctrl, "1")
	s.expectMachines(ctrl, s.getDefaultSpaces(), bamErr, nil)
	s.Backing.EXPECT().ControllerConfig().Return(controller.Config{
		"controller-uuid": testing.ControllerTag.Id(),
	}, nil)

	args := s.getShowSpaceArg("default")
	res, err := s.API.ShowSpace(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := fmt.Sprintf("fetching machine count: %v", bamErr.Error())
	c.Assert(res.Results[0].Error, gc.ErrorMatches, expectedErr)
}

func (s *APISuite) TestRenameSpaceErrorToAlreadyExist(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()

	s.expectDefaultSpace(ctrl, "blub", nil, nil)
	s.Backing.EXPECT().ControllerConfig().Return(controller.Config{
		"controller-uuid": testing.ControllerTag.Id(),
	}, nil)

	from, to := "bla", "blub"
	args := s.getRenameArgs(from, to)

	res, err := s.API.RenameSpace(args)
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
	s.expectDefaultSpace(ctrl, to, bamErr, nil)
	s.Backing.EXPECT().ControllerConfig().Return(controller.Config{
		"controller-uuid": testing.ControllerTag.Id(),
	}, nil)

	args := s.getRenameArgs(from, to)

	res, err := s.API.RenameSpace(args)
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
	s.expectDefaultSpace(ctrl, to, errors.NotFoundf(""), nil)
	args := s.getRenameArgs(from, to)

	s.OpFactory.EXPECT().NewRenameSpaceOp(from, to).Return(nil, bamErr)
	s.Backing.EXPECT().ControllerConfig().Return(controller.Config{
		"controller-uuid": testing.ControllerTag.Id(),
	}, nil)

	res, err := s.API.RenameSpace(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results[0].Error, gc.ErrorMatches, bamErr.Error())
}

func (s *APISuite) TestRenameAlphaSpaceError(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()
	from, to := "alpha", "blub"

	args := s.getRenameArgs(from, to)
	s.Backing.EXPECT().ControllerConfig().Return(controller.Config{
		"controller-uuid": testing.ControllerTag.Id(),
	}, nil)

	res, err := s.API.RenameSpace(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results[0].Error, gc.ErrorMatches, `the "alpha" space cannot be renamed`)
}

func (s *APISuite) TestRenameSpaceSuccess(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()
	from, to := "bla", "blub"

	s.OpFactory.EXPECT().NewRenameSpaceOp(from, to).Return(s.renameSpaceOp, nil)
	s.expectDefaultSpace(ctrl, to, errors.NotFoundf("abc"), nil)
	s.Backing.EXPECT().ApplyOperation(s.renameSpaceOp).Return(nil)
	s.Backing.EXPECT().ControllerConfig().Return(controller.Config{
		"controller-uuid": testing.ControllerTag.Id(),
	}, nil)
	args := s.getRenameArgs(from, to)

	res, err := s.API.RenameSpace(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results[0].Error, gc.IsNil)
}

func (s *APISuite) TestRenameSpaceErrorProviderSpacesSupport(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, true)
	defer ctrl.Finish()
	defer unreg()
	from, to := "bla", "blub"

	args := s.getRenameArgs(from, to)
	s.Backing.EXPECT().ControllerConfig().Return(controller.Config{
		"controller-uuid": testing.ControllerTag.Id(),
	}, nil)

	res, err := s.API.RenameSpace(args)
	c.Assert(err, gc.ErrorMatches, "modifying provider-sourced spaces not supported")
	c.Assert(res, gc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult(nil)})
}

func (s *APISuite) TestRemoveSpaceSuccessNoControllerConfig(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()
	space := "myspace"
	args, tag := s.getRemoveArgs(space, false)

	s.expectDefaultSpace(ctrl, space, nil, nil)
	s.expectEndpointBindings(ctrl, "2")
	s.Backing.EXPECT().ConstraintsBySpaceName(space).Return(nil, nil)
	s.Backing.EXPECT().IsController().Return(false)
	s.OpFactory.EXPECT().NewRemoveSpaceOp(tag.Id()).Return(nil, nil)
	s.Backing.EXPECT().ApplyOperation(nil).Return(nil)
	s.Backing.EXPECT().ControllerConfig().Return(controller.Config{
		"controller-uuid": testing.ControllerTag.Id(),
	}, nil)

	res, err := s.API.RemoveSpace(args)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.RemoveSpaceResults{Results: []params.RemoveSpaceResult{{}}})
}

func (s *APISuite) TestRemoveSpaceSuccessControllerConfig(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()
	space := "myspace"
	args, tag := s.getRemoveArgs(space, false)

	s.expectDefaultSpace(ctrl, space, nil, nil)
	s.expectEndpointBindings(ctrl, "2")
	s.Backing.EXPECT().ConstraintsBySpaceName(space).Return(nil, nil)
	s.Backing.EXPECT().IsController().Return(true)
	s.OpFactory.EXPECT().NewRemoveSpaceOp(tag.Id()).Return(nil, nil)
	s.Backing.EXPECT().ApplyOperation(nil).Return(nil)
	s.Backing.EXPECT().ControllerConfig().Return(
		controller.Config{
			"controller-uuid": testing.ControllerTag.Id(),
		}, nil).Times(2)

	res, err := s.API.RemoveSpace(args)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.RemoveSpaceResults{Results: []params.RemoveSpaceResult{{}}})
}

func (s *APISuite) TestRemoveSpaceErrorFoundApplications(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()
	space := "myspace"
	args, _ := s.getRemoveArgs(space, false)

	s.expectDefaultSpace(ctrl, space, nil, nil)
	s.expectEndpointBindings(ctrl, "1")
	s.Backing.EXPECT().IsController().Return(false)
	s.Backing.EXPECT().ConstraintsBySpaceName(space).Return(nil, nil)
	s.Backing.EXPECT().ControllerConfig().Return(controller.Config{
		"controller-uuid": testing.ControllerTag.Id(),
	}, nil)
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

	res, err := s.API.RemoveSpace(args)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, expected)
}

func (s *APISuite) TestRemoveSpaceErrorFoundController(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()
	space := "myspace"
	args, _ := s.getRemoveArgs(space, false)

	s.expectDefaultSpace(ctrl, space, nil, nil)
	s.expectEndpointBindings(ctrl, "2")
	s.Backing.EXPECT().IsController().Return(true)

	currentConfig := s.getDefaultControllerConfig(c, map[string]interface{}{
		controller.JujuHASpace:         "nothing",
		controller.JujuManagementSpace: space,
		"controller-uuid":              testing.ControllerTag.Id()},
	)
	s.Backing.EXPECT().ConstraintsBySpaceName(space).Return(nil, nil)
	s.Backing.EXPECT().ControllerConfig().Return(currentConfig, nil).Times(2)

	expected := params.RemoveSpaceResults{Results: []params.RemoveSpaceResult{{
		Bindings:           nil,
		Constraints:        nil,
		ControllerSettings: []string{controller.JujuManagementSpace},
		Error:              nil,
	}}}

	res, err := s.API.RemoveSpace(args)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, expected)
}

func (s *APISuite) TestRemoveSpaceErrorFoundConstraints(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, false)
	defer ctrl.Finish()
	defer unreg()
	space := "myspace"
	args, _ := s.getRemoveArgs(space, false)

	s.expectDefaultSpace(ctrl, space, nil, nil)
	s.expectEndpointBindings(ctrl, "2")
	s.Backing.EXPECT().IsController().Return(false)
	s.Backing.EXPECT().ControllerConfig().Return(controller.Config{
		"controller-uuid": testing.ControllerTag.Id(),
	}, nil)

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

	res, err := s.API.RemoveSpace(args)

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

	s.expectDefaultSpace(ctrl, space, nil, nil)
	s.expectEndpointBindings(ctrl, "1")
	s.Backing.EXPECT().IsController().Return(true)

	currentConfig := s.getDefaultControllerConfig(c, map[string]interface{}{
		controller.JujuHASpace:         "nothing",
		controller.JujuManagementSpace: space,
		"controller-uuid":              testing.ControllerTag.Id()},
	)
	s.Backing.EXPECT().ControllerConfig().Return(currentConfig, nil).Times(2)

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

	res, err := s.API.RemoveSpace(args)

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
	space := "myspace"
	args, tag := s.getRemoveArgs(space, true)

	s.expectDefaultSpace(ctrl, space, nil, nil)
	s.expectEndpointBindings(ctrl, "1")
	s.Backing.EXPECT().IsController().Return(true)

	s.OpFactory.EXPECT().NewRemoveSpaceOp(tag.Id()).Return(nil, nil)
	s.Backing.EXPECT().ApplyOperation(nil).Return(nil)
	currentConfig := s.getDefaultControllerConfig(c, map[string]interface{}{
		controller.JujuHASpace:         "nothing",
		controller.JujuManagementSpace: space,
		"controller-uuid":              testing.ControllerTag.Id()},
	)
	s.Backing.EXPECT().ControllerConfig().Return(currentConfig, nil).Times(2)

	_, _ = s.expectAllTags(space)

	expected := params.RemoveSpaceResults{Results: []params.RemoveSpaceResult{{}}}

	res, err := s.API.RemoveSpace(args)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, expected)
}

func (s *APISuite) TestRemoveSpaceErrorProviderSpacesSupport(c *gc.C) {
	ctrl, unreg := s.setupMocks(c, true, true)
	defer ctrl.Finish()
	defer unreg()
	space := "myspace"

	args, _ := s.getRemoveArgs(space, false)
	s.Backing.EXPECT().ControllerConfig().Return(controller.Config{
		"controller-uuid": testing.ControllerTag.Id(),
	}, nil)

	_, err := s.API.RemoveSpace(args)
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

	s.Backing.EXPECT().AllEndpointBindings().Return(map[string]spaces.Bindings{
		"mysql":     b1,
		"mediawiki": b2,
	}, nil)
}

// expectDefaultSpace configures a default space mock with default subnet settings
func (s *APISuite) expectDefaultSpace(ctrl *gomock.Controller, name string, spacesErr, subnetErr error) {
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
	backingSpaceInfo := network.SpaceInfo{
		ID:      "1",
		Name:    network.SpaceName(name),
		Subnets: backingSubnets,
	}

	spacesMock := netmocks.NewMockBackingSpace(ctrl)
	spacesMock.EXPECT().Id().Return("1").AnyTimes()
	spacesMock.EXPECT().Name().Return(name).AnyTimes()
	spacesMock.EXPECT().NetworkSpace().Return(backingSpaceInfo, subnetErr).AnyTimes()
	if spacesErr != nil {
		s.Backing.EXPECT().SpaceByName(name).Return(nil, spacesErr)
	} else {
		s.Backing.EXPECT().SpaceByName(name).Return(spacesMock, nil)
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
	return controller.Config{
		"controller-uuid": testing.ControllerTag.Id(),
	}, nil
}

func (sb *stubBacking) SpaceByName(_ string) (networkingcommon.BackingSpace, error) {
	panic("should not be called")
}

func (sb *stubBacking) AllEndpointBindings() (map[string]spaces.Bindings, error) {
	panic("should not be called")
}

func (sb *stubBacking) AllMachines() ([]spaces.Machine, error) {
	panic("should not be called")
}

func (sb *stubBacking) AllConstraints() ([]spaces.Constraints, error) {
	panic("should not be called")
}

func (sb *stubBacking) MovingSubnet(string) (spaces.MovingSubnet, error) {
	panic("should not be called")
}

func (sb *stubBacking) AllSpaceInfos() (network.SpaceInfos, error) {
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

	callContext  context.ProviderCallContext
	blockChecker mockBlockChecker
}

var _ = gc.Suite(&LegacySuite{})

func (s *LegacySuite) SetUpSuite(c *gc.C) {
	s.StubNetwork.SetUpSuite(c)
	s.BaseSuite.SetUpSuite(c)
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

	s.callContext = context.NewEmptyCloudCallContext()
	s.blockChecker = mockBlockChecker{}
	var err error
	s.facade, err = spaces.NewAPIWithBacking(spaces.APIConfig{
		Backing:    &stubBacking{apiservertesting.BackingInstance},
		Check:      &s.blockChecker,
		Context:    s.callContext,
		Resources:  s.resources,
		Authorizer: s.auth,
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
		Backing:    &stubBacking{apiservertesting.BackingInstance},
		Check:      &s.blockChecker,
		Context:    s.callContext,
		Resources:  s.resources,
		Authorizer: s.auth,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(facade, gc.NotNil)
	// No calls so far.
	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub)

	// Agents are not allowed
	agentAuthorizer := s.auth
	agentAuthorizer.Tag = names.NewMachineTag("42")
	facade, err = spaces.NewAPIWithBacking(spaces.APIConfig{
		Backing:    &stubBacking{apiservertesting.BackingInstance},
		Check:      &s.blockChecker,
		Context:    context.NewEmptyCloudCallContext(),
		Resources:  s.resources,
		Authorizer: agentAuthorizer,
	})
	c.Assert(err, jc.DeepEquals, apiservererrors.ErrPerm)
	c.Assert(facade, gc.IsNil)
	// No calls so far.
	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub)
}

type checkAddSpacesParams struct {
	Name       string
	Subnets    []string
	Error      string
	MakesCall  bool
	Public     bool
	ProviderId string
}

func (s *LegacySuite) checkAddSpaces(c *gc.C, p checkAddSpacesParams) {
	arg := params.CreateSpaceParams{
		Public:     p.Public,
		ProviderId: p.ProviderId,
	}
	if p.Name != "" {
		arg.SpaceTag = "space-" + p.Name
	}
	if len(p.Subnets) > 0 {
		arg.CIDRs = p.Subnets
	}

	args := params.CreateSpacesParams{
		Spaces: []params.CreateSpaceParams{arg},
	}

	results, err := s.facade.CreateSpaces(args)
	c.Assert(err, gc.IsNil)
	c.Assert(len(results.Results), gc.Equals, 1)
	if p.Error == "" {
		c.Assert(results.Results[0].Error, gc.IsNil)
	} else {
		c.Assert(results.Results[0].Error, gc.NotNil)
		c.Assert(results.Results[0].Error, gc.ErrorMatches, p.Error)
	}

	baseCalls := []apiservertesting.StubMethodCall{
		apiservertesting.BackingCall("ModelConfig"),
		apiservertesting.BackingCall("CloudSpec"),
		apiservertesting.ProviderCall("Open", apiservertesting.BackingInstance.EnvConfig),
		apiservertesting.ZonedNetworkingEnvironCall("SupportsSpaces", s.callContext),
	}

	// If we have an expected error, no calls to SubnetByCIDR() nor
	// AddSpace() should be made.  Check the methods called and
	// return.  The exception is TestAddSpacesAPIError cause an
	// error after SubnetByCIDR() is called.
	if p.Error != "" && !subnetCallMade() {
		apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub, baseCalls...)
		return
	}

	allCalls := baseCalls
	var subnetIDs []string
	for _, cidr := range p.Subnets {
		allCalls = append(allCalls, apiservertesting.BackingCall("SubnetByCIDR", cidr))
		for _, fakeSN := range apiservertesting.BackingInstance.Subnets {
			if fakeSN.CIDR() == cidr {
				subnetIDs = append(subnetIDs, fakeSN.ID())
			}
		}
	}

	// Only add the call to AddSpace() if there are no errors
	// which have continued to this point.
	if p.Error == "" {
		allCalls = append(allCalls, apiservertesting.BackingCall("AddSpace", p.Name, network.Id(p.ProviderId), subnetIDs, p.Public))
	}
	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub, allCalls...)
}

func subnetCallMade() bool {
	for _, call := range apiservertesting.SharedStub.Calls() {
		if call.FuncName == "SubnetByCIDR" {
			return true
		}
	}
	return false
}

func (s *LegacySuite) TestAddSpacesOneSubnet(c *gc.C) {
	p := checkAddSpacesParams{
		Name:    "foo",
		Subnets: []string{"10.10.0.0/24"},
	}
	s.checkAddSpaces(c, p)
}

func (s *LegacySuite) TestAddSpacesTwoSubnets(c *gc.C) {
	apiservertesting.BackingInstance.AdditionalSubnets()
	p := checkAddSpacesParams{
		Name:    "foo",
		Subnets: []string{"10.10.0.0/24", "10.0.2.0/24"},
	}
	s.checkAddSpaces(c, p)
}

func (s *LegacySuite) TestAddSpacesManySubnets(c *gc.C) {
	apiservertesting.BackingInstance.AdditionalSubnets()
	p := checkAddSpacesParams{
		Name: "foo",
		Subnets: []string{"10.10.0.0/24", "10.0.2.0/24",
			"10.0.3.0/24", "10.0.4.0/24"},
	}
	s.checkAddSpaces(c, p)
}

func (s *LegacySuite) TestAddSpacesCreateInvalidSpace(c *gc.C) {
	p := checkAddSpacesParams{
		Name:    "-",
		Subnets: []string{"10.0.0.0/24"},
		Error:   `"space--" is not a valid space tag`,
	}
	s.checkAddSpaces(c, p)
}

func (s *LegacySuite) TestAddSpacesCreateInvalidCIDR(c *gc.C) {
	p := checkAddSpacesParams{
		Name:    "foo",
		Subnets: []string{"bar"},
		Error:   `"bar" is not a valid CIDR`,
	}
	s.checkAddSpaces(c, p)
}

func (s *LegacySuite) TestAddSpacesPublic(c *gc.C) {
	p := checkAddSpacesParams{
		Name:    "foo",
		Subnets: []string{"10.10.0.0/24"},
		Public:  true,
	}
	s.checkAddSpaces(c, p)
}

func (s *LegacySuite) TestAddSpacesProviderId(c *gc.C) {
	p := checkAddSpacesParams{
		Name:       "foo",
		Subnets:    []string{"10.10.0.0/24"},
		ProviderId: "foobar",
	}
	s.checkAddSpaces(c, p)
}

func (s *LegacySuite) TestAddSpacesEmptySpaceName(c *gc.C) {
	p := checkAddSpacesParams{
		Subnets: []string{"10.0.0.0/24"},
		Error:   `"" is not a valid tag`,
	}
	s.checkAddSpaces(c, p)
}

func (s *LegacySuite) TestAddSpacesNoSubnets(c *gc.C) {
	p := checkAddSpacesParams{
		Name:    "foo",
		Subnets: nil,
	}
	s.checkAddSpaces(c, p)
}

func (s *LegacySuite) TestAddSpacesAPIError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		nil,                                // Backing.ModelConfig()
		nil,                                // Backing.CloudSpec()
		nil,                                // Provider.Open()
		nil,                                // ZonedNetworkingEnviron.SupportsSpaces()
		errors.AlreadyExistsf("space-foo"), // Backing.AddSpace()
	)
	p := checkAddSpacesParams{
		Name:      "foo",
		Subnets:   []string{"10.10.0.0/24"},
		MakesCall: true,
		Error:     "space-foo already exists",
	}
	s.checkAddSpaces(c, p)
}

func (s *LegacySuite) TestShowSpaceError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		errors.New("boom"), // Backing.ModelConfig()
	)

	entities := params.Entities{}
	_, err := s.facade.ShowSpace(entities)
	c.Assert(err, gc.ErrorMatches, "getting environ: retrieving model config: boom")
}

func (s *LegacySuite) TestCreateSpacesModelConfigError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		errors.New("boom"), // Backing.ModelConfig()
	)

	args := params.CreateSpacesParams{}
	_, err := s.facade.CreateSpaces(args)
	c.Assert(err, gc.ErrorMatches, "getting environ: retrieving model config: boom")
}

func (s *LegacySuite) TestCreateSpacesProviderOpenError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		nil,                // Backing.ModelConfig()
		nil,                // Backing.CloudSpec()
		errors.New("boom"), // Provider.Open()
	)

	args := params.CreateSpacesParams{}
	_, err := s.facade.CreateSpaces(args)
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
	_, err := s.facade.CreateSpaces(args)
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

	result, err := s.facade.ListSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, jc.DeepEquals, expected)
}

func (s *LegacySuite) TestListSpacesAllSpacesError(c *gc.C) {
	boom := errors.New("backing boom")
	apiservertesting.BackingInstance.SetErrors(boom)
	_, err := s.facade.ListSpaces()
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

	results, err := s.facade.ListSpaces()
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

	results, err := s.facade.ListSpaces()
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

	_, err := s.facade.ListSpaces()
	c.Assert(err, gc.ErrorMatches, "spaces not supported")
}

func (s *LegacySuite) TestCreateSpacesBlocked(c *gc.C) {
	s.blockChecker.SetErrors(apiservererrors.ServerError(apiservererrors.OperationBlockedError("test block")))
	_, err := s.facade.CreateSpaces(params.CreateSpacesParams{})
	c.Assert(err, gc.ErrorMatches, "test block")
	c.Assert(err, jc.Satisfies, params.IsCodeOperationBlocked)
}

func (s *LegacySuite) TestSupportsSpacesModelConfigError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		errors.New("boom"), // Backing.ModelConfig()
	)

	err := spaces.SupportsSpaces(&stubBacking{apiservertesting.BackingInstance}, context.NewEmptyCloudCallContext())
	c.Assert(err, gc.ErrorMatches, "getting environ: retrieving model config: boom")
}

func (s *LegacySuite) TestSupportsSpacesEnvironNewError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		nil,                // Backing.ModelConfig()
		nil,                // Backing.CloudSpec()
		errors.New("boom"), // environs.New()
	)

	err := spaces.SupportsSpaces(&stubBacking{apiservertesting.BackingInstance}, context.NewEmptyCloudCallContext())
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

	err := spaces.SupportsSpaces(&stubBacking{apiservertesting.BackingInstance}, context.NewEmptyCloudCallContext())
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
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

	err := spaces.SupportsSpaces(&stubBacking{apiservertesting.BackingInstance}, context.NewEmptyCloudCallContext())
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *LegacySuite) TestSupportsSpaces(c *gc.C) {
	err := spaces.SupportsSpaces(&stubBacking{apiservertesting.BackingInstance}, context.NewEmptyCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)
}

type mockBlockChecker struct {
	jtesting.Stub
}

func (c *mockBlockChecker) ChangeAllowed() error {
	c.MethodCall(c, "ChangeAllowed")
	return c.NextErr()
}

func (c *mockBlockChecker) RemoveAllowed() error {
	c.MethodCall(c, "RemoveAllowed")
	return c.NextErr()
}
