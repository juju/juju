// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"
	"fmt"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiserverstorage "github.com/juju/juju/apiserver/facades/client/storage"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/rpc/params"
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

func (s *poolSuite) TestEnsureStoragePoolFilter(c *gc.C) {
	filter := params.StoragePoolFilter{}
	c.Assert(filter.Providers, gc.HasLen, 0)
	c.Assert(apiserverstorage.EnsureStoragePoolFilter(s.apiCaas, filter).Providers, jc.DeepEquals, []string{"kubernetes"})
}

func (s *poolSuite) TestList(c *gc.C) {
	s.createPools(c, 1)
	results, err := s.api.ListPools(context.Background(), params.StoragePoolFilters{[]params.StoragePoolFilter{{}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	one := results.Results[0]
	c.Assert(one.Error, gc.IsNil)
	c.Assert(one.Result, gc.HasLen, 1)
	c.Assert(one.Result[0].Name, gc.Equals, fmt.Sprintf("%v%v", tstName, 0))
	c.Assert(one.Result[0].Provider, gc.Equals, string(provider.LoopProviderType))
}

func (s *poolSuite) TestListManyResults(c *gc.C) {
	s.createPools(c, 2)
	results, err := s.api.ListPools(context.Background(), params.StoragePoolFilters{[]params.StoragePoolFilter{{}}})
	c.Assert(err, jc.ErrorIsNil)
	assertPoolNames(c, results.Results[0].Result, "testpool0", "testpool1")
}

func assertPoolNames(c *gc.C, results []params.StoragePool, expected ...string) {
	expectedNames := set.NewStrings(expected...)
	c.Assert(len(expectedNames), gc.Equals, len(results))
	for _, one := range results {
		c.Assert(expectedNames.Contains(one.Name), jc.IsTrue)
	}
}

func (s *poolSuite) TestListNoPools(c *gc.C) {
	results, err := s.api.ListPools(context.Background(), params.StoragePoolFilters{[]params.StoragePoolFilter{{}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Result, gc.HasLen, 0)
}
