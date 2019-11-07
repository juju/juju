// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerizer

import (
	"strconv"

	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
)

type bridgePolicySuite struct {
	testing.IsolationSuite

	netBondReconfigureDelay   int
	containerNetworkingMethod string

	spaceIDs map[string]string
	host     *MockContainer
	guest    *MockContainer
}

var _ = gc.Suite(&bridgePolicySuite{})

func (s *bridgePolicySuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.netBondReconfigureDelay = 13
	s.containerNetworkingMethod = "local"
	s.spaceIDs = make(map[string]string)
}

func (s *bridgePolicySuite) TestDetermineContainerSpacesConstraints(c *gc.C) {
	defer s.setupMocks(c).Finish()

	exp := s.guest.EXPECT()
	exp.Constraints().Return(constraints.MustParse("spaces=foo,bar,^baz"), nil)

	spaces, err := s.policy().determineContainerSpaces(s.host, s.guest)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(spaces, jc.SameContents, []string{"2", "1"})
}

func (s *bridgePolicySuite) TestDetermineContainerNoSpacesConstraints(c *gc.C) {
	defer s.setupMocks(c).Finish()

	exp := s.guest.EXPECT()
	exp.Constraints().Return(constraints.MustParse(""), nil)

	spaces, err := s.policy().determineContainerSpaces(s.host, s.guest)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(spaces, jc.SameContents, []string{"0"})
}

func (s *bridgePolicySuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.host = NewMockContainer(ctrl)
	s.guest = NewMockContainer(ctrl)

	s.guest.EXPECT().Id().Return("guest-id").AnyTimes()

	for i, space := range []string{network.AlphaSpaceName, "foo", "bar", "fizz"} {
		// 0 is the AlphaSpaceId
		id := strconv.Itoa(i)
		s.spaceIDs[space] = id
	}

	return ctrl
}

func (s *bridgePolicySuite) policy() *BridgePolicy {
	return &BridgePolicy{
		spaceIDs:                  s.spaceIDs,
		netBondReconfigureDelay:   s.netBondReconfigureDelay,
		containerNetworkingMethod: s.containerNetworkingMethod,
	}
}
