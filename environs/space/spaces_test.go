// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/state"
)

type spacesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&spacesSuite{})

func (s *spacesSuite) TestReloadSpacesUsingSubnets(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	subnets := []network.SubnetInfo{
		{CIDR: "10.0.0.1/12"},
		{CIDR: "10.12.24.1/24"},
	}

	ctx := envcontext.WithoutCredentialInvalidator(context.Background())

	environ := NewMockNetworkingEnviron(ctrl)
	environ.EXPECT().SupportsSpaceDiscovery(ctx).Return(false, nil)
	environ.EXPECT().Subnets(ctx, instance.UnknownId, nil).Return(subnets, nil)

	state := NewMockReloadSpacesState(ctrl)

	spaceService := NewMockSpaceService(ctrl)
	spaceService.EXPECT().SaveProviderSubnets(gomock.Any(), subnets, network.Id("0"), gomock.Any())

	err := ReloadSpaces(ctx, state, spaceService, environ, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spacesSuite) TestReloadSpacesUsingSubnetsFailsOnSave(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	subnets := []network.SubnetInfo{
		{CIDR: "10.0.0.1/12"},
		{CIDR: "10.12.24.1/24"},
	}

	ctx := envcontext.WithoutCredentialInvalidator(context.Background())

	environ := NewMockNetworkingEnviron(ctrl)
	environ.EXPECT().SupportsSpaceDiscovery(ctx).Return(false, nil)
	environ.EXPECT().Subnets(ctx, instance.UnknownId, nil).Return(subnets, nil)

	state := NewMockReloadSpacesState(ctrl)

	spaceService := NewMockSpaceService(ctrl)
	spaceService.EXPECT().SaveProviderSubnets(gomock.Any(), subnets, network.Id("0"), gomock.Any()).Return(errors.New("boom"))

	err := ReloadSpaces(ctx, state, spaceService, environ, nil)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *spacesSuite) TestReloadSpacesNotNetworkEnviron(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := envcontext.WithoutCredentialInvalidator(context.Background())
	state := NewMockReloadSpacesState(ctrl)
	environ := NewMockBootstrapEnviron(ctrl)
	spaceService := NewMockSpaceService(ctrl)

	err := ReloadSpaces(ctx, state, spaceService, environ, nil)
	c.Assert(err, gc.ErrorMatches, "spaces discovery in a non-networking environ not supported")
}

type providerSpacesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&providerSpacesSuite{})

func (s *providerSpacesSuite) TestSaveSpaces(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockState := NewMockReloadSpacesState(ctrl)
	mockSpaceService := NewMockSpaceService(ctrl)
	res := []network.SpaceInfo{
		{
			ID:         "1",
			Name:       "space1",
			ProviderId: network.Id("1"),
		},
	}
	mockSpaceService.EXPECT().GetAllSpaces(gomock.Any()).Return(res, nil)
	mockSpaceService.EXPECT().SaveProviderSubnets(gomock.Any(), []network.SubnetInfo{{CIDR: "10.0.0.1/12"}}, network.Id("1"), nil)

	subnets := []network.SpaceInfo{
		{ProviderId: network.Id("1"), Subnets: []network.SubnetInfo{{CIDR: "10.0.0.1/12"}}},
	}

	provider := NewProviderSpaces(context.Background(), mockState, mockSpaceService)
	err := provider.SaveSpaces(subnets, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.modelSpaceMap, gc.DeepEquals, map[network.Id]network.SpaceInfo{
		network.Id("1"): res[0],
	})
}

func (s *providerSpacesSuite) TestSaveSpacesWithoutProviderId(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockState := NewMockReloadSpacesState(ctrl)
	mockSpaceService := NewMockSpaceService(ctrl)
	res := []network.SpaceInfo{
		{
			ID:         "1",
			Name:       "space1",
			ProviderId: network.Id("1"),
		},
	}
	mockSpaceService.EXPECT().GetAllSpaces(gomock.Any()).Return(res, nil)
	addedSpace := network.SpaceInfo{
		ID:         "2",
		ProviderId: network.Id("2"),
	}
	mockSpaceService.EXPECT().AddSpace(gomock.Any(), "empty", network.Id("2"), []string{}).Return(network.Id(addedSpace.ID), nil)
	mockSpaceService.EXPECT().Space(gomock.Any(), addedSpace.ID).Return(&addedSpace, nil)
	mockSpaceService.EXPECT().SaveProviderSubnets(gomock.Any(), []network.SubnetInfo{{CIDR: "10.0.0.1/12"}}, network.Id("2"), nil)

	subnets := []network.SpaceInfo{
		{ProviderId: network.Id("2"), Subnets: []network.SubnetInfo{{CIDR: "10.0.0.1/12"}}},
	}

	provider := NewProviderSpaces(context.Background(), mockState, mockSpaceService)
	err := provider.SaveSpaces(subnets, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.modelSpaceMap, gc.DeepEquals, map[network.Id]network.SpaceInfo{
		network.Id("1"): res[0],
		network.Id("2"): addedSpace,
	})
}

func (s *providerSpacesSuite) TestSaveSpacesDeltaSpaces(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockState := NewMockReloadSpacesState(ctrl)
	MockSpaceService := NewMockSpaceService(ctrl)

	provider := NewProviderSpaces(context.Background(), mockState, MockSpaceService)
	c.Assert(provider.DeltaSpaces(), gc.DeepEquals, network.MakeIDSet())
}

func (s *providerSpacesSuite) TestSaveSpacesDeltaSpacesAfterNotUpdated(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockState := NewMockReloadSpacesState(ctrl)
	mockSpaceService := NewMockSpaceService(ctrl)
	res := []network.SpaceInfo{
		{
			ID:         "1",
			Name:       "space1",
			ProviderId: network.Id("1"),
		},
	}
	mockSpaceService.EXPECT().GetAllSpaces(gomock.Any()).Return(res, nil)
	addedSpace := network.SpaceInfo{
		ID:         "2",
		ProviderId: network.Id("2"),
	}
	mockSpaceService.EXPECT().AddSpace(gomock.Any(), "empty", network.Id("2"), []string{}).Return(network.Id(addedSpace.ID), nil)
	mockSpaceService.EXPECT().Space(gomock.Any(), addedSpace.ID).Return(&addedSpace, nil)
	mockSpaceService.EXPECT().SaveProviderSubnets(gomock.Any(), []network.SubnetInfo{{CIDR: "10.0.0.1/12"}}, network.Id("2"), nil)

	subnets := []network.SpaceInfo{
		{ProviderId: network.Id("2"), Subnets: []network.SubnetInfo{{CIDR: "10.0.0.1/12"}}},
	}

	provider := NewProviderSpaces(context.Background(), mockState, mockSpaceService)
	err := provider.SaveSpaces(subnets, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.DeltaSpaces(), gc.DeepEquals, network.MakeIDSet(network.Id("1")))
}

func (s *providerSpacesSuite) TestDeleteSpacesWithNoDeltas(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockState := NewMockReloadSpacesState(ctrl)
	mockSpaceService := NewMockSpaceService(ctrl)

	provider := NewProviderSpaces(context.Background(), mockState, mockSpaceService)
	warnings, err := provider.DeleteSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(warnings, gc.DeepEquals, []string(nil))
}

func (s *providerSpacesSuite) TestDeleteSpaces(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockState := NewMockReloadSpacesState(ctrl)
	mockSpaceService := NewMockSpaceService(ctrl)

	mockState.EXPECT().AllEndpointBindingsSpaceNames().Return(set.NewStrings(), nil)
	mockState.EXPECT().ConstraintsBySpaceName("1").Return(nil, nil)
	mockSpaceService.EXPECT().Remove(gomock.Any(), "1").Return(nil)

	provider := NewProviderSpaces(context.Background(), mockState, mockSpaceService)
	provider.modelSpaceMap = map[network.Id]network.SpaceInfo{
		network.Id("1"): {
			ID:         "1",
			Name:       "1",
			ProviderId: network.Id("1"),
		},
	}

	warnings, err := provider.DeleteSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(warnings, gc.DeepEquals, []string(nil))
}

func (s *providerSpacesSuite) TestDeleteSpacesMatchesAlphaSpace(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockState := NewMockReloadSpacesState(ctrl)
	mockSpaceService := NewMockSpaceService(ctrl)

	mockState.EXPECT().AllEndpointBindingsSpaceNames().Return(set.NewStrings(), nil)

	provider := NewProviderSpaces(context.Background(), mockState, mockSpaceService)
	provider.modelSpaceMap = map[network.Id]network.SpaceInfo{
		network.Id("1"): {
			ID:   "1",
			Name: "alpha",
		},
	}

	warnings, err := provider.DeleteSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(warnings, gc.DeepEquals, []string{
		`Unable to delete space "alpha". Space is used as the default space.`,
	})
}

func (s *providerSpacesSuite) TestDeleteSpacesMatchesDefaultBindingSpace(c *gc.C) {
	c.Skip("The default space is always alpha due to scaffolding in service of Dqlite migration.")

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockState := NewMockReloadSpacesState(ctrl)
	mockSpaceService := NewMockSpaceService(ctrl)

	mockState.EXPECT().AllEndpointBindingsSpaceNames().Return(set.NewStrings(), nil)

	provider := NewProviderSpaces(context.Background(), mockState, mockSpaceService)
	provider.modelSpaceMap = map[network.Id]network.SpaceInfo{
		network.Id("1"): {
			ID:   "1",
			Name: "1",
		},
	}

	warnings, err := provider.DeleteSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(warnings, gc.DeepEquals, []string{
		`Unable to delete space "1". Space is used as the default space.`,
	})
}

func (s *providerSpacesSuite) TestDeleteSpacesContainedInAllEndpointBindings(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockState := NewMockReloadSpacesState(ctrl)
	mockSpaceService := NewMockSpaceService(ctrl)

	mockState.EXPECT().AllEndpointBindingsSpaceNames().Return(set.NewStrings("1"), nil)

	provider := NewProviderSpaces(context.Background(), mockState, mockSpaceService)
	provider.modelSpaceMap = map[network.Id]network.SpaceInfo{
		network.Id("1"): {
			ID:   "1",
			Name: "1",
		},
	}

	warnings, err := provider.DeleteSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(warnings, gc.DeepEquals, []string{
		`Unable to delete space "1". Space is used as a endpoint binding.`,
	})
}

func (s *providerSpacesSuite) TestDeleteSpacesContainsConstraintsSpace(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockState := NewMockReloadSpacesState(ctrl)
	mockSpaceService := NewMockSpaceService(ctrl)

	mockState.EXPECT().AllEndpointBindingsSpaceNames().Return(set.NewStrings(), nil)
	mockState.EXPECT().ConstraintsBySpaceName("1").Return([]*state.Constraints{{}}, nil)

	provider := NewProviderSpaces(context.Background(), mockState, mockSpaceService)
	provider.modelSpaceMap = map[network.Id]network.SpaceInfo{
		network.Id("1"): {
			ID:   "1",
			Name: "1",
		},
	}

	warnings, err := provider.DeleteSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(warnings, gc.DeepEquals, []string{
		`Unable to delete space "1". Space is used in a constraint.`,
	})
}

func (s *providerSpacesSuite) TestProviderSpacesRun(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockState := NewMockReloadSpacesState(ctrl)
	mockSpaceService := NewMockSpaceService(ctrl)

	res := []network.SpaceInfo{
		{
			ID:         "1",
			Name:       "space1",
			ProviderId: network.Id("1"),
		},
	}
	mockSpaceService.EXPECT().GetAllSpaces(gomock.Any()).Return(res, nil)
	addedSpace := network.SpaceInfo{
		ID:         "2",
		ProviderId: network.Id("2"),
	}
	mockSpaceService.EXPECT().AddSpace(gomock.Any(), "empty", network.Id("2"), []string{}).Return(network.Id(addedSpace.ID), nil)
	mockSpaceService.EXPECT().Space(gomock.Any(), addedSpace.ID).Return(&addedSpace, nil)
	mockSpaceService.EXPECT().Remove(gomock.Any(), "1").Return(nil)
	mockSpaceService.EXPECT().SaveProviderSubnets(gomock.Any(), []network.SubnetInfo{{CIDR: "10.0.0.1/12"}}, network.Id("2"), nil)

	subnets := []network.SpaceInfo{
		{ProviderId: network.Id("2"), Subnets: []network.SubnetInfo{{CIDR: "10.0.0.1/12"}}},
	}

	provider := NewProviderSpaces(context.Background(), mockState, mockSpaceService)
	err := provider.SaveSpaces(subnets, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.modelSpaceMap, gc.DeepEquals, map[network.Id]network.SpaceInfo{
		network.Id("1"): {
			ID:         "1",
			Name:       "space1",
			ProviderId: network.Id("1"),
		},
		network.Id("2"): {
			ID:         "2",
			ProviderId: network.Id("2"),
		},
	})

	mockState.EXPECT().AllEndpointBindingsSpaceNames().Return(set.NewStrings(), nil)
	mockState.EXPECT().ConstraintsBySpaceName("space1").Return(nil, nil)

	warnings, err := provider.DeleteSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(warnings, gc.DeepEquals, []string(nil))
}
