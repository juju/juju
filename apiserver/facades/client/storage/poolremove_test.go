// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/rpc/params"
)

type poolRemoveSuite struct {
	baseStorageSuite
}

var _ = gc.Suite(&poolRemoveSuite{})

func (s *poolRemoveSuite) createPools(c *gc.C, num int) {
	var err error
	for i := 0; i < num; i++ {
		poolName := fmt.Sprintf("%v%v", tstName, i)
		s.baseStorageSuite.pools[poolName], err =
			storage.NewConfig(poolName, provider.LoopProviderType, map[string]interface{}{"zip": "zap"})
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *poolRemoveSuite) TestRemovePool(c *gc.C) {
	s.createPools(c, 1)
	poolName := fmt.Sprintf("%v%v", tstName, 0)

	args := params.StoragePoolDeleteArgs{
		Pools: []params.StoragePoolDeleteArg{{
			Name: poolName,
		}},
	}
	results, err := s.api.RemovePool(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	pools, err := s.poolManager.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools, gc.HasLen, 0)
}

func (s *poolRemoveSuite) TestRemoveNotExists(c *gc.C) {
	poolName := fmt.Sprintf("%v%v", tstName, 0)

	args := params.StoragePoolDeleteArgs{
		Pools: []params.StoragePoolDeleteArg{{
			Name: poolName,
		}},
	}
	results, err := s.api.RemovePool(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	pools, err := s.poolManager.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools, gc.HasLen, 0)
}

func (s *poolRemoveSuite) TestRemoveInUse(c *gc.C) {
	s.createPools(c, 1)
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

	pools, err := s.poolManager.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools, gc.HasLen, 1)
}

func (s *poolRemoveSuite) TestRemoveSomeInUse(c *gc.C) {
	s.createPools(c, 2)
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

	pools, err := s.poolManager.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools, gc.HasLen, 1)
}
