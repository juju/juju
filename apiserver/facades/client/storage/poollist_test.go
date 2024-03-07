// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"
	"fmt"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/client/storage"
	apiserverstorage "github.com/juju/juju/apiserver/facades/client/storage"
	domainstorage "github.com/juju/juju/domain/storage"
	internalstorage "github.com/juju/juju/internal/storage"
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

func (s *poolSuite) TestEnsureStoragePoolFilter(c *gc.C) {
	filter := params.StoragePoolFilter{}
	c.Assert(filter.Providers, gc.HasLen, 0)
	c.Assert(apiserverstorage.EnsureStoragePoolFilter(s.apiCaas, filter).Providers, jc.DeepEquals, []string{"kubernetes"})
}

func (s *poolSuite) TestList(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.storageService = storage.NewMockStorageService(ctrl)

	p, err := internalstorage.NewConfig(fmt.Sprintf("%v%v", tstName, 0), provider.LoopProviderType, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.storageService.EXPECT().ListStoragePools(gomock.Any(), domainstorage.StoragePoolFilter{}).
		Return([]*internalstorage.Config{p}, nil)

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
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.storageService = storage.NewMockStorageService(ctrl)

	p, err := internalstorage.NewConfig(fmt.Sprintf("%v%v", tstName, 0), provider.LoopProviderType, nil)
	c.Assert(err, jc.ErrorIsNil)
	p2, err := internalstorage.NewConfig(fmt.Sprintf("%v%v", tstName, 1), provider.LoopProviderType, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.storageService.EXPECT().ListStoragePools(gomock.Any(), domainstorage.StoragePoolFilter{}).
		Return([]*internalstorage.Config{p, p2}, nil)

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
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.storageService = storage.NewMockStorageService(ctrl)
	s.storageService.EXPECT().ListStoragePools(gomock.Any(), domainstorage.StoragePoolFilter{}).
		Return([]*internalstorage.Config{}, nil)

	results, err := s.api.ListPools(context.Background(), params.StoragePoolFilters{[]params.StoragePoolFilter{{}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Result, gc.HasLen, 0)
}
