// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"fmt"
	"testing"

	"github.com/juju/collections/set"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apiservertesting "github.com/juju/juju/apiserver/testing"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/rpc/params"
)

// poolListSuite is test suite to assert the contracts offered by the facade for
// listing storage pools in the model.
type poolListSuite struct {
	baseStorageSuite
}

// TestListPoolSuite runs all of the tests contained within [poolListSuite].
func TestListPoolSuite(t *testing.T) {
	tc.Run(t, &poolListSuite{})
}

const (
	tstName = "testpool"
)

// TestListPoolWithReadPermission tests that a user with read permission on a
// model can successfully list storage pools.
func (s *poolListSuite) TestListPoolWithReadPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()
	userTag := tc.Must1(c, names.ParseUserTag, "user-tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		HasReadTag: userTag,
		Tag:        userTag,
	}

	s.storageService.EXPECT().ListStoragePoolsByNames(gomock.Any(),
		domainstorage.Names{
			"testpool1",
		},
	).Return([]domainstorage.StoragePool{
		{
			Name:     "testpool1",
			Provider: "loop",
		},
	}, nil)

	api := s.makeTestAPI(c)
	results, err := api.ListPools(c.Context(), params.StoragePoolFilters{
		Filters: []params.StoragePoolFilter{{
			Names: []string{"testpool1"},
		}},
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.DeepEquals, []params.StoragePoolsResult{
		{
			Result: []params.StoragePool{
				{
					Name:     "testpool1",
					Provider: "loop",
				},
			},
		},
	})
}

// TestListPoolWithWritePermission tests that a user with write permission on a
// model can successfully list storage pools.
func (s *poolListSuite) TestListPoolWithWritePermission(c *tc.C) {
	defer s.setupMocks(c).Finish()
	userTag := tc.Must1(c, names.ParseUserTag, "user-tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		HasWriteTag: userTag,
		Tag:         userTag,
	}

	s.storageService.EXPECT().ListStoragePoolsByNames(gomock.Any(),
		domainstorage.Names{
			"testpool1",
		},
	).Return([]domainstorage.StoragePool{
		{
			Name:     "testpool1",
			Provider: "loop",
		},
	}, nil)

	api := s.makeTestAPI(c)
	results, err := api.ListPools(c.Context(), params.StoragePoolFilters{
		Filters: []params.StoragePoolFilter{{
			Names: []string{"testpool1"},
		}},
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.DeepEquals, []params.StoragePoolsResult{
		{
			Result: []params.StoragePool{
				{
					Name:     "testpool1",
					Provider: "loop",
				},
			},
		},
	})
}

// TestListPoolWithAdminPermission tests that a user with admin permission on a
// model can successfully list storage pools.
func (s *poolListSuite) TestListPoolWithAdminPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()
	userTag := tc.Must1(c, names.ParseUserTag, "user-tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		AdminTag: userTag,
		Tag:      userTag,
	}

	s.storageService.EXPECT().ListStoragePoolsByNames(gomock.Any(),
		domainstorage.Names{
			"testpool1",
		},
	).Return([]domainstorage.StoragePool{
		{
			Name:     "testpool1",
			Provider: "loop",
		},
	}, nil)

	api := s.makeTestAPI(c)
	results, err := api.ListPools(c.Context(), params.StoragePoolFilters{
		Filters: []params.StoragePoolFilter{{
			Names: []string{"testpool1"},
		}},
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.DeepEquals, []params.StoragePoolsResult{
		{
			Result: []params.StoragePool{
				{
					Name:     "testpool1",
					Provider: "loop",
				},
			},
		},
	})
}

// TestListPoolWithNoPermissionFails tests that if the caller does
// not have model write permission they are unable to list storage pools. The
// caller MUST get back an error with [params.CodeUnauthorized] set.
func (s *poolListSuite) TestListPoolWithNoPermissionFails(c *tc.C) {
	defer s.setupMocks(c).Finish()
	userTag := tc.Must1(c, names.ParseUserTag, "user-tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: userTag,
	}

	api := s.makeTestAPI(c)
	res, err := api.ListPools(c.Context(), params.StoragePoolFilters{
		Filters: []params.StoragePoolFilter{{
			Names: []string{"testpool1"},
		}},
	})
	paramsErr, is := errors.AsType[*params.Error](err)
	c.Assert(is, tc.IsTrue)
	c.Check(paramsErr.Code, tc.Equals, params.CodeUnauthorized)
	c.Check(res.Results, tc.HasLen, 0)
}

func (s *poolListSuite) TestListByNames(c *tc.C) {
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

	api := s.makeTestAPI(c)
	results, err := api.ListPools(c.Context(), params.StoragePoolFilters{
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

func (s *poolListSuite) TestListByProviders(c *tc.C) {
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

	api := s.makeTestAPI(c)
	results, err := api.ListPools(c.Context(), params.StoragePoolFilters{
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

func (s *poolListSuite) TestList(c *tc.C) {
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

	api := s.makeTestAPI(c)
	results, err := api.ListPools(c.Context(), params.StoragePoolFilters{
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

func (s *poolListSuite) TestListAll(c *tc.C) {
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

	api := s.makeTestAPI(c)
	results, err := api.ListPools(c.Context(), params.StoragePoolFilters{[]params.StoragePoolFilter{{}}})
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

func (s *poolListSuite) TestListNoPools(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.storageService.EXPECT().ListStoragePools(gomock.Any()).
		Return([]domainstorage.StoragePool{}, nil)

	api := s.makeTestAPI(c)
	results, err := api.ListPools(c.Context(), params.StoragePoolFilters{[]params.StoragePoolFilter{{}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Result, tc.HasLen, 0)
}
