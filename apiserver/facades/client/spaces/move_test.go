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

	sub1 := spaces.NewMockMovingSubnet(ctrl)
	sub2 := spaces.NewMockMovingSubnet(ctrl)

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
	sub1.EXPECT().CIDR().Return("10.0.0.10/24")

	sub2.EXPECT().ID().Return("2")
	sub2.EXPECT().SpaceName().Return("space-2")
	sub2.EXPECT().CIDR().Return("10.0.1.10/16")

	c.Assert(op.Done(err), jc.ErrorIsNil)
	c.Check(op.GetMovedSubnets(), gc.DeepEquals, []spaces.MovedSubnet{
		{
			ID:        "1",
			FromSpace: "space-1",
			CIDR:      "10.0.0.10/24",
		},
		{
			ID:        "2",
			FromSpace: "space-2",
			CIDR:      "10.0.1.10/16",
		},
	})
}

func (s *moveSubnetsOpSuite) TestError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	spaceID := "13"

	space := netmocks.NewMockBackingSpace(ctrl)
	space.EXPECT().Id().Return(spaceID)

	sub1 := spaces.NewMockMovingSubnet(ctrl)
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

type moveSubnetsAPISuite struct {
	spaces.APISuite
}

var _ = gc.Suite(&moveSubnetsAPISuite{})

func (s *moveSubnetsAPISuite) TestMoveSubnetsSuccess(c *gc.C) {
	ctrl, unReg := s.SetupMocks(c, true, false)
	defer ctrl.Finish()
	defer unReg()

	spaceName := "destination"
	subnetID := "3"
	cidr := "10.0.0.0/24"

	subnet := expectMovingSubnet(ctrl, cidr, "")

	moveSubnetsOp := spaces.NewMockMoveSubnetsOp(ctrl)
	moveSubnetsOp.EXPECT().GetMovedSubnets().Return([]spaces.MovedSubnet{{
		ID:        subnetID,
		FromSpace: "from",
	}})
	s.OpFactory.EXPECT().NewMoveSubnetsOp(spaceName, []spaces.MovingSubnet{subnet}).Return(moveSubnetsOp, nil)

	bExp := s.Backing.EXPECT()
	bExp.AllConstraints().Return(nil, nil)
	bExp.MovingSubnet(subnetID).Return(subnet, nil)
	bExp.ApplyOperation(moveSubnetsOp).Return(nil)

	// Using different subnet - triggers no constraint violation.
	s.expectMachinesAndAddresses(ctrl, "10.11.12.12/14")

	res, err := s.API.MoveSubnets(moveSubnetsArg(subnetID, spaceName, false))
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

func (s *moveSubnetsAPISuite) TestMoveSubnetsSubnetNotFoundError(c *gc.C) {
	ctrl, unReg := s.SetupMocks(c, true, false)
	defer ctrl.Finish()
	defer unReg()

	spaceName := "destination"
	subnetID := "3"

	s.Backing.EXPECT().MovingSubnet(subnetID).Return(nil, errors.NotFoundf("subnet 3"))

	res, err := s.API.MoveSubnets(moveSubnetsArg(subnetID, spaceName, false))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	c.Assert(res.Results[0].Error.Message, gc.Equals, "subnet 3 not found")
}

func (s *moveSubnetsAPISuite) TestMoveSubnetsConstraintsViolatedNoForceError(c *gc.C) {
	ctrl, unReg := s.SetupMocks(c, true, false)
	defer ctrl.Finish()
	defer unReg()

	spaceName := "destination"
	subnetID := "3"
	cidr := "10.0.0.0/24"

	subnet := expectMovingSubnet(ctrl, cidr, "")

	// MySQL is constrained to be in a different space.
	cons := spaces.NewMockConstraints(ctrl)
	cons.EXPECT().ID().Return("c9741ea1-0c2a-444d-82f5-787583a48557:a#mysql")
	cons.EXPECT().Value().Return(constraints.MustParse("spaces=a-different-space"))

	s.expectMachinesAndAddresses(ctrl, cidr, "mysql")

	bExp := s.Backing.EXPECT()
	bExp.AllConstraints().Return([]spaces.Constraints{cons}, nil)
	bExp.MovingSubnet(subnetID).Return(subnet, nil)

	res, err := s.API.MoveSubnets(moveSubnetsArg(subnetID, spaceName, false))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	c.Assert(res.Results[0].Error.Message, gc.Equals,
		`moving subnet "10.0.0.0/24" to space "destination" violates space constraints for application `+
			`"mysql": a-different-space`)
}

func (s *moveSubnetsAPISuite) TestSubnetsConstraintsViolatedForceSuccess(c *gc.C) {
	ctrl, unReg := s.SetupMocks(c, true, false)
	defer ctrl.Finish()
	defer unReg()

	spaceName := "destination"
	subnetID := "3"
	cidr := "10.0.0.0/24"

	subnet := expectMovingSubnet(ctrl, cidr, "")

	// MySQL is constrained to be in a different space.
	cons := spaces.NewMockConstraints(ctrl)
	cons.EXPECT().ID().Return("c9741ea1-0c2a-444d-82f5-787583a48557:a#mysql")
	cons.EXPECT().Value().Return(constraints.MustParse("spaces=a-different-space"))

	s.expectMachinesAndAddresses(ctrl, cidr, "mysql")

	moveSubnetsOp := spaces.NewMockMoveSubnetsOp(ctrl)
	moveSubnetsOp.EXPECT().GetMovedSubnets().Return([]spaces.MovedSubnet{{
		ID:        subnetID,
		FromSpace: "from",
	}})
	s.OpFactory.EXPECT().NewMoveSubnetsOp(spaceName, []spaces.MovingSubnet{subnet}).Return(moveSubnetsOp, nil)

	bExp := s.Backing.EXPECT()
	bExp.AllConstraints().Return([]spaces.Constraints{cons}, nil)
	bExp.MovingSubnet(subnetID).Return(subnet, nil)
	bExp.ApplyOperation(moveSubnetsOp).Return(nil)

	// Supplying force=true succeeds despite the violation.
	res, err := s.API.MoveSubnets(moveSubnetsArg(subnetID, spaceName, true))
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

func (s *moveSubnetsAPISuite) TestMoveSubnetsHasUnderlayError(c *gc.C) {
	ctrl, unReg := s.SetupMocks(c, true, false)
	defer ctrl.Finish()
	defer unReg()

	spaceName := "destination"
	subnetID := "3"
	cidr := "10.0.0.0/8"

	subnet := expectMovingSubnet(ctrl, cidr, "20.0.0.0/24")

	bExp := s.Backing.EXPECT()
	bExp.MovingSubnet(subnetID).Return(subnet, nil)

	res, err := s.API.MoveSubnets(moveSubnetsArg(subnetID, spaceName, false))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	c.Assert(res.Results[0].Error, gc.NotNil)
	c.Assert(res.Results[0].Error.Message, gc.Equals,
		`subnet "10.0.0.0/8" is a fan overlay of "20.0.0.0/24" and cannot be moved; move the underlay instead`)
}

func expectMovingSubnet(ctrl *gomock.Controller, cidr, underlay string) *spaces.MockMovingSubnet {
	subnetMock := spaces.NewMockMovingSubnet(ctrl)

	subnetMock.EXPECT().CIDR().Return(cidr).MinTimes(1)
	subnetMock.EXPECT().FanLocalUnderlay().Return(underlay).MinTimes(1)

	return subnetMock
}

func (s *moveSubnetsAPISuite) expectMachinesAndAddresses(ctrl *gomock.Controller, subnetCIDR string, apps ...string) {
	address := spaces.NewMockAddress(ctrl)
	address.EXPECT().SubnetCIDR().Return(subnetCIDR)

	machine := spaces.NewMockMachine(ctrl)
	machine.EXPECT().AllAddresses().Return([]spaces.Address{address}, nil)

	if len(apps) > 0 {
		machine.EXPECT().ApplicationNames().Return(apps, nil)
	}

	s.Backing.EXPECT().AllMachines().Return([]spaces.Machine{machine}, nil)
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
