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
	state.EXPECT().SaveProviderSubnets(subnets, "")

	err := ReloadSpaces(ctx, state, environ)
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
	state.EXPECT().SaveProviderSubnets(subnets, "").Return(errors.New("boom"))

	err := ReloadSpaces(ctx, state, environ)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *spacesSuite) TestReloadSpacesNotNetworkEnviron(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := envcontext.WithoutCredentialInvalidator(context.Background())
	state := NewMockReloadSpacesState(ctrl)
	environ := NewMockBootstrapEnviron(ctrl)

	err := ReloadSpaces(ctx, state, environ)
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
	res := []network.SpaceInfo{
		{
			ID:         "1",
			Name:       "space1",
			ProviderId: network.Id("1"),
		},
	}
	mockState.EXPECT().AllSpaces().Return(res, nil)
	mockState.EXPECT().SaveProviderSubnets([]network.SubnetInfo{{CIDR: "10.0.0.1/12"}}, "1")

	subnets := []network.SpaceInfo{
		{ProviderId: network.Id("1"), Subnets: []network.SubnetInfo{{CIDR: "10.0.0.1/12"}}},
	}

	provider := NewProviderSpaces(mockState)
	err := provider.SaveSpaces(subnets)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.modelSpaceMap, gc.DeepEquals, map[network.Id]network.SpaceInfo{
		network.Id("1"): res[0],
	})
}

func (s *providerSpacesSuite) TestSaveSpacesWithoutProviderId(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockState := NewMockReloadSpacesState(ctrl)
	res := []network.SpaceInfo{
		{
			ID:         "1",
			Name:       "space1",
			ProviderId: network.Id("1"),
		},
	}
	mockState.EXPECT().AllSpaces().Return(res, nil)
	addedSpace := network.SpaceInfo{
		ID:         "2",
		ProviderId: network.Id("2"),
	}
	mockState.EXPECT().AddSpace("empty", network.Id("2"), []string{}, false).Return(addedSpace, nil)

	mockState.EXPECT().SaveProviderSubnets([]network.SubnetInfo{{CIDR: "10.0.0.1/12"}}, "2")

	subnets := []network.SpaceInfo{
		{ProviderId: network.Id("2"), Subnets: []network.SubnetInfo{{CIDR: "10.0.0.1/12"}}},
	}

	provider := NewProviderSpaces(mockState)
	err := provider.SaveSpaces(subnets)
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

	provider := NewProviderSpaces(mockState)
	c.Assert(provider.DeltaSpaces(), gc.DeepEquals, network.MakeIDSet())
}

func (s *providerSpacesSuite) TestSaveSpacesDeltaSpacesAfterNotUpdated(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockState := NewMockReloadSpacesState(ctrl)
	res := []network.SpaceInfo{
		{
			ID:         "1",
			Name:       "space1",
			ProviderId: network.Id("1"),
		},
	}
	mockState.EXPECT().AllSpaces().Return(res, nil)
	addedSpace := network.SpaceInfo{
		ID:         "2",
		ProviderId: network.Id("2"),
	}
	mockState.EXPECT().AddSpace("empty", network.Id("2"), []string{}, false).Return(addedSpace, nil)

	mockState.EXPECT().SaveProviderSubnets([]network.SubnetInfo{{CIDR: "10.0.0.1/12"}}, "2")

	subnets := []network.SpaceInfo{
		{ProviderId: network.Id("2"), Subnets: []network.SubnetInfo{{CIDR: "10.0.0.1/12"}}},
	}

	provider := NewProviderSpaces(mockState)
	err := provider.SaveSpaces(subnets)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.DeltaSpaces(), gc.DeepEquals, network.MakeIDSet(network.Id("1")))
}

func (s *providerSpacesSuite) TestDeleteSpacesWithNoDeltas(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockState := NewMockReloadSpacesState(ctrl)

	provider := NewProviderSpaces(mockState)
	warnings, err := provider.DeleteSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(warnings, gc.DeepEquals, []string(nil))
}

func (s *providerSpacesSuite) TestDeleteSpaces(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockState := NewMockReloadSpacesState(ctrl)
	mockState.EXPECT().DefaultEndpointBindingSpace().Return("2", nil)
	mockState.EXPECT().AllEndpointBindingsSpaceNames().Return(set.NewStrings(), nil)
	mockState.EXPECT().ConstraintsBySpaceName("1").Return(nil, nil)
	mockState.EXPECT().Life("1").Return(state.Alive, nil)
	mockState.EXPECT().EnsureDead("1").Return(nil)
	mockState.EXPECT().Remove("1").Return(nil)

	provider := NewProviderSpaces(mockState)
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
	mockState.EXPECT().DefaultEndpointBindingSpace().Return("1", nil)
	mockState.EXPECT().AllEndpointBindingsSpaceNames().Return(set.NewStrings(), nil)

	provider := NewProviderSpaces(mockState)
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
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockState := NewMockReloadSpacesState(ctrl)
	mockState.EXPECT().DefaultEndpointBindingSpace().Return("1", nil)
	mockState.EXPECT().AllEndpointBindingsSpaceNames().Return(set.NewStrings(), nil)

	provider := NewProviderSpaces(mockState)
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
	mockState.EXPECT().DefaultEndpointBindingSpace().Return("2", nil)
	mockState.EXPECT().AllEndpointBindingsSpaceNames().Return(set.NewStrings("1"), nil)

	provider := NewProviderSpaces(mockState)
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
	mockState.EXPECT().DefaultEndpointBindingSpace().Return("2", nil)
	mockState.EXPECT().AllEndpointBindingsSpaceNames().Return(set.NewStrings(), nil)
	mockState.EXPECT().ConstraintsBySpaceName("1").Return([]Constraints{struct{}{}}, nil)

	provider := NewProviderSpaces(mockState)
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
	res := []network.SpaceInfo{
		{
			ID:         "1",
			Name:       "space1",
			ProviderId: network.Id("1"),
		},
	}
	mockState.EXPECT().AllSpaces().Return(res, nil)
	addedSpace := network.SpaceInfo{
		ID:         "2",
		ProviderId: network.Id("2"),
	}
	mockState.EXPECT().AddSpace("empty", network.Id("2"), []string{}, false).Return(addedSpace, nil)
	mockState.EXPECT().Life("1").Return(state.Alive, nil)
	mockState.EXPECT().EnsureDead("1").Return(nil)
	mockState.EXPECT().Remove("1").Return(nil)

	mockState.EXPECT().SaveProviderSubnets([]network.SubnetInfo{{CIDR: "10.0.0.1/12"}}, "2")

	subnets := []network.SpaceInfo{
		{ProviderId: network.Id("2"), Subnets: []network.SubnetInfo{{CIDR: "10.0.0.1/12"}}},
	}

	provider := NewProviderSpaces(mockState)
	err := provider.SaveSpaces(subnets)
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

	mockState.EXPECT().DefaultEndpointBindingSpace().Return("2", nil)
	mockState.EXPECT().AllEndpointBindingsSpaceNames().Return(set.NewStrings(), nil)
	mockState.EXPECT().ConstraintsBySpaceName("space1").Return(nil, nil)

	warnings, err := provider.DeleteSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(warnings, gc.DeepEquals, []string(nil))
}
