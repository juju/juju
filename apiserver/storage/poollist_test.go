// Copyright 2015 Canonical Ltd.
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

type poolSuite struct {
	baseStorageSuite
}

var _ = gc.Suite(&poolSuite{})

const (
	tstName = "testpool"
)

func (s *poolSuite) createPools(c *gc.C, num int) {
	var err error
	for i := 0; i < num; i++ {
		poolName := fmt.Sprintf("%v%v", tstName, i)
		s.baseStorageSuite.pools[poolName], err =
			storage.NewConfig(poolName, provider.LoopProviderType, nil)
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *poolSuite) TestList(c *gc.C) {
	s.createPools(c, 1)
	pools, err := s.api.ListPools(params.StoragePoolFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools.Results, gc.HasLen, 1)
	one := pools.Results[0]
	c.Assert(one.Name, gc.Equals, fmt.Sprintf("%v%v", tstName, 0))
	c.Assert(one.Provider, gc.Equals, string(provider.LoopProviderType))
}

func (s *poolSuite) TestListManyResults(c *gc.C) {
	s.createPools(c, 2)
	pools, err := s.api.ListPools(params.StoragePoolFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools.Results, gc.HasLen, 2)
}

func (s *poolSuite) TestListByName(c *gc.C) {
	s.createPools(c, 2)
	tstName := fmt.Sprintf("%v%v", tstName, 1)

	pools, err := s.api.ListPools(params.StoragePoolFilter{
		Names: []string{tstName}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools.Results, gc.HasLen, 1)
	c.Assert(pools.Results[0].Name, gc.DeepEquals, tstName)
}

func (s *poolSuite) TestListByType(c *gc.C) {
	s.createPools(c, 2)
	tstType := string(provider.HostLoopProviderType)
	poolName := "rayofsunshine"
	var err error
	s.baseStorageSuite.pools[poolName], err =
		storage.NewConfig(poolName, provider.HostLoopProviderType, nil)
	c.Assert(err, jc.ErrorIsNil)
	pools, err := s.api.ListPools(params.StoragePoolFilter{
		Providers: []string{tstType}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools.Results, gc.HasLen, 1)
	c.Assert(pools.Results[0].Provider, gc.DeepEquals, tstType)
	c.Assert(pools.Results[0].Name, gc.DeepEquals, poolName)
}

func (s *poolSuite) TestListByNameAndTypeAnd(c *gc.C) {
	s.createPools(c, 2)
	tstType := string(provider.HostLoopProviderType)
	poolName := "rayofsunshine"
	var err error
	s.baseStorageSuite.pools[poolName], err =
		storage.NewConfig(poolName, provider.HostLoopProviderType, nil)
	c.Assert(err, jc.ErrorIsNil)
	pools, err := s.api.ListPools(params.StoragePoolFilter{
		Providers: []string{tstType},
		Names:     []string{poolName}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools.Results, gc.HasLen, 1)
	c.Assert(pools.Results[0].Provider, gc.DeepEquals, tstType)
	c.Assert(pools.Results[0].Name, gc.DeepEquals, poolName)
}

func (s *poolSuite) TestListByNameAndTypeOr(c *gc.C) {
	s.createPools(c, 2)
	tstType := string(provider.HostLoopProviderType)
	poolName := "rayofsunshine"
	var err error
	s.baseStorageSuite.pools[poolName], err =
		storage.NewConfig(poolName, provider.HostLoopProviderType, nil)
	c.Assert(err, jc.ErrorIsNil)
	pools, err := s.api.ListPools(params.StoragePoolFilter{
		Providers: []string{tstType},
		Names:     []string{fmt.Sprintf("%v%v", tstName, 1)}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(pools.Results) < len(s.pools), jc.IsTrue)
}

func (s *poolSuite) TestListNoPools(c *gc.C) {
	pools, err := s.api.ListPools(params.StoragePoolFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools.Results, gc.HasLen, 0)
}
