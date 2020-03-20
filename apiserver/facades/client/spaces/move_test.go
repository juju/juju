// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"
	"gopkg.in/mgo.v2/txn"

	netmocks "github.com/juju/juju/apiserver/common/networkingcommon/mocks"
	"github.com/juju/juju/apiserver/facades/client/spaces"
	"github.com/juju/juju/apiserver/facades/client/spaces/mocks"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/constraints"
)

// moveSubsetOpSuite tests the model operation used to
// move subnets to a new space.
type moveSubnetsOpSuite struct{}

var _ = gc.Suite(&moveSubnetsOpSuite{})

func (s *moveSubnetsOpSuite) TestSuccess(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	spaceID := "13"

	space := netmocks.NewMockBackingSpace(ctrl)
	space.EXPECT().Id().Return(spaceID).MinTimes(1)

	sub1 := mocks.NewMockMovingSubnet(ctrl)
	sub2 := mocks.NewMockMovingSubnet(ctrl)

	// Here we are just testing that we get an op per subnet.
	sub1.EXPECT().UpdateSpaceOps(spaceID).Return([]txn.Op{{}})
	sub2.EXPECT().UpdateSpaceOps(spaceID).Return([]txn.Op{{}})

	op := spaces.NewMoveSubnetsOp(space, []spaces.MovingSubnet{sub1, sub2})

	txnOps, err := op.Build(0)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(txnOps, gc.HasLen, 2)

	// Now test that we get the correct return for GetMovedSubnets.
	sub1.EXPECT().ID().Return("1")
	sub1.EXPECT().SpaceName().Return("space-1")

	sub2.EXPECT().ID().Return("2")
	sub2.EXPECT().SpaceName().Return("space-2")

	c.Assert(op.Done(err), jc.ErrorIsNil)
	c.Check(op.GetMovedSubnets(), gc.DeepEquals, []spaces.MovedSubnet{
		{
			ID:        "1",
			FromSpace: "space-1",
		},
		{
			ID:        "2",
			FromSpace: "space-2",
		},
	})
}

func (s *moveSubnetsOpSuite) TestError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	spaceID := "13"

	space := netmocks.NewMockBackingSpace(ctrl)
	space.EXPECT().Id().Return(spaceID)

	sub1 := mocks.NewMockMovingSubnet(ctrl)
	sub1.EXPECT().UpdateSpaceOps(spaceID).Return([]txn.Op{{}})

	op := spaces.NewMoveSubnetsOp(space, []spaces.MovingSubnet{sub1})

	_, err := op.Build(0)
	c.Assert(err, jc.ErrorIsNil)

	// Now simulate getting an error when running the txn,
	// and having it passed into the Done method.
	err = errors.New("belch")
	c.Assert(op.Done(err), gc.ErrorMatches, "belch")

	c.Check(op.GetMovedSubnets(), gc.IsNil)
}

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
