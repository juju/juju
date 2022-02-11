// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
)

type poolUpdateSuite struct {
	baseStorageSuite
}

var _ = gc.Suite(&poolUpdateSuite{})

func (s *poolUpdateSuite) createPools(c *gc.C, num int) {
	var err error
	for i := 0; i < num; i++ {
		poolName := fmt.Sprintf("%v%v", tstName, i)
		s.baseStorageSuite.pools[poolName], err =
			storage.NewConfig(poolName, provider.LoopProviderType, map[string]interface{}{
				"zip":  "zap",
				"beep": "boop",
			})
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *poolUpdateSuite) TestUpdatePool(c *gc.C) {
	s.createPools(c, 1)
	poolName := fmt.Sprintf("%v%v", tstName, 0)
	newAttrs := map[string]interface{}{
		"foo1": "bar1",
		"zip":  "zoom",
	}

	args := params.StoragePoolArgs{
		Pools: []params.StoragePool{{
			Name:  poolName,
			Attrs: newAttrs,
		}},
	}
	results, err := s.api.UpdatePool(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	expected, err := storage.NewConfig(poolName, provider.LoopProviderType, newAttrs)
	c.Assert(err, jc.ErrorIsNil)

	pools, err := s.poolManager.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools, gc.HasLen, 1)
	c.Assert(pools[0], gc.DeepEquals, expected)
}

func (s *poolUpdateSuite) TestUpdatePoolError(c *gc.C) {
	poolName := fmt.Sprintf("%v%v", tstName, 0)
	args := params.StoragePoolArgs{
		Pools: []params.StoragePool{{
			Name: poolName,
		}},
	}
	results, err := s.api.UpdatePool(args)
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, jc.DeepEquals, &params.Error{
		Message: "mock pool manager: get pool testpool0 not found",
		Code:    "not found",
	})
}
