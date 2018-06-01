// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container/lxd"
	lxdtesting "github.com/juju/juju/container/lxd/testing"
	"github.com/pkg/errors"
)

type clusterSuite struct {
	lxdtesting.BaseSuite
}

var _ = gc.Suite(&clusterSuite{})

func (s *imageSuite) TestUseTargetGoodNode(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	c1Svr := s.NewMockServerClustered(ctrl, "cluster-1")
	c2Svr := s.NewMockServerClustered(ctrl, "cluster-2")

	c1Svr.EXPECT().UseTarget("cluster-2").Return(c2Svr)

	jujuSvr, err := lxd.NewServer(c1Svr)
	c.Assert(err, jc.ErrorIsNil)

	_, err = jujuSvr.UseTargetServer("cluster-2")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *imageSuite) TestUseTargetBadNode(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	c1Svr := s.NewMockServerClustered(ctrl, "cluster-1")
	c2Svr := lxdtesting.NewMockContainerServer(ctrl)

	c1Svr.EXPECT().UseTarget("cluster-2").Return(c2Svr)
	c2Svr.EXPECT().GetServer().Return(nil, "", errors.New("not a cluster member"))

	jujuSvr, err := lxd.NewServer(c1Svr)
	c.Assert(err, jc.ErrorIsNil)

	_, err = jujuSvr.UseTargetServer("cluster-2")
	c.Assert(err, gc.ErrorMatches, "not a cluster member")
}
