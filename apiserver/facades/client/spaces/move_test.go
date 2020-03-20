// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/juju/apiserver/params"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/facades/client/spaces"
	"github.com/juju/juju/apiserver/facades/client/spaces/mocks"
)

func (s *SpaceTestMockSuite) TestAPIMoveSubnetsSuccess(c *gc.C) {
	ctrl, unReg := s.setupSpacesAPI(c, true, false)
	defer ctrl.Finish()
	defer unReg()

	spaceName := "destination"
	spaceTag := names.NewSpaceTag(spaceName)
	subnetID := "3"
	cidr := "10.0.0.0/24"

	moveSubnetsOp := mocks.NewMockMoveSubnetsOp(ctrl)
	moveSubnetsOp.EXPECT().GetMovedSubnets().Return([]spaces.MovedSubnet{{
		ID:        subnetID,
		FromSpace: "from",
	}})

	subnet := expectMovingSubnet(ctrl, cidr)

	bExp := s.mockBacking.EXPECT()
	bExp.AllConstraints().Return(nil, nil)
	bExp.MovingSubnet(subnetID).Return(subnet, nil)
	bExp.ApplyOperation(moveSubnetsOp).Return(nil)

	s.mockOpFactory.EXPECT().NewMoveSubnetsOp(spaceName, []spaces.MovingSubnet{subnet}).Return(moveSubnetsOp, nil)
	s.expectMachinesAndAddresses(ctrl, "10.11.12.12/14", nil, nil)

	arg := params.MoveSubnetsParams{
		Args: []params.MoveSubnetsParam{{
			SubnetTags: []string{names.NewSubnetTag(subnetID).String()},
			SpaceTag:   spaceTag.String(),
			Force:      false,
		}},
	}
	res, err := s.api.MoveSubnets(arg)
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

func (s *SpaceTestMockSuite) expectMachinesAndAddresses(
	ctrl *gomock.Controller, subnetCIDR string, machErr, addressesErr error,
) {
	address := mocks.NewMockAddress(ctrl)
	address.EXPECT().SubnetCIDR().Return(subnetCIDR)

	machine := mocks.NewMockMachine(ctrl)
	machine.EXPECT().AllAddresses().Return([]spaces.Address{address}, addressesErr)

	s.mockBacking.EXPECT().AllMachines().Return([]spaces.Machine{machine}, machErr)
}
