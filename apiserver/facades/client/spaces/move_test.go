// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/facades/client/spaces"
	"github.com/juju/juju/apiserver/facades/client/spaces/mocks"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/constraints"
)

func (s *SpaceTestMockSuite) TestAPIMoveSubnetsSuccess(c *gc.C) {
	ctrl, unReg := s.setupSpacesAPI(c, true, false)
	defer ctrl.Finish()
	defer unReg()

	spaceName := "destination"
	subnetID := "3"
	cidr := "10.0.0.0/24"

	subnet := expectMovingSubnet(ctrl, cidr)

	moveSubnetsOp := mocks.NewMockMoveSubnetsOp(ctrl)
	moveSubnetsOp.EXPECT().GetMovedSubnets().Return([]spaces.MovedSubnet{{
		ID:        subnetID,
		FromSpace: "from",
	}})
	s.mockOpFactory.EXPECT().NewMoveSubnetsOp(spaceName, []spaces.MovingSubnet{subnet}).Return(moveSubnetsOp, nil)

	bExp := s.mockBacking.EXPECT()
	bExp.AllConstraints().Return(nil, nil)
	bExp.MovingSubnet(subnetID).Return(subnet, nil)
	bExp.ApplyOperation(moveSubnetsOp).Return(nil)

	// Using different subnet - triggers no constraint violation.
	s.expectMachinesAndAddresses(ctrl, "10.11.12.12/14")

	res, err := s.api.MoveSubnets(moveSubnetsArg(subnetID, spaceName, false))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.DeepEquals, []params.MoveSubnetsResult{{
		MovedSubnets: []params.MovedSubnet{{
			SubnetTag:   "subnet-3",
			OldSpaceTag: "space-from",
		}},
		NewSpaceTag: "space-destination",
		Error:       nil,
	}})
}

func (s *SpaceTestMockSuite) TestAPIMoveSubnetsSubnetNotFoundError(c *gc.C) {
	ctrl, unReg := s.setupSpacesAPI(c, true, false)
	defer ctrl.Finish()
	defer unReg()

	spaceName := "destination"
	subnetID := "3"

	s.mockBacking.EXPECT().MovingSubnet(subnetID).Return(nil, errors.NotFoundf("subnet 3"))

	res, err := s.api.MoveSubnets(moveSubnetsArg(subnetID, spaceName, false))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	c.Assert(res.Results[0].Error.Message, gc.Equals, "subnet 3 not found")
}

func (s *SpaceTestMockSuite) TestAPIMoveSubnetsConstraintsViolatedNoForceError(c *gc.C) {
	ctrl, unReg := s.setupSpacesAPI(c, true, false)
	defer ctrl.Finish()
	defer unReg()

	spaceName := "destination"
	subnetID := "3"
	cidr := "10.0.0.0/24"

	subnet := expectMovingSubnet(ctrl, cidr)

	// MySQL is constrained to be in a different space.
	cons := mocks.NewMockConstraints(ctrl)
	cons.EXPECT().ID().Return("c9741ea1-0c2a-444d-82f5-787583a48557:a#mysql")
	cons.EXPECT().Value().Return(constraints.MustParse("spaces=a-different-space"))

	s.expectMachinesAndAddresses(ctrl, cidr, "mysql")

	bExp := s.mockBacking.EXPECT()
	bExp.AllConstraints().Return([]spaces.Constraints{cons}, nil)
	bExp.MovingSubnet(subnetID).Return(subnet, nil)

	res, err := s.api.MoveSubnets(moveSubnetsArg(subnetID, spaceName, false))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	c.Assert(res.Results[0].Error.Message, gc.Equals,
		`moving subnet "10.0.0.0/24" to space "destination" violates space constraints for application `+
			`"mysql": a-different-space`)
}

func (s *SpaceTestMockSuite) TestMoveSubnetsConstraintsViolatedForceSuccess(c *gc.C) {
	ctrl, unReg := s.setupSpacesAPI(c, true, false)
	defer ctrl.Finish()
	defer unReg()

	spaceName := "destination"
	subnetID := "3"
	cidr := "10.0.0.0/24"

	subnet := expectMovingSubnet(ctrl, cidr)

	// MySQL is constrained to be in a different space.
	cons := mocks.NewMockConstraints(ctrl)
	cons.EXPECT().ID().Return("c9741ea1-0c2a-444d-82f5-787583a48557:a#mysql")
	cons.EXPECT().Value().Return(constraints.MustParse("spaces=a-different-space"))

	s.expectMachinesAndAddresses(ctrl, cidr, "mysql")

	moveSubnetsOp := mocks.NewMockMoveSubnetsOp(ctrl)
	moveSubnetsOp.EXPECT().GetMovedSubnets().Return([]spaces.MovedSubnet{{
		ID:        subnetID,
		FromSpace: "from",
	}})
	s.mockOpFactory.EXPECT().NewMoveSubnetsOp(spaceName, []spaces.MovingSubnet{subnet}).Return(moveSubnetsOp, nil)

	bExp := s.mockBacking.EXPECT()
	bExp.AllConstraints().Return([]spaces.Constraints{cons}, nil)
	bExp.MovingSubnet(subnetID).Return(subnet, nil)
	bExp.ApplyOperation(moveSubnetsOp).Return(nil)

	// Supplying force=true succeeds despite the violation.
	res, err := s.api.MoveSubnets(moveSubnetsArg(subnetID, spaceName, true))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.DeepEquals, []params.MoveSubnetsResult{{
		MovedSubnets: []params.MovedSubnet{{
			SubnetTag:   "subnet-3",
			OldSpaceTag: "space-from",
		}},
		NewSpaceTag: "space-destination",
		Error:       nil,
	}})
}

func expectMovingSubnet(ctrl *gomock.Controller, cidr string) *mocks.MockMovingSubnet {
	subnetMock := mocks.NewMockMovingSubnet(ctrl)
	subnetMock.EXPECT().CIDR().Return(cidr)
	return subnetMock
}

func (s *SpaceTestMockSuite) expectMachinesAndAddresses(ctrl *gomock.Controller, subnetCIDR string, apps ...string) {
	address := mocks.NewMockAddress(ctrl)
	address.EXPECT().SubnetCIDR().Return(subnetCIDR)

	machine := mocks.NewMockMachine(ctrl)
	machine.EXPECT().AllAddresses().Return([]spaces.Address{address}, nil)

	if len(apps) > 0 {
		machine.EXPECT().ApplicationNames().Return(apps, nil)
	}

	s.mockBacking.EXPECT().AllMachines().Return([]spaces.Machine{machine}, nil)
}

func moveSubnetsArg(subnetID, spaceName string, force bool) params.MoveSubnetsParams {
	return params.MoveSubnetsParams{
		Args: []params.MoveSubnetsParam{{
			SubnetTags: []string{names.NewSubnetTag(subnetID).String()},
			SpaceTag:   names.NewSpaceTag(spaceName).String(),
			Force:      force,
		}},
	}
}
