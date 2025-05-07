// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"
	"fmt"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/rpc/params"
)

type poolRemoveSuite struct {
	baseStorageSuite
}

var _ = tc.Suite(&poolRemoveSuite{})

func (s *poolRemoveSuite) TestRemovePool(c *tc.C) {
	defer s.setupMocks(c).Finish()

	poolName := fmt.Sprintf("%v%v", tstName, 0)
	s.storageService.EXPECT().DeleteStoragePool(gomock.Any(), poolName)

	args := params.StoragePoolDeleteArgs{
		Pools: []params.StoragePoolDeleteArg{{
			Name: poolName,
		}},
	}
	results, err := s.api.RemovePool(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
}

func (s *poolRemoveSuite) TestRemoveNotExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	poolName := fmt.Sprintf("%v%v", tstName, 0)
	s.storageService.EXPECT().DeleteStoragePool(gomock.Any(), poolName).Return(storageerrors.PoolNotFoundError)

	args := params.StoragePoolDeleteArgs{
		Pools: []params.StoragePoolDeleteArg{{
			Name: poolName,
		}},
	}
	results, err := s.api.RemovePool(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, "storage pool is not found")
}

func (s *poolRemoveSuite) TestRemoveInUse(c *tc.C) {
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
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, fmt.Sprintf("storage pool %q in use", poolName))

	pools, err := s.storageService.ListStoragePools(context.Background(), domainstorage.NilNames, domainstorage.NilProviders)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools, tc.HasLen, 1)
}

func (s *poolRemoveSuite) TestRemoveSomeInUse(c *tc.C) {
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
	c.Assert(results.Results, tc.HasLen, 2)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, fmt.Sprintf("storage pool %q in use", poolNameInUse))
	c.Assert(results.Results[1].Error, tc.IsNil)

	pools, err := s.storageService.ListStoragePools(context.Background(), domainstorage.NilNames, domainstorage.NilProviders)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools, tc.HasLen, 1)
}
