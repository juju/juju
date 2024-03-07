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
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/rpc/params"
)

type poolRemoveSuite struct {
	baseStorageSuite
}

var _ = gc.Suite(&poolRemoveSuite{})

func (s *poolRemoveSuite) TestRemovePool(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.storageService = storage.NewMockStorageService(ctrl)
	poolName := fmt.Sprintf("%v%v", tstName, 0)
	s.storageService.EXPECT().DeleteStoragePool(gomock.Any(), poolName)

	args := params.StoragePoolDeleteArgs{
		Pools: []params.StoragePoolDeleteArg{{
			Name: poolName,
		}},
	}
	results, err := s.api.RemovePool(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
}

func (s *poolRemoveSuite) TestRemoveNotExists(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.storageService = storage.NewMockStorageService(ctrl)
	poolName := fmt.Sprintf("%v%v", tstName, 0)
	s.storageService.EXPECT().DeleteStoragePool(gomock.Any(), poolName).Return(storageerrors.PoolNotFoundError)

	args := params.StoragePoolDeleteArgs{
		Pools: []params.StoragePoolDeleteArg{{
			Name: poolName,
		}},
	}
	results, err := s.api.RemovePool(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, "storage pool is not found")
}

func (s *poolRemoveSuite) TestRemoveInUse(c *gc.C) {
	c.Skip("TODO(storage) - support storage pool in-use checks")
	poolName := fmt.Sprintf("%v%v", tstName, 0)
	s.poolsInUse = []string{poolName}
	args := params.StoragePoolDeleteArgs{
		Pools: []params.StoragePoolDeleteArg{{
			Name: poolName,
		}},
	}
	results, err := s.api.RemovePool(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, fmt.Sprintf("storage pool %q in use", poolName))

	pools, err := s.storageService.ListStoragePools(context.Background(), domainstorage.NilNames, domainstorage.NilProviders)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools, gc.HasLen, 1)
}

func (s *poolRemoveSuite) TestRemoveSomeInUse(c *gc.C) {
	c.Skip("TODO(storage) - support storage pool in-use checks")
	poolNameInUse := fmt.Sprintf("%v%v", tstName, 0)
	poolNameNotInUse := fmt.Sprintf("%v%v", tstName, 1)
	s.poolsInUse = []string{poolNameInUse}
	args := params.StoragePoolDeleteArgs{
		Pools: []params.StoragePoolDeleteArg{{
			Name: poolNameInUse,
		}, {
			Name: poolNameNotInUse,
		}},
	}
	results, err := s.api.RemovePool(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, fmt.Sprintf("storage pool %q in use", poolNameInUse))
	c.Assert(results.Results[1].Error, gc.IsNil)

	pools, err := s.storageService.ListStoragePools(context.Background(), domainstorage.NilNames, domainstorage.NilProviders)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools, gc.HasLen, 1)
}
