// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"context"
	"errors"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/container/lxd"
	lxdtesting "github.com/juju/juju/internal/container/lxd/testing"
)

type clusterSuite struct {
	lxdtesting.BaseSuite
}

var _ = tc.Suite(&clusterSuite{})

func (s *imageSuite) TestUseTargetGoodNode(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	c1Svr := s.NewMockServerClustered(ctrl, "cluster-1")
	c2Svr := s.NewMockServerClustered(ctrl, "cluster-2")

	c1Svr.EXPECT().UseTarget("cluster-2").Return(c2Svr)

	jujuSvr, err := lxd.NewServer(c1Svr)
	c.Assert(err, tc.ErrorIsNil)

	_, err = jujuSvr.UseTargetServer(context.Background(), "cluster-2")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *imageSuite) TestUseTargetBadNode(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	c1Svr := s.NewMockServerClustered(ctrl, "cluster-1")
	c2Svr := lxdtesting.NewMockInstanceServer(ctrl)

	c1Svr.EXPECT().UseTarget("cluster-2").Return(c2Svr)
	c2Svr.EXPECT().GetServer().Return(nil, "", errors.New("not a cluster member"))

	jujuSvr, err := lxd.NewServer(c1Svr)
	c.Assert(err, tc.ErrorIsNil)

	_, err = jujuSvr.UseTargetServer(context.Background(), "cluster-2")
	c.Assert(err, gc.ErrorMatches, "not a cluster member")
}
