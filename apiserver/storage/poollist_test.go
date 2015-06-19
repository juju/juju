// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	apiserverstorage "github.com/juju/juju/apiserver/storage"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/storage/provider/registry"
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
	assertPoolNames(c, pools.Results,
		"testpool0", "testpool1",
		"dummy", "loop",
		"tmpfs", "rootfs")
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
	s.registerProviders(c)
	tstType := string(provider.TmpfsProviderType)
	poolName := "rayofsunshine"
	var err error
	s.baseStorageSuite.pools[poolName], err =
		storage.NewConfig(poolName, provider.TmpfsProviderType, nil)
	c.Assert(err, jc.ErrorIsNil)

	pools, err := s.api.ListPools(params.StoragePoolFilter{
		Providers: []string{tstType}})
	c.Assert(err, jc.ErrorIsNil)
	assertPoolNames(c, pools.Results, "rayofsunshine", "tmpfs")
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
	pools, err := s.api.ListPools(params.StoragePoolFilter{
		Providers: []string{tstType},
		Names:     []string{poolName}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools.Results, gc.HasLen, 1)
	c.Assert(pools.Results[0].Provider, gc.DeepEquals, tstType)
	c.Assert(pools.Results[0].Name, gc.DeepEquals, poolName)
}

func (s *poolSuite) TestListByNamesOr(c *gc.C) {
	s.createPools(c, 2)
	s.registerProviders(c)
	poolName := "rayofsunshine"
	var err error
	s.baseStorageSuite.pools[poolName], err =
		storage.NewConfig(poolName, provider.TmpfsProviderType, nil)
	c.Assert(err, jc.ErrorIsNil)
	pools, err := s.api.ListPools(params.StoragePoolFilter{
		Names: []string{
			fmt.Sprintf("%v%v", tstName, 1),
			fmt.Sprintf("%v%v", tstName, 0),
		}})
	c.Assert(err, jc.ErrorIsNil)
	assertPoolNames(c, pools.Results, "testpool0", "testpool1")
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
	pools, err := s.api.ListPools(params.StoragePoolFilter{
		Providers: []string{tstType, string(provider.LoopProviderType)}})
	c.Assert(err, jc.ErrorIsNil)
	assertPoolNames(c, pools.Results, "testpool0", "testpool1", "rayofsunshine", "loop", "tmpfs")
}

func (s *poolSuite) TestListNoPools(c *gc.C) {
	pools, err := s.api.ListPools(params.StoragePoolFilter{})
	c.Assert(err, jc.ErrorIsNil)
	assertPoolNames(c, pools.Results, "dummy", "rootfs", "loop", "tmpfs")
}

func (s *poolSuite) TestListFilterEmpty(c *gc.C) {
	valid, err := apiserverstorage.IsValidPoolListFilter(s.api, params.StoragePoolFilter{})
	s.assertNoError(c, valid, err)
}

const (
	validProvider   = string(provider.LoopProviderType)
	invalidProvider = "invalid"
	validName       = "pool"
	invalidName     = "7ool"
)

func (s *poolSuite) TestListFilterValidProviders(c *gc.C) {
	s.registerProviders(c)
	valid, err := apiserverstorage.ValidateProviders(
		s.api,
		[]string{validProvider})
	s.assertNoError(c, valid, err)
}

func (s *poolSuite) TestListFilterUnregisteredProvider(c *gc.C) {
	s.state.envName = "noprovidersregistered"
	valid, err := apiserverstorage.ValidateProviders(
		s.api,
		[]string{validProvider})
	s.assertError(c, valid, err, ".*not supported.*")
}

func (s *poolSuite) TestListFilterUnknownProvider(c *gc.C) {
	s.registerProviders(c)
	valid, err := apiserverstorage.ValidateProviders(
		s.api,
		[]string{invalidProvider})
	s.assertError(c, valid, err, ".*not supported.*")
}

func (s *poolSuite) TestListFilterValidNames(c *gc.C) {
	valid, err := apiserverstorage.ValidateNames(
		s.api,
		[]string{validName})
	s.assertNoError(c, valid, err)
}

func (s *poolSuite) TestListFilterInvalidNames(c *gc.C) {
	valid, err := apiserverstorage.ValidateNames(
		s.api,
		[]string{invalidName})
	s.assertError(c, valid, err, ".*not valid.*")
}

func (s *poolSuite) TestListFilterValidProvidersAndNames(c *gc.C) {
	s.registerProviders(c)
	valid, err := apiserverstorage.IsValidPoolListFilter(
		s.api,
		params.StoragePoolFilter{
			Providers: []string{validProvider},
			Names:     []string{validName}})
	s.assertNoError(c, valid, err)
}

func (s *poolSuite) TestListFilterValidProvidersAndInvalidNames(c *gc.C) {
	s.registerProviders(c)
	valid, err := apiserverstorage.IsValidPoolListFilter(
		s.api,
		params.StoragePoolFilter{
			Providers: []string{validProvider},
			Names:     []string{invalidName}})
	s.assertError(c, valid, err, ".*not valid.*")
}

func (s *poolSuite) TestListFilterInvalidProvidersAndValidNames(c *gc.C) {
	valid, err := apiserverstorage.IsValidPoolListFilter(
		s.api,
		params.StoragePoolFilter{
			Providers: []string{invalidProvider},
			Names:     []string{validName}})
	s.assertError(c, valid, err, ".*not supported.*")
}

func (s *poolSuite) TestListFilterInvalidProvidersAndNames(c *gc.C) {
	valid, err := apiserverstorage.IsValidPoolListFilter(
		s.api,
		params.StoragePoolFilter{
			Providers: []string{invalidProvider},
			Names:     []string{invalidName}})
	s.assertError(c, valid, err, ".*not supported.*")
}

func (s *poolSuite) registerProviders(c *gc.C) {
	registry.RegisterEnvironStorageProviders(s.state.envName, "dummy")
}

func (s *poolSuite) assertNoError(c *gc.C, result bool, err error) {
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.IsTrue)
}

func (s *poolSuite) assertError(c *gc.C, result bool, err error, msg string) {
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(result, jc.IsFalse)
}
