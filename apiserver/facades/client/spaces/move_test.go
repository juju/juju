// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/apiserver/facades/client/spaces"
	"github.com/juju/juju/apiserver/facades/client/spaces/mocks"
)

type SpaceUpdateSuite struct {
	moveSubnet *mocks.MockMoveSubnet
}

var _ = gc.Suite(&SpaceUpdateSuite{})

func (s *SpaceUpdateSuite) TearDownTest(c *gc.C) {
}

func (s *SpaceUpdateSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.moveSubnet = mocks.NewMockMoveSubnet(ctrl)

	return ctrl
}

func (s *SpaceUpdateSuite) TestSuccess(c *gc.C) {
	spaceName := "spaceA"

	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.moveSubnet.EXPECT().SpaceName().Return(spaceName)
	s.moveSubnet.EXPECT().UpdateSpaceOps(spaceName).Return([]txn.Op{}, nil)
	s.moveSubnet.EXPECT().CIDR().Return("10.10.10.10/14")

	modelOp := spaces.NewUpdateSpaceModelOp(spaceName, []spaces.MoveSubnet{s.moveSubnet})
	ops, err := modelOp.Build(0)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ops, gc.HasLen, 0)
}

func (s *SpaceUpdateSuite) TestError(c *gc.C) {
	spaceName := "spaceA"

	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	bam := errors.New("bam")
	s.moveSubnet.EXPECT().SpaceName().Return(spaceName)
	s.moveSubnet.EXPECT().UpdateSpaceOps(spaceName).Return(nil, bam)
	s.moveSubnet.EXPECT().CIDR().Return("10.10.10.10/14")
	subnets := []spaces.MoveSubnet{s.moveSubnet}
	modelOp := spaces.NewUpdateSpaceModelOp(spaceName, subnets)
	ops, err := modelOp.Build(0)

	c.Assert(err, gc.ErrorMatches, bam.Error())
	c.Assert(ops, gc.IsNil)
}
