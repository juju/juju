// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
)

type poolDeleteSuite struct {
	baseStorageSuite
}

var _ = gc.Suite(&poolDeleteSuite{})

func (s *poolDeleteSuite) createPools(c *gc.C, num int) {
	var err error
	for i := 0; i < num; i++ {
		poolName := fmt.Sprintf("%v%v", tstName, i)
		s.baseStorageSuite.pools[poolName], err =
			storage.NewConfig(poolName, provider.LoopProviderType, map[string]interface{}{"zip": "zap"})
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *poolDeleteSuite) createOperatorStorage(c *gc.C) {
	poolName := "operator-storage"
	pool, err := storage.NewConfig(poolName, provider.LoopProviderType, map[string]interface{}{"zip": "zap"})
	c.Assert(err, jc.ErrorIsNil)
	s.baseStorageSuite.pools[poolName] = pool
}

func (s *poolDeleteSuite) TestRemovePool(c *gc.C) {
	s.createPools(c, 1)
	poolName := fmt.Sprintf("%v%v", tstName, 0)

	args := params.StoragePoolDeleteArgs{
		Pools: []params.StoragePoolDeleteArg{{
			Name: poolName,
		}},
	}
	results, err := s.api.RemovePool(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	pools, err := s.poolManager.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools, gc.HasLen, 0)
}

func (s *poolDeleteSuite) TestDeleteNotExists(c *gc.C) {
	poolName := fmt.Sprintf("%v%v", tstName, 0)

	args := params.StoragePoolDeleteArgs{
		Pools: []params.StoragePoolDeleteArg{{
			Name: poolName,
		}},
	}
	results, err := s.api.RemovePool(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	pools, err := s.poolManager.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools, gc.HasLen, 0)
}

func (s *poolDeleteSuite) TestDeleteInUse(c *gc.C) {
	s.createPools(c, 1)
	poolName := fmt.Sprintf("%v%v", tstName, 0)
	s.poolsInUse = []string{poolName}
	args := params.StoragePoolDeleteArgs{
		Pools: []params.StoragePoolDeleteArg{{
			Name: poolName,
		}},
	}
	results, err := s.api.DeletePool(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, fmt.Sprintf("pool %q in use", poolName))

	pools, err := s.poolManager.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools, gc.HasLen, 1)
}

func (s *poolDeleteSuite) TestDeleteSomeInUse(c *gc.C) {
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
	results, err := s.api.DeletePool(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, fmt.Sprintf("pool %q in use", poolNameInUse))
	c.Assert(results.Results[1].Error, gc.IsNil)

	pools, err := s.poolManager.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools, gc.HasLen, 1)
}

func (s *poolDeleteSuite) TestDeleteOperatorStorageInUse(c *gc.C) {
	s.createPools(c, 1)
	s.createOperatorStorage(c)
	s.state.applicationNames = []string{"mariadb"}
	poolNameInUse := "operator-storage"
	// operator-storage is not included in the results of the StoragePoolsInUse call.
	args := params.StoragePoolDeleteArgs{
		Pools: []params.StoragePoolDeleteArg{{
			Name: poolNameInUse,
		}},
	}
	results, err := s.apiCaas.DeletePool(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, fmt.Sprintf("pool %q in use", poolNameInUse))

	pools, err := s.poolManager.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools, gc.HasLen, 2)
}

func (s *poolDeleteSuite) TestDeleteOperatorStorageNotInUse(c *gc.C) {
	s.createPools(c, 1)
	s.createOperatorStorage(c)
	poolNameInUse := "operator-storage"
	// operator-storage is not included in the results of the StoragePoolsInUse call.
	args := params.StoragePoolDeleteArgs{
		Pools: []params.StoragePoolDeleteArg{{
			Name: poolNameInUse,
		}},
	}
	results, err := s.apiCaas.DeletePool(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	pools, err := s.poolManager.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools, gc.HasLen, 1)
}

func (s *poolDeleteSuite) TestDeleteOperatorStorageIgnoresIAAS(c *gc.C) {
	s.createPools(c, 1)
	s.createOperatorStorage(c)
	s.state.applicationNames = []string{"mariadb"}
	poolNameInUse := "operator-storage"
	// operator-storage is not included in the results of the StoragePoolsInUse call.
	args := params.StoragePoolDeleteArgs{
		Pools: []params.StoragePoolDeleteArg{{
			Name: poolNameInUse,
		}},
	}
	results, err := s.api.DeletePool(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	pools, err := s.poolManager.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools, gc.HasLen, 1)
}
