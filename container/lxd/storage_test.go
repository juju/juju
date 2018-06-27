// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	lxdapi "github.com/lxc/lxd/shared/api"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container/lxd"
	lxdtesting "github.com/juju/juju/container/lxd/testing"
)

type storageSuite struct {
	lxdtesting.BaseSuite
}

var _ = gc.Suite(&storageSuite{})

func (s *storageSuite) TestCreatePool(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServerWithExtensions(ctrl, "storage")

	cfg := map[string]string{"size": "1024MB"}

	req := lxdapi.StoragePoolsPost{
		Name:   "new-pool",
		Driver: "dir",
		StoragePoolPut: lxdapi.StoragePoolPut{
			Config: cfg,
		},
	}
	cSvr.EXPECT().CreateStoragePool(req).Return(nil)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	err = jujuSvr.CreatePool("new-pool", "dir", cfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storageSuite) TestCreateVolume(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServerWithExtensions(ctrl, "storage")

	cfg := map[string]string{"size": "1024MB"}

	req := lxdapi.StorageVolumesPost{
		Name: "volume",
		Type: "custom",
		StorageVolumePut: lxdapi.StorageVolumePut{
			Config: cfg,
		},
	}
	cSvr.EXPECT().CreateStoragePoolVolume("default-pool", req).Return(nil)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	err = jujuSvr.CreateVolume("default-pool", "volume", cfg)
	c.Assert(err, jc.ErrorIsNil)
}
