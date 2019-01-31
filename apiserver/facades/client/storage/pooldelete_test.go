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

func (s *poolDeleteSuite) TestDeletePool(c *gc.C) {
	s.createPools(c, 1)
	poolName := fmt.Sprintf("%v%v", tstName, 0)

	err := s.api.DeletePool(params.StoragePoolDeleteArg{
		Name: poolName,
	})
	c.Assert(err, jc.ErrorIsNil)

	pools, err := s.poolManager.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools, gc.HasLen, 0)
}

func (s *poolDeleteSuite) TestDeleteNotExists(c *gc.C) {
	poolName := fmt.Sprintf("%v%v", tstName, 0)

	err := s.api.DeletePool(params.StoragePoolDeleteArg{
		Name: poolName,
	})
	c.Assert(err, jc.ErrorIsNil)

	pools, err := s.poolManager.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools, gc.HasLen, 0)
}
