// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"fmt"
	"testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apiserverstorage "github.com/juju/juju/apiserver/facades/client/storage"
	domainstorage "github.com/juju/juju/domain/storage"
	internalstorage "github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/rpc/params"
)

type poolSuite struct {
	baseStorageSuite
}

func TestPoolSuite(t *testing.T) {
	tc.Run(t, &poolSuite{})
}

const (
	tstName = "testpool"
)

func (s *poolSuite) TestEnsureStoragePoolFilter(c *tc.C) {
	defer s.setupMocks(c).Finish()

	filter := params.StoragePoolFilter{}
	c.Assert(filter.Providers, tc.HasLen, 0)
	c.Assert(apiserverstorage.EnsureStoragePoolFilter(s.apiCaas, filter).Providers, tc.DeepEquals, []string{"kubernetes"})
}

func (s *poolSuite) TestList(c *tc.C) {
	defer s.setupMocks(c).Finish()

	p, err := internalstorage.NewConfig(fmt.Sprintf("%v%v", tstName, 0), provider.LoopProviderType, nil)
	c.Assert(err, tc.ErrorIsNil)
	s.storageService.EXPECT().ListStoragePools(gomock.Any(), domainstorage.NilNames, domainstorage.NilProviders).
		Return([]*internalstorage.Config{p}, nil)

	results, err := s.api.ListPools(c.Context(), params.StoragePoolFilters{[]params.StoragePoolFilter{{}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	one := results.Results[0]
	c.Assert(one.Error, tc.IsNil)
	c.Assert(one.Result, tc.HasLen, 1)
	c.Assert(one.Result[0].Name, tc.Equals, fmt.Sprintf("%v%v", tstName, 0))
	c.Assert(one.Result[0].Provider, tc.Equals, string(provider.LoopProviderType))
}

func (s *poolSuite) TestListManyResults(c *tc.C) {
	defer s.setupMocks(c).Finish()

	p, err := internalstorage.NewConfig(fmt.Sprintf("%v%v", tstName, 0), provider.LoopProviderType, nil)
	c.Assert(err, tc.ErrorIsNil)
	p2, err := internalstorage.NewConfig(fmt.Sprintf("%v%v", tstName, 1), provider.LoopProviderType, nil)
	c.Assert(err, tc.ErrorIsNil)
	s.storageService.EXPECT().ListStoragePools(gomock.Any(), domainstorage.NilNames, domainstorage.NilProviders).
		Return([]*internalstorage.Config{p, p2}, nil)

	results, err := s.api.ListPools(c.Context(), params.StoragePoolFilters{[]params.StoragePoolFilter{{}}})
	c.Assert(err, tc.ErrorIsNil)
	assertPoolNames(c, results.Results[0].Result, "testpool0", "testpool1")
}

func assertPoolNames(c *tc.C, results []params.StoragePool, expected ...string) {
	expectedNames := set.NewStrings(expected...)
	c.Assert(len(expectedNames), tc.Equals, len(results))
	for _, one := range results {
		c.Assert(expectedNames.Contains(one.Name), tc.IsTrue)
	}
}

func (s *poolSuite) TestListNoPools(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.storageService.EXPECT().ListStoragePools(gomock.Any(), domainstorage.NilNames, domainstorage.NilProviders).
		Return([]*internalstorage.Config{}, nil)

	results, err := s.api.ListPools(c.Context(), params.StoragePoolFilters{[]params.StoragePoolFilter{{}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Result, tc.HasLen, 0)
}
