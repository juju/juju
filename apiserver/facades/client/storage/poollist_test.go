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

func (s *poolSuite) TestListByNames(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.storageService.EXPECT().ListStoragePoolsByNames(gomock.Any(),
		domainstorage.Names{
			fmt.Sprintf("%v%v", tstName, 0),
		},
	).Return([]domainstorage.StoragePool{
		{
			Name:     fmt.Sprintf("%v%v", tstName, 0),
			Provider: string(provider.LoopProviderType),
		},
	}, nil)

	results, err := s.api.ListPools(c.Context(), params.StoragePoolFilters{
		Filters: []params.StoragePoolFilter{{
			Names: []string{fmt.Sprintf("%v%v", tstName, 0)},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	one := results.Results[0]
	c.Assert(one.Error, tc.IsNil)
	c.Assert(one.Result, tc.HasLen, 1)
	c.Assert(one.Result[0].Name, tc.Equals, fmt.Sprintf("%v%v", tstName, 0))
	c.Assert(one.Result[0].Provider, tc.Equals, string(provider.LoopProviderType))
}

func (s *poolSuite) TestListByProviders(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.storageService.EXPECT().ListStoragePoolsByProviders(gomock.Any(),
		domainstorage.Providers{
			string(provider.LoopProviderType),
		},
	).Return([]domainstorage.StoragePool{
		{
			Name:     fmt.Sprintf("%v%v", tstName, 0),
			Provider: string(provider.LoopProviderType),
		},
	}, nil)

	results, err := s.api.ListPools(c.Context(), params.StoragePoolFilters{
		Filters: []params.StoragePoolFilter{{
			Providers: []string{string(provider.LoopProviderType)},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	one := results.Results[0]
	c.Assert(one.Error, tc.IsNil)
	c.Assert(one.Result, tc.HasLen, 1)
	c.Assert(one.Result[0].Name, tc.Equals, fmt.Sprintf("%v%v", tstName, 0))
	c.Assert(one.Result[0].Provider, tc.Equals, string(provider.LoopProviderType))
}

func (s *poolSuite) TestList(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.storageService.EXPECT().ListStoragePoolsByNamesAndProviders(gomock.Any(),
		domainstorage.Names{
			fmt.Sprintf("%v%v", tstName, 0),
		}, domainstorage.Providers{
			string(provider.LoopProviderType),
		},
	).Return([]domainstorage.StoragePool{
		{
			Name:     fmt.Sprintf("%v%v", tstName, 0),
			Provider: string(provider.LoopProviderType),
		},
	}, nil)

	results, err := s.api.ListPools(c.Context(), params.StoragePoolFilters{
		Filters: []params.StoragePoolFilter{{
			Names:     []string{fmt.Sprintf("%v%v", tstName, 0)},
			Providers: []string{string(provider.LoopProviderType)},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	one := results.Results[0]
	c.Assert(one.Error, tc.IsNil)
	c.Assert(one.Result, tc.HasLen, 1)
	c.Assert(one.Result[0].Name, tc.Equals, fmt.Sprintf("%v%v", tstName, 0))
	c.Assert(one.Result[0].Provider, tc.Equals, string(provider.LoopProviderType))
}

func (s *poolSuite) TestListAll(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.storageService.EXPECT().ListStoragePools(gomock.Any()).
		Return([]domainstorage.StoragePool{
			{
				Name:     fmt.Sprintf("%v%v", tstName, 0),
				Provider: string(provider.LoopProviderType),
			},
			{
				Name:     fmt.Sprintf("%v%v", tstName, 1),
				Provider: string(provider.LoopProviderType),
			},
		}, nil)

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

	s.storageService.EXPECT().ListStoragePools(gomock.Any()).
		Return([]domainstorage.StoragePool{}, nil)

	results, err := s.api.ListPools(c.Context(), params.StoragePoolFilters{[]params.StoragePoolFilter{{}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Result, tc.HasLen, 0)
}
