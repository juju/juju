// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces_test

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/apiserver/facades/client/spaces"
	"github.com/juju/juju/apiserver/facades/client/spaces/mocks"
	"github.com/juju/juju/core/network"
)

type SpaceRemoveSuite struct {
	space  *mocks.MockRemoveSpace
	subnet *mocks.MockSubnet
}

var _ = gc.Suite(&SpaceRemoveSuite{})

func (s *SpaceRemoveSuite) TearDownTest(c *gc.C) {
}
func (s *SpaceRemoveSuite) TestSuccess(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	removeSpaceOps := []txn.Op{{
		C:      "1",
		Id:     "2",
		Remove: true,
	}}

	moveSubnetOps := []txn.Op{{
		C:      "1",
		Remove: false,
	}}

	s.space.EXPECT().RemoveSpaceOps().Return(removeSpaceOps)
	s.subnet.EXPECT().MoveSubnetOps(network.AlphaSpaceId).Return(moveSubnetOps)
	op := spaces.NewRemoveSpaceModelOp(s.space, []spaces.Subnet{s.subnet})

	ops, err := op.Build(0)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ops, gc.HasLen, 2)
}

func (s *SpaceRemoveSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.space = mocks.NewMockRemoveSpace(ctrl)
	s.subnet = mocks.NewMockSubnet(ctrl)

	return ctrl
}
