// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"
	"fmt"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/client/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	internalstorage "github.com/juju/juju/internal/storage"
	"github.com/juju/juju/rpc/params"
)

type poolUpdateSuite struct {
	baseStorageSuite
}

var _ = gc.Suite(&poolUpdateSuite{})

func (s *poolUpdateSuite) TestUpdatePool(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.storageService = storage.NewMockStorageService(ctrl)
	poolName := fmt.Sprintf("%v%v", tstName, 0)
	newAttrs := map[string]interface{}{
		"foo1": "bar1",
		"zip":  "zoom",
	}
	s.storageService.EXPECT().ReplaceStoragePool(gomock.Any(), poolName, internalstorage.ProviderType(""), newAttrs).Return(nil)

	args := params.StoragePoolArgs{
		Pools: []params.StoragePool{{
			Name:  poolName,
			Attrs: newAttrs,
		}},
	}
	results, err := s.api.UpdatePool(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
}

func (s *poolUpdateSuite) TestUpdatePoolError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.storageService = storage.NewMockStorageService(ctrl)
	poolName := fmt.Sprintf("%v%v", tstName, 0)
	args := params.StoragePoolArgs{
		Pools: []params.StoragePool{{
			Name: poolName,
		}},
	}
	s.storageService.EXPECT().ReplaceStoragePool(gomock.Any(), poolName, internalstorage.ProviderType(""), nil).Return(storageerrors.PoolNotFoundError)

	results, err := s.api.UpdatePool(context.Background(), args)
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, "storage pool is not found")
}
