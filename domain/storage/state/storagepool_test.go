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
