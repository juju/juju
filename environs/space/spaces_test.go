// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	instance "github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
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

	context := NewMockProviderCallContext(ctrl)

	environ := NewMockNetworkingEnviron(ctrl)
	environ.EXPECT().SupportsSpaceDiscovery(context).Return(false, nil)
	environ.EXPECT().Subnets(context, instance.UnknownId, nil).Return(subnets, nil)

	state := NewMockReloadSpacesState(ctrl)
	state.EXPECT().SaveProviderSubnets(subnets, "")

	err := ReloadSpaces(context, state, environ)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spacesSuite) TestReloadSpacesUsingSubnetsFailsOnSave(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	subnets := []network.SubnetInfo{
		{CIDR: "10.0.0.1/12"},
		{CIDR: "10.12.24.1/24"},
	}

	context := NewMockProviderCallContext(ctrl)

	environ := NewMockNetworkingEnviron(ctrl)
	environ.EXPECT().SupportsSpaceDiscovery(context).Return(false, nil)
	environ.EXPECT().Subnets(context, instance.UnknownId, nil).Return(subnets, nil)

	state := NewMockReloadSpacesState(ctrl)
	state.EXPECT().SaveProviderSubnets(subnets, "").Return(errors.New("boom"))

	err := ReloadSpaces(context, state, environ)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *spacesSuite) TestReloadSpacesNotNetworkEnviron(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	context := NewMockProviderCallContext(ctrl)
	state := NewMockReloadSpacesState(ctrl)
	environ := NewMockBootstrapEnviron(ctrl)

	err := ReloadSpaces(context, state, environ)
	c.Assert(err, gc.ErrorMatches, "spaces discovery in a non-networking environ not supported")
}

type providerSpacesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&providerSpacesSuite{})

func (s *providerSpacesSuite) TestSaveSpaces(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockSpace := NewMockSpace(ctrl)
	mockSpace.EXPECT().ProviderId().Return(network.Id("1"))
	mockSpace.EXPECT().Name().Return("space1")
	mockSpace.EXPECT().Id().Return("1")

	mockState := NewMockReloadSpacesState(ctrl)
	mockState.EXPECT().AllSpaces().Return([]Space{mockSpace}, nil)
	mockState.EXPECT().SaveProviderSubnets([]network.SubnetInfo{{CIDR: "10.0.0.1/12"}}, "1")

	subnets := []network.SpaceInfo{
		{ProviderId: network.Id("1"), Subnets: []network.SubnetInfo{{CIDR: "10.0.0.1/12"}}},
	}

	provider := NewProviderSpaces(mockState)
	err := provider.SaveSpaces(subnets)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.modelSpaceMap, gc.DeepEquals, map[network.Id]Space{
		network.Id("1"): mockSpace,
	})
}

func (s *providerSpacesSuite) TestSaveSpacesWithoutProviderId(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockSpace := NewMockSpace(ctrl)
	mockSpace.EXPECT().ProviderId().Return(network.Id("1"))
	mockSpace.EXPECT().Name().Return("space1")

	newMockSpace := NewMockSpace(ctrl)
	newMockSpace.EXPECT().Id().Return("2")
	newMockSpace.EXPECT().ProviderId().Return(network.Id("2"))

	mockState := NewMockReloadSpacesState(ctrl)
	mockState.EXPECT().AllSpaces().Return([]Space{mockSpace}, nil)
	mockState.EXPECT().AddSpace("empty", network.Id("2"), []string{}, false).Return(newMockSpace, nil)

	mockState.EXPECT().SaveProviderSubnets([]network.SubnetInfo{{CIDR: "10.0.0.1/12"}}, "2")

	subnets := []network.SpaceInfo{
		{ProviderId: network.Id("2"), Subnets: []network.SubnetInfo{{CIDR: "10.0.0.1/12"}}},
	}

	provider := NewProviderSpaces(mockState)
	err := provider.SaveSpaces(subnets)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.modelSpaceMap, gc.DeepEquals, map[network.Id]Space{
		network.Id("1"): mockSpace,
		network.Id("2"): newMockSpace,
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

	mockSpace := NewMockSpace(ctrl)
	mockSpace.EXPECT().ProviderId().Return(network.Id("1"))
	mockSpace.EXPECT().Name().Return("space1")

	newMockSpace := NewMockSpace(ctrl)
	newMockSpace.EXPECT().Id().Return("2")
	newMockSpace.EXPECT().ProviderId().Return(network.Id("2"))

	mockState := NewMockReloadSpacesState(ctrl)
	mockState.EXPECT().AllSpaces().Return([]Space{mockSpace}, nil)
	mockState.EXPECT().AddSpace("empty", network.Id("2"), []string{}, false).Return(newMockSpace, nil)

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

	mockSpace := NewMockSpace(ctrl)
	mockSpace.EXPECT().Name().Return("1").MinTimes(1)
	mockSpace.EXPECT().Id().Return("1").MinTimes(1)
	mockSpace.EXPECT().Life().Return(state.Alive)

	// These are the important calls to check for.
	mockSpace.EXPECT().EnsureDead().Return(nil)
	mockSpace.EXPECT().Remove().Return(nil)

	mockState := NewMockReloadSpacesState(ctrl)
	mockState.EXPECT().DefaultEndpointBindingSpace().Return("2", nil)
	mockState.EXPECT().AllEndpointBindingsSpaceNames().Return(set.NewStrings(), nil)
	mockState.EXPECT().ConstraintsBySpaceName("1").Return(nil, nil)

	provider := NewProviderSpaces(mockState)
	provider.modelSpaceMap = map[network.Id]Space{
		network.Id("1"): mockSpace,
	}

	warnings, err := provider.DeleteSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(warnings, gc.DeepEquals, []string(nil))
}

func (s *providerSpacesSuite) TestDeleteSpacesMatchesAlphaSpace(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockSpace := NewMockSpace(ctrl)
	mockSpace.EXPECT().Name().Return("alpha").MinTimes(1)

	mockState := NewMockReloadSpacesState(ctrl)
	mockState.EXPECT().DefaultEndpointBindingSpace().Return("1", nil)
	mockState.EXPECT().AllEndpointBindingsSpaceNames().Return(set.NewStrings(), nil)

	provider := NewProviderSpaces(mockState)
	provider.modelSpaceMap = map[network.Id]Space{
		network.Id("1"): mockSpace,
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

	mockSpace := NewMockSpace(ctrl)
	mockSpace.EXPECT().Name().Return("1").MinTimes(1)
	mockSpace.EXPECT().Id().Return("1").MinTimes(1)

	mockState := NewMockReloadSpacesState(ctrl)
	mockState.EXPECT().DefaultEndpointBindingSpace().Return("1", nil)
	mockState.EXPECT().AllEndpointBindingsSpaceNames().Return(set.NewStrings(), nil)

	provider := NewProviderSpaces(mockState)
	provider.modelSpaceMap = map[network.Id]Space{
		network.Id("1"): mockSpace,
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

	mockSpace := NewMockSpace(ctrl)
	mockSpace.EXPECT().Name().Return("1").MinTimes(1)
	mockSpace.EXPECT().Id().Return("1").MinTimes(1)

	mockState := NewMockReloadSpacesState(ctrl)
	mockState.EXPECT().DefaultEndpointBindingSpace().Return("2", nil)
	mockState.EXPECT().AllEndpointBindingsSpaceNames().Return(set.NewStrings("1"), nil)

	provider := NewProviderSpaces(mockState)
	provider.modelSpaceMap = map[network.Id]Space{
		network.Id("1"): mockSpace,
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

	mockSpace := NewMockSpace(ctrl)
	mockSpace.EXPECT().Name().Return("1").MinTimes(1)
	mockSpace.EXPECT().Id().Return("1").MinTimes(1)

	mockState := NewMockReloadSpacesState(ctrl)
	mockState.EXPECT().DefaultEndpointBindingSpace().Return("2", nil)
	mockState.EXPECT().AllEndpointBindingsSpaceNames().Return(set.NewStrings(), nil)
	mockState.EXPECT().ConstraintsBySpaceName("1").Return([]Constraints{struct{}{}}, nil)

	provider := NewProviderSpaces(mockState)
	provider.modelSpaceMap = map[network.Id]Space{
		network.Id("1"): mockSpace,
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

	mockSpace := NewMockSpace(ctrl)
	mockSpace.EXPECT().ProviderId().Return(network.Id("1")).MinTimes(1)
	mockSpace.EXPECT().Name().Return("1").MinTimes(1)
	mockSpace.EXPECT().Id().Return("1").MinTimes(1)
	mockSpace.EXPECT().Life().Return(state.Alive)
	mockSpace.EXPECT().EnsureDead().Return(nil)
	mockSpace.EXPECT().Remove().Return(nil)

	newMockSpace := NewMockSpace(ctrl)
	newMockSpace.EXPECT().Id().Return("2")
	newMockSpace.EXPECT().ProviderId().Return(network.Id("2"))

	mockState := NewMockReloadSpacesState(ctrl)
	mockState.EXPECT().AllSpaces().Return([]Space{mockSpace}, nil)
	mockState.EXPECT().AddSpace("empty", network.Id("2"), []string{}, false).Return(newMockSpace, nil)

	mockState.EXPECT().SaveProviderSubnets([]network.SubnetInfo{{CIDR: "10.0.0.1/12"}}, "2")

	subnets := []network.SpaceInfo{
		{ProviderId: network.Id("2"), Subnets: []network.SubnetInfo{{CIDR: "10.0.0.1/12"}}},
	}

	provider := NewProviderSpaces(mockState)
	err := provider.SaveSpaces(subnets)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.modelSpaceMap, gc.DeepEquals, map[network.Id]Space{
		network.Id("1"): mockSpace,
		network.Id("2"): newMockSpace,
	})

	mockState.EXPECT().DefaultEndpointBindingSpace().Return("2", nil)
	mockState.EXPECT().AllEndpointBindingsSpaceNames().Return(set.NewStrings(), nil)
	mockState.EXPECT().ConstraintsBySpaceName("1").Return(nil, nil)

	warnings, err := provider.DeleteSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(warnings, gc.DeepEquals, []string(nil))
}
