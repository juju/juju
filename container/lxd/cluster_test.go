// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"github.com/golang/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container/lxd"
	lxdtesting "github.com/juju/juju/container/lxd/testing"
)

type clusterSuite struct {
	lxdtesting.BaseSuite
}

var _ = gc.Suite(&clusterSuite{})

func (s *imageSuite) TestUseTarget(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	c1Svr := s.NewMockServerClustered(ctrl, "cluster-1")
	c2Svr := s.NewMockServerClustered(ctrl, "cluster-2")

	c1Svr.EXPECT().UseTarget("cluster-2").Return(c2Svr)

	lxd.NewServer(c1Svr).UseTargetServer("cluster-2")
}
