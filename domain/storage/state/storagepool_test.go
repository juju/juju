// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/schema/testing"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	domainstorageinternal "github.com/juju/juju/domain/storage/internal"
)

// storagePoolStateSuite is a set of tests to assert the interface and contracts
// on offer for storage pools in this state package.
type storagePoolStateSuite struct {
	testing.ModelSuite
}

// TestStoragePoolStateSuite runs all of the tests contained in
// [storagePoolStateSuite].
func TestStoragePoolStateSuite(t *stdtesting.T) {
	tc.Run(t, &storagePoolStateSuite{})
}

// TestCreateStoragePool is a happy path test for creating a new storage pool in
// the model.
func (s *storagePoolStateSuite) TestCreateStoragePool(c *tc.C) {
	storagePoolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	args := domainstorageinternal.CreateStoragePool{
		Attrs: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
		Name:         "ebs-fast",
		Origin:       domainstorage.StoragePoolOriginUser,
		ProviderType: domainstorage.ProviderType("ebs"),
		UUID:         storagePoolUUID,
	}

	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	err := st.CreateStoragePool(ctx, args)
	c.Assert(err, tc.ErrorIsNil)

	storagePool, err := st.GetStoragePool(ctx, storagePoolUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(storagePool, tc.DeepEquals, domainstorage.StoragePool{
		Attrs: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
		Name:     "ebs-fast",
		Provider: "ebs",
		UUID:     storagePoolUUID.String(),
	})
}

// TestCreateStoragePoolWithNoAttributes tests that creating a storage pool with
// no attributes works with no errors. This is a common case we expect.
func (s *storagePoolStateSuite) TestCreateStoragePoolWithNoAttributes(c *tc.C) {
	storagePoolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	args := domainstorageinternal.CreateStoragePool{
		Attrs:        nil,
		Name:         "ebs-fast",
		Origin:       domainstorage.StoragePoolOriginUser,
		ProviderType: domainstorage.ProviderType("ebs"),
		UUID:         storagePoolUUID,
	}

	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	err := st.CreateStoragePool(ctx, args)
	c.Assert(err, tc.ErrorIsNil)

	storagePool, err := st.GetStoragePool(ctx, storagePoolUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(storagePool, tc.DeepEquals, domainstorage.StoragePool{
		Name:     "ebs-fast",
		Provider: "ebs",
		UUID:     storagePoolUUID.String(),
	})
}

// TestCreateStoragePoolNameAlreadyExists tests that creating a storage pool
// with the same name of one that exists returns to the caller an error
// satisfying [domainstorageerrors.StoragePoolAlreadyExists].
func (s *storagePoolStateSuite) TestCreateStoragePoolNameAlreadyExists(c *tc.C) {
	storagePoolUUID1 := tc.Must(c, domainstorage.NewStoragePoolUUID)
	args1 := domainstorageinternal.CreateStoragePool{
		Attrs:        nil,
		Name:         "ebs-fast",
		Origin:       domainstorage.StoragePoolOriginUser,
		ProviderType: domainstorage.ProviderType("ebs"),
		UUID:         storagePoolUUID1,
	}

	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	err := st.CreateStoragePool(ctx, args1)
	c.Assert(err, tc.ErrorIsNil)

	storagePoolUUID2 := tc.Must(c, domainstorage.NewStoragePoolUUID)
	args2 := domainstorageinternal.CreateStoragePool{
		Attrs:        nil,
		Name:         "ebs-fast",
		Origin:       domainstorage.StoragePoolOriginUser,
		ProviderType: domainstorage.ProviderType("ebs"),
		UUID:         storagePoolUUID2,
	}
	err = st.CreateStoragePool(ctx, args2)
	c.Check(err, tc.ErrorIs, domainstorageerrors.StoragePoolAlreadyExists)
}

// TestCreateStoragePoolNameUUIDExists tests that creating a storage pool
// with the same uuid of one that exists returns to the caller an error
// satisfying [domainstorageerrors.StoragePoolAlreadyExists].
func (s *storagePoolStateSuite) TestCreateStoragePoolUUIDAlreadyExists(c *tc.C) {
	storagePoolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	args1 := domainstorageinternal.CreateStoragePool{
		Attrs:        nil,
		Name:         "ebs-fast1", // unique name 1
		Origin:       domainstorage.StoragePoolOriginUser,
		ProviderType: domainstorage.ProviderType("ebs"),
		UUID:         storagePoolUUID, // same uuid
	}

	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	err := st.CreateStoragePool(ctx, args1)
	c.Assert(err, tc.ErrorIsNil)

	args2 := domainstorageinternal.CreateStoragePool{
		Attrs:        nil,
		Name:         "ebs-fast2", // unique name 2
		Origin:       domainstorage.StoragePoolOriginUser,
		ProviderType: domainstorage.ProviderType("ebs"),
		UUID:         storagePoolUUID, // same uuid
	}
	err = st.CreateStoragePool(ctx, args2)
	c.Check(err, tc.ErrorIs, domainstorageerrors.StoragePoolAlreadyExists)
}

func (s *storagePoolStateSuite) TestReplaceStoragePool(c *tc.C) {
	storagePoolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	args := domainstorageinternal.CreateStoragePool{
		Attrs:        nil,
		Name:         "ebs-fast",
		Origin:       domainstorage.StoragePoolOriginUser,
		ProviderType: domainstorage.ProviderType("ebs"),
		UUID:         storagePoolUUID,
	}

	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	err := st.CreateStoragePool(ctx, args)
	c.Assert(err, tc.ErrorIsNil)

	sp2 := domainstorage.StoragePool{
		UUID:     storagePoolUUID.String(),
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"baz": "baz val",
		},
	}

	err = st.ReplaceStoragePool(ctx, sp2)
	c.Assert(err, tc.ErrorIsNil)

	out, err := st.GetStoragePool(ctx, storagePoolUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.DeepEquals, domainstorage.StoragePool{
		Attrs: map[string]string{
			"baz": "baz val",
		},
		Name:     "ebs-fast",
		Provider: "ebs",
		UUID:     storagePoolUUID.String(),
	})
}

func (s *storagePoolStateSuite) TestReplaceStoragePoolNoAttributes(c *tc.C) {
	storagePoolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	args := domainstorageinternal.CreateStoragePool{
		Attrs:        nil,
		Name:         "ebs-fast",
		Origin:       domainstorage.StoragePoolOriginUser,
		ProviderType: domainstorage.ProviderType("ebs"),
		UUID:         storagePoolUUID,
	}

	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	err := st.CreateStoragePool(ctx, args)
	c.Assert(err, tc.ErrorIsNil)

	sp2 := domainstorage.StoragePool{
		UUID:     storagePoolUUID.String(),
		Name:     "ebs-fast",
		Provider: "ebs",
	}
	err = st.ReplaceStoragePool(ctx, sp2)
	c.Assert(err, tc.ErrorIsNil)

	out, err := st.GetStoragePool(ctx, storagePoolUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.DeepEquals, domainstorage.StoragePool{
		Name:     "ebs-fast",
		Provider: "ebs",
		UUID:     storagePoolUUID.String(),
	})
}

func (s *storagePoolStateSuite) TestReplaceStoragePoolNotFound(c *tc.C) {
	poolUUID, err := domainstorage.NewStoragePoolUUID()
	c.Assert(err, tc.ErrorIsNil)
	sp := domainstorage.StoragePool{
		UUID:     poolUUID.String(),
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"baz": "baz val",
		},
	}

	st := NewState(s.TxnRunnerFactory())
	err = st.ReplaceStoragePool(c.Context(), sp)
	c.Check(err, tc.ErrorIs, domainstorageerrors.StoragePoolNotFound)
}

// TestSetModelStoragePoolsEmptyArgs tests that supplying no arguments results
// in a no-op with no error.
func (s *storagePoolStateSuite) TestSetModelStoragePoolsEmptyArgs(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	err := st.SetModelStoragePools(ctx, nil)
	c.Check(err, tc.ErrorIsNil)
}

// TestSetModelStoragePools tests that model storage pools are replaced with
// the supplied recommended storage pools.
func (s *storagePoolStateSuite) TestSetModelStoragePools(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	// Create storage pools first
	uuid1 := tc.Must(c, domainstorage.NewStoragePoolUUID)
	uuid2 := tc.Must(c, domainstorage.NewStoragePoolUUID)

	err := st.CreateStoragePool(ctx, domainstorageinternal.CreateStoragePool{
		Name:         "pool-fs",
		ProviderType: "ebs",
		Origin:       domainstorage.StoragePoolOriginUser,
		UUID:         uuid1,
	})
	c.Assert(err, tc.ErrorIsNil)

	err = st.CreateStoragePool(ctx, domainstorageinternal.CreateStoragePool{
		Name:         "pool-block",
		ProviderType: "ebs",
		Origin:       domainstorage.StoragePoolOriginUser,
		UUID:         uuid2,
	})
	c.Assert(err, tc.ErrorIsNil)

	args := []domainstorageinternal.RecommendedStoragePoolArg{
		{
			StorageKind:     domainstorage.StorageKindFilesystem,
			StoragePoolUUID: uuid1,
		},
		{
			StorageKind:     domainstorage.StorageKindBlock,
			StoragePoolUUID: uuid2,
		},
	}

	err = st.SetModelStoragePools(ctx, args)
	c.Assert(err, tc.ErrorIsNil)

	modelStoragePools := s.getModelStoragePools(c)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(modelStoragePools, tc.SameContents, []dbModelStoragePool{
		{
			StoragePoolUUID: uuid2.String(),
			StorageKindID:   int(domainstorage.StorageKindBlock),
		},
		{
			StoragePoolUUID: uuid1.String(),
			StorageKindID:   int(domainstorage.StorageKindFilesystem),
		},
	})
}

// TestSetModelStoragePoolsReplacesExisting tests that existing model storage
// pools are deleted before inserting new ones.
func (s *storagePoolStateSuite) TestSetModelStoragePoolsReplacesExisting(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	uuid1 := tc.Must(c, domainstorage.NewStoragePoolUUID)
	uuid2 := tc.Must(c, domainstorage.NewStoragePoolUUID)

	for _, uuid := range []domainstorage.StoragePoolUUID{uuid1, uuid2} {
		err := st.CreateStoragePool(ctx, domainstorageinternal.CreateStoragePool{
			Name:         uuid.String(),
			ProviderType: "ebs",
			Origin:       domainstorage.StoragePoolOriginUser,
			UUID:         uuid,
		})
		c.Assert(err, tc.ErrorIsNil)
	}

	err := st.SetModelStoragePools(ctx, []domainstorageinternal.RecommendedStoragePoolArg{
		{
			StorageKind:     domainstorage.StorageKindFilesystem,
			StoragePoolUUID: uuid1,
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	err = st.SetModelStoragePools(ctx, []domainstorageinternal.RecommendedStoragePoolArg{
		{
			StorageKind:     domainstorage.StorageKindBlock,
			StoragePoolUUID: uuid2,
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	modelPools := s.getModelStoragePools(c)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(modelPools, tc.DeepEquals, []dbModelStoragePool{
		{
			StorageKindID:   int(domainstorage.StorageKindBlock),
			StoragePoolUUID: uuid2.String(),
		},
	})
}

// TestSetModelStoragePoolsPoolNotFound tests that supplying a storage pool UUID
// that does not exist returns [domainstorageerrors.StoragePoolNotFound].
func (s *storagePoolStateSuite) TestSetModelStoragePoolsPoolNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	missingUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)

	err := st.SetModelStoragePools(ctx, []domainstorageinternal.RecommendedStoragePoolArg{
		{
			StorageKind:     domainstorage.StorageKindFilesystem,
			StoragePoolUUID: missingUUID,
		},
	})
	c.Check(err, tc.ErrorIs, domainstorageerrors.StoragePoolNotFound)
}

// TestSetModelStoragePoolsDeduplicatesUUIDs tests that duplicate storage pool
// UUIDs do not cause existence checks to fail.
func (s *storagePoolStateSuite) TestSetModelStoragePoolsDeduplicatesUUIDs(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	uuid := tc.Must(c, domainstorage.NewStoragePoolUUID)

	err := st.CreateStoragePool(ctx, domainstorageinternal.CreateStoragePool{
		Name:         "shared-pool",
		ProviderType: "ebs",
		Origin:       domainstorage.StoragePoolOriginUser,
		UUID:         uuid,
	})
	c.Assert(err, tc.ErrorIsNil)

	err = st.SetModelStoragePools(ctx, []domainstorageinternal.RecommendedStoragePoolArg{
		{
			StorageKind:     domainstorage.StorageKindFilesystem,
			StoragePoolUUID: uuid,
		},
		{
			StorageKind: domainstorage.StorageKindBlock,
			// This is a duplicate UUID.
			StoragePoolUUID: uuid,
		},
	})
	c.Check(err, tc.ErrorIsNil)
}

func (s *storagePoolStateSuite) TestGetStoragePoolNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	_, err := st.GetStoragePool(c.Context(), poolUUID)
	c.Check(err, tc.ErrorIs, domainstorageerrors.StoragePoolNotFound)
}

func (s *storagePoolStateSuite) TestGetStoragePoolProvidersByNames(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Arrange
	err := st.CreateStoragePool(c.Context(), domainstorageinternal.CreateStoragePool{
		Name:         "foo",
		ProviderType: domainstorage.ProviderType("bar"),
		UUID:         tc.Must(c, domainstorage.NewStoragePoolUUID),
	})
	c.Assert(err, tc.ErrorIsNil)

	err = st.CreateStoragePool(c.Context(), domainstorageinternal.CreateStoragePool{
		Name:         "bar",
		ProviderType: domainstorage.ProviderType("foo"),
		UUID:         tc.Must(c, domainstorage.NewStoragePoolUUID),
	})
	c.Assert(err, tc.ErrorIsNil)

	// Act
	providers, err := st.GetStoragePoolProvidersByNames(c.Context(), []string{"foo", "bar"})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(providers, tc.DeepEquals, map[string]string{
		"foo": "bar",
		"bar": "foo",
	})
}

func (s *storagePoolStateSuite) TestGetStoragePoolProvidersByNamesDuplicateNames(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Arrange
	err := st.CreateStoragePool(c.Context(), domainstorageinternal.CreateStoragePool{
		Name:         "foo",
		ProviderType: domainstorage.ProviderType("bar"),
		UUID:         tc.Must(c, domainstorage.NewStoragePoolUUID),
	})
	c.Assert(err, tc.ErrorIsNil)

	err = st.CreateStoragePool(c.Context(), domainstorageinternal.CreateStoragePool{
		Name:         "bar",
		ProviderType: domainstorage.ProviderType("foo"),
		UUID:         tc.Must(c, domainstorage.NewStoragePoolUUID),
	})
	c.Assert(err, tc.ErrorIsNil)

	// Act
	providers, err := st.GetStoragePoolProvidersByNames(c.Context(), []string{"foo", "bar", "bar"})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(providers, tc.DeepEquals, map[string]string{
		"foo": "bar",
		"bar": "foo",
	})
}

func (s *storagePoolStateSuite) TestGetStoragePoolProvidersByNamesMiss(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Arrange
	err := st.CreateStoragePool(c.Context(), domainstorageinternal.CreateStoragePool{
		Name:         "foo",
		ProviderType: domainstorage.ProviderType("bar"),
		UUID:         tc.Must(c, domainstorage.NewStoragePoolUUID),
	})
	c.Assert(err, tc.ErrorIsNil)

	err = st.CreateStoragePool(c.Context(), domainstorageinternal.CreateStoragePool{
		Name:         "bar",
		ProviderType: domainstorage.ProviderType("foo"),
		UUID:         tc.Must(c, domainstorage.NewStoragePoolUUID),
	})
	c.Assert(err, tc.ErrorIsNil)

	// Act
	_, err = st.GetStoragePoolProvidersByNames(c.Context(), []string{"foo", "bar", "baz", "qux"})

	// Assert
	c.Check(err, tc.ErrorIs, domainstorageerrors.StoragePoolNotFound)
}

func (s *storagePoolStateSuite) TestGetStoragePoolProvidersByNamesNoPools(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetStoragePoolProvidersByNames(c.Context(), []string{"foo", "bar"})

	c.Check(err, tc.ErrorIs, domainstorageerrors.StoragePoolNotFound)
}

// getModelStoragePools is a helper method to fetch model storage pools from DB.
func (s *storagePoolStateSuite) getModelStoragePools(
	c *tc.C,
) []dbModelStoragePool {
	query := `
SELECT storage_kind_id, storage_pool_uuid
FROM model_storage_pool
`
	var result []dbModelStoragePool

	rows, err := s.DB().Query(query)
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	for rows.Next() {
		pool := dbModelStoragePool{}
		err = rows.Scan(&pool.StorageKindID, &pool.StoragePoolUUID)
		c.Assert(err, tc.ErrorIsNil)

		result = append(result, pool)
	}

	return result
}

//func (s *storagePoolStateSuite) TestDeleteStoragePool(c *tc.C) {
//	st := newStoragePoolState(s.TxnRunnerFactory())
//
//	sp := domainstorage.StoragePool{
//		Name:     "ebs-fast",
//		Provider: "ebs",
//		Attrs: map[string]string{
//			"foo": "foo val",
//			"bar": "bar val",
//		},
//	}
//	ctx := c.Context()
//	err := st.CreateStoragePool(ctx, sp)
//	c.Assert(err, tc.ErrorIsNil)
//
//	err = st.DeleteStoragePool(ctx, "ebs-fast")
//	c.Assert(err, tc.ErrorIsNil)
//
//	_, err = st.getStoragePoolByName(ctx, "ebs-fast")
//	c.Assert(err, tc.ErrorIs, storageerrors.PoolNotFoundError)
//}
//
//func (s *storagePoolStateSuite) TestDeleteStoragePoolNotFound(c *tc.C) {
//	st := newStoragePoolState(s.TxnRunnerFactory())
//
//	ctx := c.Context()
//	err := st.DeleteStoragePool(ctx, "ebs-fast")
//	c.Assert(err, tc.ErrorIs, storageerrors.PoolNotFoundError)
//}

//func (s *storagePoolStateSuite) TestListStoragePools(c *tc.C) {
//	st := newStoragePoolState(s.TxnRunnerFactory())
//
//	defaultPools := s.ensureProviderDefaultStoragePools(c)
//
//	sp := domainstorage.StoragePool{
//		Name:     "ebs-fast",
//		Provider: "ebs",
//		Attrs: map[string]string{
//			"foo": "foo val",
//			"bar": "bar val",
//		},
//	}
//	sp2 := domainstorage.StoragePool{
//		Name:     "ebs-faster",
//		Provider: "ebs",
//		Attrs: map[string]string{
//			"baz": "baz val",
//		},
//	}
//	ctx := c.Context()
//	err := st.CreateStoragePool(ctx, sp)
//	c.Assert(err, tc.ErrorIsNil)
//	err = st.CreateStoragePool(ctx, sp2)
//	c.Assert(err, tc.ErrorIsNil)
//
//	out, err := st.ListStoragePools(c.Context())
//	c.Assert(err, tc.ErrorIsNil)
//
//	expected := []domainstorage.StoragePool{sp, sp2}
//	expected = append(expected, defaultPools...)
//	c.Assert(out, tc.SameContents, expected)
//}
//
//func (s *storagePoolStateSuite) TestListStoragePoolsNoUserPools(c *tc.C) {
//	st := newStoragePoolState(s.TxnRunnerFactory())
//
//	defaultPools := s.ensureProviderDefaultStoragePools(c)
//
//	out, err := st.ListStoragePools(c.Context())
//	c.Assert(err, tc.ErrorIsNil)
//
//	var expected []domainstorage.StoragePool
//	expected = append(expected, defaultPools...)
//	c.Assert(out, tc.SameContents, expected)
//}
//
//func (s *storagePoolStateSuite) TestListStoragePoolsByNamesAndProviders(c *tc.C) {
//	st := newStoragePoolState(s.TxnRunnerFactory())
//
//	_ = s.ensureProviderDefaultStoragePools(c)
//
//	sp := domainstorage.StoragePool{
//		Name:     "ebs-fast",
//		Provider: "ebs",
//		Attrs: map[string]string{
//			"foo": "foo val",
//			"bar": "bar val",
//		},
//	}
//
//	ctx := c.Context()
//	err := st.CreateStoragePool(ctx, sp)
//	c.Assert(err, tc.ErrorIsNil)
//
//	out, err := st.ListStoragePoolsByNamesAndProviders(c.Context(),
//		[]string{"pool1", "pool2", "ebs-fast", "ebs-fast", "loop", ""},
//		[]string{"whatever", "ebs", "ebs", "loop", ""},
//	)
//	c.Assert(err, tc.ErrorIsNil)
//	c.Assert(out, tc.SameContents, []domainstorage.StoragePool{
//		sp,
//		{
//			Name:     "pool1",
//			Provider: "whatever",
//			Attrs: map[string]string{
//				"1": "2",
//			},
//		},
//		{
//			Name:     "pool2",
//			Provider: "whatever",
//			Attrs: map[string]string{
//				"3": "4",
//				"5": "6",
//			},
//		},
//	})
//}
//
//func (s *storagePoolStateSuite) TestListStoragePoolsByNames(c *tc.C) {
//	st := newStoragePoolState(s.TxnRunnerFactory())
//
//	_ = s.ensureProviderDefaultStoragePools(c)
//
//	sp := domainstorage.StoragePool{
//		Name:     "ebs-fast",
//		Provider: "ebs",
//		Attrs: map[string]string{
//			"foo": "foo val",
//			"bar": "bar val",
//		},
//	}
//
//	ctx := c.Context()
//	err := st.CreateStoragePool(ctx, sp)
//	c.Assert(err, tc.ErrorIsNil)
//
//	out, err := st.ListStoragePoolsByNames(c.Context(), []string{"pool1", "ebs-fast", ""})
//	c.Assert(err, tc.ErrorIsNil)
//	c.Assert(out, tc.SameContents, []domainstorage.StoragePool{
//		sp,
//		{
//			Name:     "pool1",
//			Provider: "whatever",
//			Attrs: map[string]string{
//				"1": "2",
//			},
//		},
//	})
//}
//
//func (s *storagePoolStateSuite) TestListStoragePoolsByProviders(c *tc.C) {
//	st := newStoragePoolState(s.TxnRunnerFactory())
//
//	_ = s.ensureProviderDefaultStoragePools(c)
//
//	sp := domainstorage.StoragePool{
//		Name:     "ebs-fast",
//		Provider: "ebs",
//		Attrs: map[string]string{
//			"foo": "foo val",
//			"bar": "bar val",
//		},
//	}
//
//	ctx := c.Context()
//	err := st.CreateStoragePool(ctx, sp)
//	c.Assert(err, tc.ErrorIsNil)
//
//	out, err := st.ListStoragePoolsByProviders(c.Context(), []string{"whatever", "ebs", ""})
//	c.Assert(err, tc.ErrorIsNil)
//	c.Assert(out, tc.SameContents, []domainstorage.StoragePool{
//		sp,
//		{
//			Name:     "pool1",
//			Provider: "whatever",
//			Attrs: map[string]string{
//				"1": "2",
//			},
//		},
//		{
//			Name:     "pool2",
//			Provider: "whatever",
//			Attrs: map[string]string{
//				"3": "4",
//				"5": "6",
//			},
//		},
//	})
//}
//
//func (s *storagePoolStateSuite) TestGetStoragePoolUUID(c *tc.C) {
//	st := newStoragePoolState(s.TxnRunnerFactory())
//
//	sp := domainstorage.StoragePool{
//		Name:     "ebs-fast",
//		Provider: "ebs",
//		Attrs: map[string]string{
//			"foo": "foo val",
//			"bar": "bar val",
//		},
//	}
//
//	ctx := c.Context()
//	err := st.CreateStoragePool(ctx, sp)
//	c.Assert(err, tc.ErrorIsNil)
//
//	var poolUUID string
//	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
//		return tx.QueryRowContext(ctx, `
//SELECT sp.uuid
//FROM   storage_pool sp
//WHERE  sp.name = ?`, "ebs-fast").Scan(&poolUUID)
//	})
//	c.Assert(err, tc.ErrorIsNil)
//
//	result, err := st.GetStoragePoolUUID(ctx, "ebs-fast")
//	c.Assert(err, tc.ErrorIsNil)
//	c.Assert(result.String(), tc.Equals, poolUUID)
//}
//
//func (s *storagePoolStateSuite) TestGetStoragePoolUUIDNotFound(c *tc.C) {
//	st := newStoragePoolState(s.TxnRunnerFactory())
//
//	_, err := st.GetStoragePoolUUID(c.Context(), "non-existent-pool")
//	c.Assert(err, tc.ErrorIs, storageerrors.PoolNotFoundError)
//}
//
//func (s *storagePoolStateSuite) TestGetStoragePool(c *tc.C) {
//	st := newStoragePoolState(s.TxnRunnerFactory())
//
//	_ = s.ensureProviderDefaultStoragePools(c)
//
//	sp := domainstorage.StoragePool{
//		Name:     "ebs-fast",
//		Provider: "ebs",
//		Attrs: map[string]string{
//			"foo": "foo val",
//			"bar": "bar val",
//		},
//	}
//
//	ctx := c.Context()
//	err := st.CreateStoragePool(ctx, sp)
//	c.Assert(err, tc.ErrorIsNil)
//
//	poolUUID, err := st.GetStoragePoolUUID(ctx, "ebs-fast")
//	c.Assert(err, tc.ErrorIsNil)
//
//	out, err := st.GetStoragePool(c.Context(), poolUUID)
//	c.Assert(err, tc.ErrorIsNil)
//	c.Assert(out, tc.DeepEquals, sp)
//}
//
//func (s *storagePoolStateSuite) TestGetStoragePoolDefault(c *tc.C) {
//	st := newStoragePoolState(s.TxnRunnerFactory())
//
//	_ = s.ensureProviderDefaultStoragePools(c)
//
//	poolUUID, err := st.GetStoragePoolUUID(c.Context(), "pool1")
//	c.Assert(err, tc.ErrorIsNil)
//
//	out, err := st.GetStoragePool(c.Context(), poolUUID)
//	c.Assert(err, tc.ErrorIsNil)
//	c.Assert(out, tc.DeepEquals, domainstorage.StoragePool{
//		Name:     "pool1",
//		Provider: "whatever",
//		Attrs: map[string]string{
//			"1": "2",
//		},
//	})
//}
//
//func (s *storagePoolStateSuite) TestGetStoragePoolNotFound(c *tc.C) {
//	st := newStoragePoolState(s.TxnRunnerFactory())
//
//	_ = s.ensureProviderDefaultStoragePools(c)
//
//	poolUUID, err := domainstorage.NewStoragePoolUUID()
//	c.Assert(err, tc.ErrorIsNil)
//
//	_, err = st.GetStoragePool(c.Context(), poolUUID)
//	c.Assert(err, tc.ErrorIs, storageerrors.PoolNotFoundError)
//}
//
