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

	spaces   map[string]string
	host     *MockContainer
	guest    *MockContainer
	unit     *MockUnit
	app      *MockApplication
	bindings *MockBindings
}

var _ = gc.Suite(&bridgePolicySuite{})

func (s *bridgePolicySuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.netBondReconfigureDelay = 13
	s.containerNetworkingMethod = "local"
	s.spaces = make(map[string]string)
}

func (s *bridgePolicySuite) TestDetermineContainerSpacesConstraints(c *gc.C) {
	defer s.setupMocks(c).Finish()

	exp := s.guest.EXPECT()
	exp.Constraints().Return(constraints.MustParse("spaces=foo,bar,^baz"), nil)
	exp.Units().Return(nil, nil)

	spaces, err := s.policy().determineContainerSpaces(s.host, s.guest)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(spaces, jc.SameContents, []string{"2", "1"})
}

func (s *bridgePolicySuite) TestDetermineContainerSpacesEndpoints(c *gc.C) {
	defer s.setupMocks(c).Finish()

	exp := s.guest.EXPECT()
	exp.Constraints().Return(constraints.MustParse("spaces="), nil)
	exp.Units().Return([]Unit{s.unit}, nil)

	s.unit.EXPECT().Application().Return(s.app, nil)
	s.app.EXPECT().EndpointBindings().Return(s.bindings, nil)
	s.bindings.EXPECT().Map().Return(map[string]string{"endpoint": "3"})

	spaces, err := s.policy().determineContainerSpaces(s.host, s.guest)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(spaces, jc.SameContents, []string{"3"})
}

func (s *bridgePolicySuite) TestDetermineContainerSpacesConstraintsAndEndpoints(c *gc.C) {
	defer s.setupMocks(c).Finish()

	exp := s.guest.EXPECT()
	exp.Constraints().Return(constraints.MustParse("spaces=foo,bar,^baz"), nil)
	exp.Units().Return([]Unit{s.unit}, nil)

	s.unit.EXPECT().Application().Return(s.app, nil)
	s.app.EXPECT().EndpointBindings().Return(s.bindings, nil)
	s.bindings.EXPECT().Map().Return(map[string]string{"": "0", "endpoint": "3"})

	spaces, err := s.policy().determineContainerSpaces(s.host, s.guest)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(spaces, jc.SameContents, []string{"2", "3", "1"})
}

func (s *bridgePolicySuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.host = NewMockContainer(ctrl)
	s.guest = NewMockContainer(ctrl)
	s.unit = NewMockUnit(ctrl)
	s.app = NewMockApplication(ctrl)
	s.bindings = NewMockBindings(ctrl)

	s.guest.EXPECT().Id().Return("guest-id").AnyTimes()

	for i, space := range []string{network.DefaultSpaceName, "foo", "bar", "fizz"} {
		// 0 is the DefaultSpaceId
		id := strconv.Itoa(i)
		s.spaces[space] = id
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
