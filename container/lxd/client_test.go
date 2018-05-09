// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"github.com/golang/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container/lxd"
	lxdtesting "github.com/juju/juju/container/lxd/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/lxc/lxd/shared/api"
)

type clientSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&clientSuite{})

const eTag = "etag"

func (s *connectionSuite) TestUpdateServerConfig(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := lxdtesting.NewMockContainerServer(ctrl)

	newConfig := map[string]interface{}{"key1": "val1"}
	updateReq := api.ServerPut{Config: newConfig}
	gomock.InOrder(
		cSvr.EXPECT().GetServer().Return(&api.Server{}, eTag, nil),
		cSvr.EXPECT().UpdateServer(updateReq, eTag).Return(nil),
	)

	client := lxd.NewClient(cSvr)
	err := client.UpdateServerConfig(newConfig)
	c.Assert(err, gc.IsNil)
}

func (s *connectionSuite) TestUpdateContainerConfig(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := lxdtesting.NewMockContainerServer(ctrl)

	cName := "juju-lxd-1"
	newConfig := map[string]string{"key1": "val1"}
	updateReq := api.ContainerPut{Config: newConfig}
	op := lxdtesting.NewMockOperation(ctrl)
	gomock.InOrder(
		cSvr.EXPECT().GetContainer(cName).Return(&api.Container{}, eTag, nil),
		cSvr.EXPECT().UpdateContainer(cName, updateReq, eTag).Return(op, nil),
		op.EXPECT().Wait().Return(nil),
	)

	client := lxd.NewClient(cSvr)
	err := client.UpdateContainerConfig("juju-lxd-1", newConfig)
	c.Assert(err, gc.IsNil)
}
