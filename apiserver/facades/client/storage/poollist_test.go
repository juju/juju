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
	"github.com/juju/juju/rpc/params"
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
	s.registry.Providers["static"] = nil
	s.createPools(c, 2)
	results, err := s.api.ListPools(context.Background(), params.StoragePoolFilters{[]params.StoragePoolFilter{{}}})
	c.Assert(err, jc.ErrorIsNil)
	assertPoolNames(c, results.Results[0].Result, "testpool0", "testpool1", "static")
}

func (s *poolSuite) TestListByName(c *gc.C) {
	s.createPools(c, 2)
	tstName := fmt.Sprintf("%v%v", tstName, 1)

	results, err := s.api.ListPools(context.Background(), params.StoragePoolFilters{
		[]params.StoragePoolFilter{{
			Names: []string{tstName},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Result, gc.HasLen, 1)
	c.Assert(results.Results[0].Result[0].Name, gc.DeepEquals, tstName)
}

func (s *poolSuite) TestListByType(c *gc.C) {
	s.createPools(c, 2)
	s.registerProviders(c)
	tstType := string(provider.TmpfsProviderType)
	poolName := "rayofsunshine"
	var err error
	s.baseStorageSuite.pools[poolName], err =
		storage.NewConfig(poolName, provider.TmpfsProviderType, nil)
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.api.ListPools(context.Background(), params.StoragePoolFilters{
		[]params.StoragePoolFilter{{
			Providers: []string{tstType},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	assertPoolNames(c, results.Results[0].Result, "rayofsunshine", "tmpfs")
}

func (s *poolSuite) TestListByNameAndTypeAnd(c *gc.C) {
	s.createPools(c, 2)
	s.registerProviders(c)
	tstType := string(provider.TmpfsProviderType)
	poolName := "rayofsunshine"
	var err error
	s.baseStorageSuite.pools[poolName], err =
		storage.NewConfig(poolName, provider.TmpfsProviderType, nil)
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.api.ListPools(context.Background(), params.StoragePoolFilters{
		[]params.StoragePoolFilter{{
			Providers: []string{tstType},
			Names:     []string{poolName},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Result, gc.HasLen, 1)
	c.Assert(results.Results[0].Result[0].Provider, gc.DeepEquals, tstType)
	c.Assert(results.Results[0].Result[0].Name, gc.DeepEquals, poolName)
}

func (s *poolSuite) TestListByNamesOr(c *gc.C) {
	s.createPools(c, 2)
	s.registerProviders(c)
	poolName := "rayofsunshine"
	var err error
	s.baseStorageSuite.pools[poolName], err =
		storage.NewConfig(poolName, provider.TmpfsProviderType, nil)
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.api.ListPools(context.Background(), params.StoragePoolFilters{
		[]params.StoragePoolFilter{{
			Names: []string{
				fmt.Sprintf("%v%v", tstName, 1),
				fmt.Sprintf("%v%v", tstName, 0),
			},
		}},
	})
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

func (s *poolSuite) TestListByTypesOr(c *gc.C) {
	s.createPools(c, 2)
	s.registerProviders(c)
	tstType := string(provider.TmpfsProviderType)
	poolName := "rayofsunshine"
	var err error
	s.baseStorageSuite.pools[poolName], err =
		storage.NewConfig(poolName, provider.TmpfsProviderType, nil)
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.api.ListPools(context.Background(), params.StoragePoolFilters{
		[]params.StoragePoolFilter{{
			Providers: []string{tstType, string(provider.LoopProviderType)},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	assertPoolNames(c, results.Results[0].Result, "testpool0", "testpool1", "rayofsunshine", "loop", "tmpfs")
}

func (s *poolSuite) TestListNoPools(c *gc.C) {
	s.registry.Providers["static"] = nil
	results, err := s.api.ListPools(context.Background(), params.StoragePoolFilters{[]params.StoragePoolFilter{{}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	assertPoolNames(c, results.Results[0].Result, "static")
}

func (s *poolSuite) TestListFilterEmpty(c *gc.C) {
	err := apiserverstorage.ValidatePoolListFilter(s.api, s.registry, params.StoragePoolFilter{})
	c.Assert(err, jc.ErrorIsNil)
}

const (
	validProvider   = string(provider.LoopProviderType)
	invalidProvider = "invalid"
	validName       = "pool"
	invalidName     = "7ool"
)

func (s *poolSuite) TestListFilterValidProviders(c *gc.C) {
	s.registerProviders(c)
	err := apiserverstorage.ValidateProviderCriteria(
		s.api,
		s.registry,
		[]string{validProvider})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *poolSuite) TestListFilterUnregisteredProvider(c *gc.C) {
	err := apiserverstorage.ValidateProviderCriteria(
		s.api,
		s.registry,
		[]string{validProvider})
	c.Assert(err, gc.ErrorMatches, `storage provider "loop" not found`)
}

func (s *poolSuite) TestListFilterUnknownProvider(c *gc.C) {
	s.registerProviders(c)
	err := apiserverstorage.ValidateProviderCriteria(
		s.api,
		s.registry,
		[]string{invalidProvider})
	c.Assert(err, gc.ErrorMatches, `storage provider "invalid" not found`)
}

func (s *poolSuite) TestListFilterValidNames(c *gc.C) {
	err := apiserverstorage.ValidateNameCriteria(
		s.api,
		[]string{validName})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *poolSuite) TestListFilterInvalidNames(c *gc.C) {
	err := apiserverstorage.ValidateNameCriteria(
		s.api,
		[]string{invalidName})
	c.Assert(err, gc.ErrorMatches, ".*not valid.*")
}

func (s *poolSuite) TestListFilterValidProvidersAndNames(c *gc.C) {
	s.registerProviders(c)
	err := apiserverstorage.ValidatePoolListFilter(
		s.api,
		s.registry,
		params.StoragePoolFilter{
			Providers: []string{validProvider},
			Names:     []string{validName}})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *poolSuite) TestListFilterValidProvidersAndInvalidNames(c *gc.C) {
	s.registerProviders(c)
	err := apiserverstorage.ValidatePoolListFilter(
		s.api,
		s.registry,
		params.StoragePoolFilter{
			Providers: []string{validProvider},
			Names:     []string{invalidName}})
	c.Assert(err, gc.ErrorMatches, ".*not valid.*")
}

func (s *poolSuite) TestListFilterInvalidProvidersAndValidNames(c *gc.C) {
	err := apiserverstorage.ValidatePoolListFilter(
		s.api,
		s.registry,
		params.StoragePoolFilter{
			Providers: []string{invalidProvider},
			Names:     []string{validName}})
	c.Assert(err, gc.ErrorMatches, `storage provider "invalid" not found`)
}

func (s *poolSuite) TestListFilterInvalidProvidersAndNames(c *gc.C) {
	err := apiserverstorage.ValidatePoolListFilter(
		s.api,
		s.registry,
		params.StoragePoolFilter{
			Providers: []string{invalidProvider},
			Names:     []string{invalidName}})
	c.Assert(err, gc.ErrorMatches, `storage provider "invalid" not found`)
}

func (s *poolSuite) registerProviders(c *gc.C) {
	common := provider.CommonStorageProviders()
	providerTypes, err := common.StorageProviderTypes()
	c.Assert(err, jc.ErrorIsNil)
	for _, providerType := range providerTypes {
		p, err := common.StorageProvider(providerType)
		c.Assert(err, jc.ErrorIsNil)
		s.registry.Providers[providerType] = p
	}
}
