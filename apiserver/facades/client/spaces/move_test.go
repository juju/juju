// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/juju/core/network"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/apiserver/facades/client/spaces"
	"github.com/juju/juju/apiserver/facades/client/spaces/mocks"
)

type SpaceUpdateSuite struct {
	subnet *mocks.MockUpdateSubnet
}

var _ = gc.Suite(&SpaceUpdateSuite{})

func (s *SpaceUpdateSuite) TearDownTest(c *gc.C) {
}

func (s *SpaceUpdateSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.subnet = mocks.NewMockUpdateSubnet(ctrl)

	return ctrl
}

func (s *SpaceUpdateSuite) TestSuccess(c *gc.C) {
	spaceName := "1"

	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	expectedNetworkInfo := network.SubnetInfo{SpaceName: spaceName}
	s.subnet.EXPECT().UpdateOps(expectedNetworkInfo).Return([]txn.Op{}, nil)
	s.subnet.EXPECT().SpaceName().Return("spaceA")
	s.subnet.EXPECT().CIDR().Return("10.10.10.10/14")

	modelOp := spaces.NewUpdateSpaceModelOp(spaceName, []spaces.UpdateSubnet{s.subnet})
	ops, err := modelOp.Build(0)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ops, gc.HasLen, 0)
}

func (s *SpaceUpdateSuite) TestError(c *gc.C) {
	spaceName := "1"

	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	expectedNetworkInfo := network.SubnetInfo{SpaceName: spaceName}
	bam := errors.New("bam")
	s.subnet.EXPECT().UpdateOps(expectedNetworkInfo).Return(nil, bam)
	s.subnet.EXPECT().SpaceName().Return("spaceA")
	s.subnet.EXPECT().CIDR().Return("10.10.10.10/14")
	modelOp := spaces.NewUpdateSpaceModelOp(spaceName, []spaces.UpdateSubnet{s.subnet})
	ops, err := modelOp.Build(0)

	c.Assert(err, gc.ErrorMatches, bam.Error())
	c.Assert(ops, gc.IsNil)
}
