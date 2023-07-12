// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerizer

import (
	"strconv"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
)

type bridgePolicySuite struct {
	testing.IsolationSuite

	netBondReconfigureDelay   int
	containerNetworkingMethod string

	spaces network.SpaceInfos
	host   *MockContainer
	guest  *MockContainer
}

var _ = gc.Suite(&bridgePolicySuite{})

func (s *bridgePolicySuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.netBondReconfigureDelay = 13
	s.containerNetworkingMethod = "local"
}

func (s *bridgePolicySuite) TestDetermineContainerSpacesConstraints(c *gc.C) {
	defer s.setupMocks(c).Finish()

	exp := s.guest.EXPECT()
	exp.Constraints().Return(constraints.MustParse("spaces=foo,bar,^baz"), nil)

	obtained, err := s.policy().determineContainerSpaces(s.host, s.guest)
	c.Assert(err, jc.ErrorIsNil)
	expected := network.SpaceInfos{
		*s.spaces.GetByName("foo"),
		*s.spaces.GetByName("bar"),
	}
	c.Check(obtained, jc.DeepEquals, expected)
}

func (s *bridgePolicySuite) TestDetermineContainerNoSpacesConstraints(c *gc.C) {
	defer s.setupMocks(c).Finish()

	exp := s.guest.EXPECT()
	exp.Constraints().Return(constraints.MustParse(""), nil)

	obtained, err := s.policy().determineContainerSpaces(s.host, s.guest)
	c.Assert(err, jc.ErrorIsNil)
	expected := network.SpaceInfos{
		*s.spaces.GetByName(network.AlphaSpaceName),
	}
	c.Check(obtained, jc.DeepEquals, expected)
}

func (s *bridgePolicySuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.host = NewMockContainer(ctrl)
	s.guest = NewMockContainer(ctrl)

	s.guest.EXPECT().Id().Return("guest-id").AnyTimes()

	s.spaces = make(network.SpaceInfos, 4)
	for i, space := range []string{network.AlphaSpaceName, "foo", "bar", "fizz"} {
		// 0 is the AlphaSpaceId
		id := strconv.Itoa(i)
		s.spaces[i] = network.SpaceInfo{ID: id, Name: network.SpaceName(space)}
	}
	return ctrl
}

func (s *bridgePolicySuite) policy() *BridgePolicy {
	return &BridgePolicy{
		spaces:                    s.spaces,
		netBondReconfigureDelay:   s.netBondReconfigureDelay,
		containerNetworkingMethod: s.containerNetworkingMethod,
	}
}
