// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apiservertesting "github.com/juju/juju/apiserver/testing"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// poolCreateSuite is a test suite to assert the contracts offered by the facade
// for creating storage pools.
type poolCreateSuite struct {
	baseStorageSuite
}

// TestPoolCreateSuite runs all of the tests contained within [poolCreateSuite].
func TestPoolCreateSuite(t *testing.T) {
	tc.Run(t, &poolCreateSuite{})
}

// TestCreateStoragePool is a happy path test for creating multiple storage
// pools in the facade.
func (s *poolCreateSuite) TestCreateStoragePool(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.makeTestAPIForIAASModel(c)

	storageEXP := s.storageService.EXPECT()
	storageEXP.CreateStoragePool(
		gomock.Any(),
		"test-pool1",
		domainstorage.ProviderType("myprovider1"),
		nil,
	).Return(tc.Must(c, domainstorage.NewStoragePoolUUID), nil)
	storageEXP.CreateStoragePool(
		gomock.Any(),
		"test-pool2",
		domainstorage.ProviderType("myprovider2"),
		map[string]any{
			"key1": 10,
			"key2": "foo",
		},
	).Return(tc.Must(c, domainstorage.NewStoragePoolUUID), nil)

	apiArgs := params.StoragePoolArgs{
		Pools: []params.StoragePool{
			{
				Name:     "test-pool2",
				Provider: "myprovider2",
				Attrs: map[string]any{
					"key1": 10,
					"key2": "foo",
				},
			},
			{
				Name:     "test-pool1",
				Provider: "myprovider1",
				Attrs:    nil,
			},
		},
	}

	res, err := api.CreatePool(c.Context(), apiArgs)
	c.Check(err, tc.IsNil)
	c.Assert(res.Results, tc.HasLen, 2)
	c.Check(res.Results[0].Error, tc.IsNil)
	c.Check(res.Results[1].Error, tc.IsNil)
}

// TestCreateStoragePoolWithOneFailure tests that multiple storage pools can be
// created but where a storage pool fails to be created the callers gets back a
// result error. The other non failure storage pools are expected to be created.
func (s *poolCreateSuite) TestCreateStoragePoolWithOneFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.makeTestAPIForIAASModel(c)

	storageEXP := s.storageService.EXPECT()
	storageEXP.CreateStoragePool(
		gomock.Any(),
		"test-pool1",
		domainstorage.ProviderType("myprovider1"),
		nil,
	).Return(tc.Must(c, domainstorage.NewStoragePoolUUID), nil)
	storageEXP.CreateStoragePool(
		gomock.Any(),
		"test-pool2",
		domainstorage.ProviderType("myprovider2"),
		map[string]any{
			"key1": 10,
			"key2": "foo",
		},
	).Return(tc.Must(c, domainstorage.NewStoragePoolUUID), nil)
	storageEXP.CreateStoragePool(
		gomock.Any(),
		"test-pool3",
		domainstorage.ProviderType("myprovider3"),
		nil,
	).Return("", domainstorageerrors.StoragePoolNameInvalid)

	apiArgs := params.StoragePoolArgs{
		Pools: []params.StoragePool{
			{
				Name:     "test-pool2",
				Provider: "myprovider2",
				Attrs: map[string]any{
					"key1": 10,
					"key2": "foo",
				},
			},
			{
				Name:     "test-pool3",
				Provider: "myprovider3",
				Attrs:    nil,
			},
			{
				Name:     "test-pool1",
				Provider: "myprovider1",
				Attrs:    nil,
			},
		},
	}

	res, err := api.CreatePool(c.Context(), apiArgs)
	c.Check(err, tc.IsNil)
	c.Assert(res.Results, tc.HasLen, 3)
	c.Check(res.Results[0].Error, tc.IsNil)
	c.Check(res.Results[1].Error.Code, tc.Equals, params.CodeNotValid)
	c.Check(res.Results[2].Error, tc.IsNil)
}

// TestCreateStoragePoolWithReadPermissionFails tests that if the caller does
// not have model write permission they are unable to create storage pools. The
// caller MUST get back an error with [params.CodeUnauthorized] set.
func (s *poolCreateSuite) TestCreateStoragePoolWithNoPermissionFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	userTag := tc.Must1(c, names.ParseUserTag, "user-tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: userTag,
	}

	apiArgs := params.StoragePoolArgs{
		Pools: []params.StoragePool{
			{
				Name:     "test-pool3",
				Provider: "myprovider3",
				Attrs:    nil,
			},
		},
	}

	api := s.makeTestAPIForIAASModel(c)
	res, err := api.CreatePool(c.Context(), apiArgs)
	paramsErr, is := errors.AsType[*params.Error](err)
	c.Assert(is, tc.IsTrue)
	c.Check(paramsErr.Code, tc.Equals, params.CodeUnauthorized)
	c.Check(res.Results, tc.HasLen, 0)
}

// TestCreateStoragePoolWithReadPermissionFails tests that if the caller does
// not have model write permission they are unable to create storage pools. The
// caller MUST get back an error with [params.CodeUnauthorized] set.
func (s *poolCreateSuite) TestCreateStoragePoolWithReadPermissionFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	userTag := tc.Must1(c, names.ParseUserTag, "user-tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		HasReadTag: userTag,
		Tag:        userTag,
	}

	apiArgs := params.StoragePoolArgs{
		Pools: []params.StoragePool{
			{
				Name:     "test-pool3",
				Provider: "myprovider3",
				Attrs:    nil,
			},
		},
	}

	api := s.makeTestAPIForIAASModel(c)
	res, err := api.CreatePool(c.Context(), apiArgs)
	paramsErr, is := errors.AsType[*params.Error](err)
	c.Assert(is, tc.IsTrue)
	c.Check(paramsErr.Code, tc.Equals, params.CodeUnauthorized)
	c.Check(res.Results, tc.HasLen, 0)
}

// TestCreateStoragePoolWithWritePermission tests that if the caller has model
// write permissions they are allowed to create storage pools.
func (s *poolCreateSuite) TestCreateStoragePoolWithWritePermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	userTag := tc.Must1(c, names.ParseUserTag, "user-tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		HasWriteTag: userTag,
		Tag:         userTag,
	}

	storageEXP := s.storageService.EXPECT()
	storageEXP.CreateStoragePool(
		gomock.Any(),
		"test-pool",
		domainstorage.ProviderType("myprovider"),
		nil,
	).Return(tc.Must(c, domainstorage.NewStoragePoolUUID), nil)

	apiArgs := params.StoragePoolArgs{
		Pools: []params.StoragePool{
			{
				Name:     "test-pool",
				Provider: "myprovider",
				Attrs:    nil,
			},
		},
	}

	api := s.makeTestAPIForIAASModel(c)
	res, err := api.CreatePool(c.Context(), apiArgs)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.IsNil)
}

// TestCreateStoragePoolWithAdminPermission tests that if the caller has model
// admin permissions they are allowed to create storage pools.
func (s *poolCreateSuite) TestCreateStoragePoolWithAdminPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	userTag := tc.Must1(c, names.ParseUserTag, "user-tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		AdminTag: userTag,
		Tag:      userTag,
	}

	storageEXP := s.storageService.EXPECT()
	storageEXP.CreateStoragePool(
		gomock.Any(),
		"test-pool",
		domainstorage.ProviderType("myprovider"),
		nil,
	).Return(tc.Must(c, domainstorage.NewStoragePoolUUID), nil)

	apiArgs := params.StoragePoolArgs{
		Pools: []params.StoragePool{
			{
				Name:     "test-pool",
				Provider: "myprovider",
				Attrs:    nil,
			},
		},
	}

	api := s.makeTestAPIForIAASModel(c)
	res, err := api.CreatePool(c.Context(), apiArgs)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.IsNil)
}

// TestCreateStoragePoolInvalidName tests that when a caller attempts to create
// a storage pool with an invalid name they get back a params error with the
// code set to [params.CodeNotValid].
//
// This test also ensures correct translation of the domain error
// [domainstorageerrors.StoragePoolNameInvalid] to a params error code.
func (s *poolCreateSuite) TestCreateStoragePoolInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.makeTestAPIForIAASModel(c)

	storageEXP := s.storageService.EXPECT()
	storageEXP.CreateStoragePool(
		gomock.Any(),
		"🍔🍔🍔",
		domainstorage.ProviderType("triple-burger"),
		nil,
	).Return("", domainstorageerrors.StoragePoolNameInvalid)

	apiArgs := params.StoragePoolArgs{
		Pools: []params.StoragePool{
			{
				Name:     "🍔🍔🍔",
				Provider: "triple-burger",
				Attrs:    nil,
			},
		},
	}

	res, err := api.CreatePool(c.Context(), apiArgs)
	c.Check(err, tc.IsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error.Code, tc.Equals, params.CodeNotValid)
}

// TestCreateStoragePoolInvalidProviderType tests that when a caller attempts to
// create a storage pool with an invalid provider type they get back a params
// error with the code set to [params.CodeNotValid].
//
// This test also ensures correct translation of the domain error
// [domainstorageerrors.ProviderTypeInvalid] to a params error code.
func (s *poolCreateSuite) TestCreateStoragePoolInvalidProviderType(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.makeTestAPIForIAASModel(c)

	storageEXP := s.storageService.EXPECT()
	storageEXP.CreateStoragePool(
		gomock.Any(),
		"storagepool1",
		domainstorage.ProviderType("👻"),
		nil,
	).Return("", domainstorageerrors.ProviderTypeInvalid)

	apiArgs := params.StoragePoolArgs{
		Pools: []params.StoragePool{
			{
				Name:     "storagepool1",
				Provider: "👻",
				Attrs:    nil,
			},
		},
	}

	res, err := api.CreatePool(c.Context(), apiArgs)
	c.Check(err, tc.IsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error.Code, tc.Equals, params.CodeNotValid)
}

// TestCreateStoragePoolProviderTypeNotFound tests that when a caller attempts
// to create a storage pool with a provider type that doesn't exist, they get
// back a params error with the code set to [params.CodeNotFound].
//
// This test also ensures correct translation of the domain error
// [domainstorageerrors.ProviderTypeNotFound] to a params error code.
func (s *poolCreateSuite) TestCreateStoragePoolProviderTypeNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.makeTestAPIForIAASModel(c)

	storageEXP := s.storageService.EXPECT()
	storageEXP.CreateStoragePool(
		gomock.Any(),
		"storagepool1",
		domainstorage.ProviderType("notfoundprovider"),
		nil,
	).Return("", domainstorageerrors.ProviderTypeNotFound)

	apiArgs := params.StoragePoolArgs{
		Pools: []params.StoragePool{
			{
				Name:     "storagepool1",
				Provider: "notfoundprovider",
				Attrs:    nil,
			},
		},
	}

	res, err := api.CreatePool(c.Context(), apiArgs)
	c.Check(err, tc.IsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
}

// TestCreateStoragePoolAlreadyExists tests that when a caller attempts
// to create a storage pool with a name that already exists in the model they
// get back a params error with the code set to [params.CodeAlreadyExists].
//
// This test also ensures correct translation of the domain error
// [domainstorageerrors.StoragePoolAlreadyExists] to a params error code.
func (s *poolCreateSuite) TestCreateStoragePoolAlreadyExists(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.makeTestAPIForIAASModel(c)

	storageEXP := s.storageService.EXPECT()
	storageEXP.CreateStoragePool(
		gomock.Any(),
		"storagepool1",
		domainstorage.ProviderType("myprovider"),
		nil,
	).Return("", domainstorageerrors.StoragePoolAlreadyExists)

	apiArgs := params.StoragePoolArgs{
		Pools: []params.StoragePool{
			{
				Name:     "storagepool1",
				Provider: "myprovider",
				Attrs:    nil,
			},
		},
	}

	res, err := api.CreatePool(c.Context(), apiArgs)
	c.Check(err, tc.IsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error.Code, tc.Equals, params.CodeAlreadyExists)
}

// TestCreateStoragePoolAlreadyExists tests that when a caller attempts
// to create a storage pool with a name that already exists in the model they
// get back a params error with the code set to [params.CodeAlreadyExists].
//
// This test also ensures correct translation of the domain error
// [domainstorageerrors.StoragePoolAlreadyExists] to a params error code.
func (s *poolCreateSuite) TestCreateStoragePoolInvalidAttribute(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.makeTestAPIForIAASModel(c)

	storageEXP := s.storageService.EXPECT()
	storageEXP.CreateStoragePool(
		gomock.Any(),
		"storagepool1",
		domainstorage.ProviderType("myprovider"),
		map[string]any{
			"key1": "badvalue",
		},
	).Return("", domainstorageerrors.StoragePoolAttributeInvalid{
		Key:     "key1",
		Message: "invalid value",
	})

	apiArgs := params.StoragePoolArgs{
		Pools: []params.StoragePool{
			{
				Name:     "storagepool1",
				Provider: "myprovider",
				Attrs: map[string]any{
					"key1": "badvalue",
				},
			},
		},
	}

	res, err := api.CreatePool(c.Context(), apiArgs)
	c.Check(err, tc.IsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error.Code, tc.Equals, params.CodeNotValid)

	// Check the error message returned in this case. We care about this as the
	// error type [domainstorageerrors.StoragePoolAttributeInvalid] is used to
	// form a better error message for the client. While we shouldn't be writing
	// user facing error messages in the API facade this is what the client
	// relies upon today.
	c.Check(
		res.Results[0].Error.Message,
		tc.Equals,
		"storage pool attribute \"key1\" is not valid: invalid value",
	)
}
