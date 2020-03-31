// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	instance "github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
)

type spacesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&spacesSuite{})

func (s *spacesSuite) TestReloadSpaces(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	spaces := []network.SpaceInfo{
		{ID: "1", Name: "alpha"},
		{ID: "2", Name: "db"},
	}

	context := NewMockProviderCallContext(ctrl)

	environ := NewMockNetworkingEnviron(ctrl)
	environ.EXPECT().SupportsSpaceDiscovery(context).Return(true, nil)
	environ.EXPECT().Spaces(context).Return(spaces, nil)

	state := NewMockReloadSpacesState(ctrl)
	state.EXPECT().SaveProviderSpaces(spaces)

	err := ReloadSpaces(context, state, environ)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spacesSuite) TestReloadSpacesFailsOnSave(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	spaces := []network.SpaceInfo{
		{ID: "1", Name: "alpha"},
		{ID: "2", Name: "db"},
	}

	context := NewMockProviderCallContext(ctrl)

	environ := NewMockNetworkingEnviron(ctrl)
	environ.EXPECT().SupportsSpaceDiscovery(context).Return(true, nil)
	environ.EXPECT().Spaces(context).Return(spaces, nil)

	state := NewMockReloadSpacesState(ctrl)
	state.EXPECT().SaveProviderSpaces(spaces).Return(errors.New("boom"))

	err := ReloadSpaces(context, state, environ)
	c.Assert(err, gc.ErrorMatches, "boom")
}

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
