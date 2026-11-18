// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	stdtesting "testing"

	"github.com/juju/tc"

	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	domainstorageinternal "github.com/juju/juju/domain/storage/internal"
	"github.com/juju/juju/internal/errors"
)

// storagePoolStateSuite is a set of tests to assert the interface and contracts
// on offer for storage pools in this state package.
type storagePoolStateSuite struct {
	baseSuite
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

func (s *storagePoolStateSuite) TestCreateStoragePoolOrigins(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	for _, origin := range []domainstorage.StoragePoolOrigin{
		domainstorage.StoragePoolOriginUser,
		domainstorage.StoragePoolOriginProviderDefault,
	} {
		poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
		err := st.CreateStoragePool(c.Context(), domainstorageinternal.CreateStoragePool{
			UUID:         poolUUID,
			Name:         poolUUID.String(),
			ProviderType: "ebs",
			Origin:       origin,
		})
		c.Assert(err, tc.ErrorIsNil)
		var storedOrigin int
		err = s.DB().QueryRowContext(c.Context(),
			"SELECT origin_id FROM storage_pool WHERE uuid = ?", poolUUID.String(),
		).Scan(&storedOrigin)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(storedOrigin, tc.Equals, int(origin))
		pool, err := st.GetStoragePool(c.Context(), poolUUID)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(pool, tc.DeepEquals, domainstorage.StoragePool{
			UUID: poolUUID.String(), Name: poolUUID.String(), Provider: "ebs",
		})
	}
}

func (s *storagePoolStateSuite) TestReplaceStoragePoolRenameCollision(c *tc.C) {
	poolUUID := s.newStoragePool(c, "original", "ebs", domainstorage.Attrs{"type": "fast"})
	otherUUID := s.newStoragePool(c, "occupied", "lxd", domainstorage.Attrs{"pool": "default"})
	st := NewState(s.TxnRunnerFactory())
	err := st.ReplaceStoragePool(c.Context(), domainstorage.StoragePool{
		UUID:     poolUUID.String(),
		Name:     "occupied",
		Provider: "lxd",
		Attrs:    domainstorage.Attrs{"type": "slow"},
	})
	c.Check(err, tc.NotNil)
	c.Check(errors.Is(err, domainstorageerrors.StoragePoolAlreadyExists), tc.IsFalse)
	pool, err := st.GetStoragePool(c.Context(), poolUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(pool, tc.DeepEquals, domainstorage.StoragePool{
		UUID: poolUUID.String(), Name: "original", Provider: "ebs",
		Attrs: domainstorage.Attrs{"type": "fast"},
	})
	other, err := st.GetStoragePool(c.Context(), otherUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(other, tc.DeepEquals, domainstorage.StoragePool{
		UUID: otherUUID.String(), Name: "occupied", Provider: "lxd",
		Attrs: domainstorage.Attrs{"pool": "default"},
	})
}

func (s *storagePoolStateSuite) TestReplaceStoragePoolRemovesAttributes(c *tc.C) {
	poolUUID := s.newStoragePool(c, "ebs-fast", "ebs", domainstorage.Attrs{
		"type": "fast", "stale": "remove me",
	})
	st := NewState(s.TxnRunnerFactory())
	replacement := domainstorage.StoragePool{
		UUID: poolUUID.String(), Name: "ebs-fast", Provider: "ebs",
		Attrs: domainstorage.Attrs{"type": "slow"},
	}
	c.Assert(st.ReplaceStoragePool(c.Context(), replacement), tc.ErrorIsNil)
	pool, err := st.GetStoragePool(c.Context(), poolUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(pool, tc.DeepEquals, replacement)
}

func (s *storagePoolStateSuite) TestReplaceStoragePoolRenameInUse(c *tc.C) {
	poolUUID := s.newStoragePool(c, "used", "ebs", domainstorage.Attrs{"type": "fast"})
	st := NewState(s.TxnRunnerFactory())
	args := []domainstorage.RecommendedStoragePoolArg{{
		StoragePoolUUID: poolUUID,
		StorageKind:     domainstorage.StorageKindBlock,
	}}
	c.Assert(st.SetModelStoragePools(c.Context(), args), tc.ErrorIsNil)
	replacement := domainstorage.StoragePool{
		UUID: poolUUID.String(), Name: "renamed", Provider: "ebs",
		Attrs: domainstorage.Attrs{"type": "slow"},
	}
	c.Assert(st.ReplaceStoragePool(c.Context(), replacement), tc.ErrorIsNil)
	pool, err := st.GetStoragePool(c.Context(), poolUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(pool, tc.DeepEquals, replacement)
	c.Check(s.getModelStoragePools(c), tc.DeepEquals, []dbModelStoragePool{{
		StoragePoolUUID: poolUUID.String(), StorageKindID: int(domainstorage.StorageKindBlock),
	}})
	_, err = st.GetStoragePoolUUID(c.Context(), "used")
	c.Check(err, tc.ErrorIs, domainstorageerrors.StoragePoolNotFound)
	uuid, err := st.GetStoragePoolUUID(c.Context(), "renamed")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid, tc.Equals, poolUUID)
	c.Check(st.DeleteStoragePool(c.Context(), "renamed"), tc.ErrorIs, domainstorageerrors.StoragePoolInUse)
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
		Attrs:        domainstorage.Attrs{"stale": "remove me"},
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

func (s *storagePoolStateSuite) TestReplaceStoragePoolProviderUnused(c *tc.C) {
	poolUUID := s.newStoragePool(c, "unused", "ebs", domainstorage.Attrs{"type": "fast"})
	replacement := domainstorage.StoragePool{
		UUID:     poolUUID.String(),
		Name:     "unused",
		Provider: "lxd",
		Attrs:    domainstorage.Attrs{"pool": "default"},
	}
	st := NewState(s.TxnRunnerFactory())
	c.Assert(st.ReplaceStoragePool(c.Context(), replacement), tc.ErrorIsNil)
	pool, err := st.GetStoragePool(c.Context(), poolUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(pool, tc.DeepEquals, replacement)
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

	args := []domainstorage.RecommendedStoragePoolArg{
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

	err := st.SetModelStoragePools(ctx, []domainstorage.RecommendedStoragePoolArg{
		{
			StorageKind:     domainstorage.StorageKindFilesystem,
			StoragePoolUUID: uuid1,
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	err = st.SetModelStoragePools(ctx, []domainstorage.RecommendedStoragePoolArg{
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

	err := st.SetModelStoragePools(ctx, []domainstorage.RecommendedStoragePoolArg{
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

	err = st.SetModelStoragePools(ctx, []domainstorage.RecommendedStoragePoolArg{
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

func (s *storagePoolStateSuite) TestSetModelStoragePoolsDuplicatePairs(c *tc.C) {
	originalUUID := s.newStoragePool(c, "original", "ebs", nil)
	poolUUID := s.newStoragePool(c, "replacement", "ebs", nil)
	st := NewState(s.TxnRunnerFactory())
	c.Assert(st.SetModelStoragePools(c.Context(), []domainstorage.RecommendedStoragePoolArg{{
		StoragePoolUUID: originalUUID, StorageKind: domainstorage.StorageKindFilesystem,
	}}), tc.ErrorIsNil)
	arg := domainstorage.RecommendedStoragePoolArg{
		StoragePoolUUID: poolUUID, StorageKind: domainstorage.StorageKindBlock,
	}
	err := st.SetModelStoragePools(c.Context(), []domainstorage.RecommendedStoragePoolArg{arg, arg})
	c.Check(err, tc.NotNil)
	c.Check(s.getModelStoragePools(c), tc.DeepEquals, []dbModelStoragePool{{
		StoragePoolUUID: originalUUID.String(), StorageKindID: int(domainstorage.StorageKindFilesystem),
	}})
}

func (s *storagePoolStateSuite) TestSetModelStoragePoolsPartiallyMissing(c *tc.C) {
	poolUUID := s.newStoragePool(c, "existing", "ebs", nil)
	missingUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	st := NewState(s.TxnRunnerFactory())
	c.Assert(st.SetModelStoragePools(c.Context(), []domainstorage.RecommendedStoragePoolArg{{
		StoragePoolUUID: poolUUID, StorageKind: domainstorage.StorageKindFilesystem,
	}}), tc.ErrorIsNil)
	err := st.SetModelStoragePools(c.Context(), []domainstorage.RecommendedStoragePoolArg{
		{StoragePoolUUID: poolUUID, StorageKind: domainstorage.StorageKindBlock},
		{StoragePoolUUID: missingUUID, StorageKind: domainstorage.StorageKindFilesystem},
	})
	c.Check(err, tc.ErrorIs, domainstorageerrors.StoragePoolNotFound)
	c.Check(s.getModelStoragePools(c), tc.DeepEquals, []dbModelStoragePool{{
		StoragePoolUUID: poolUUID.String(), StorageKindID: int(domainstorage.StorageKindFilesystem),
	}})
}

func (s *storagePoolStateSuite) TestListStoragePools(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	pools := []domainstorage.StoragePool{
		{Name: "fast", Provider: "ebs", Attrs: domainstorage.Attrs{"type": "fast", "size": "10"}},
		{Name: "slow", Provider: "ebs", Attrs: domainstorage.Attrs{"type": "slow"}},
		{Name: "local", Provider: "lxd"},
	}
	for i, pool := range pools {
		origin := domainstorage.StoragePoolOriginUser
		if i == 2 {
			origin = domainstorage.StoragePoolOriginProviderDefault
		}
		c.Assert(st.CreateStoragePool(c.Context(), domainstorageinternal.CreateStoragePool{
			UUID: tc.Must(c, domainstorage.NewStoragePoolUUID), Name: pool.Name,
			ProviderType: domainstorage.ProviderType(pool.Provider), Attrs: pool.Attrs, Origin: origin,
		}), tc.ErrorIsNil)
	}
	for _, test := range []struct {
		name string
		list func(context.Context) ([]domainstorage.StoragePool, error)
		want []domainstorage.StoragePool
	}{
		{name: "all", list: st.ListStoragePools, want: pools},
		{
			name: "names",
			list: func(ctx context.Context) ([]domainstorage.StoragePool, error) {
				return st.ListStoragePoolsByNames(ctx, []string{"local", "fast", "fast", "missing", ""})
			},
			want: []domainstorage.StoragePool{pools[0], pools[2]},
		},
		{
			name: "providers",
			list: func(ctx context.Context) ([]domainstorage.StoragePool, error) {
				return st.ListStoragePoolsByProviders(ctx, []string{"ebs", "ebs", "missing", ""})
			},
			want: pools[:2],
		},
		{
			name: "names and providers intersect",
			list: func(ctx context.Context) ([]domainstorage.StoragePool, error) {
				return st.ListStoragePoolsByNamesAndProviders(ctx,
					[]string{"fast", "fast", "local", "missing", ""}, []string{"ebs", "ebs", "missing", ""})
			},
			want: pools[:1],
		},
		{
			name: "provider default",
			list: func(ctx context.Context) ([]domainstorage.StoragePool, error) {
				return st.ListStoragePoolsByNamesAndProviders(ctx, []string{"local"}, []string{"lxd"})
			},
			want: pools[2:],
		},
	} {
		c.Logf("%s", test.name)
		got, err := test.list(c.Context())
		c.Assert(err, tc.ErrorIsNil)
		c.Check(got, tc.SameContents, test.want)
	}
}

func (s *storagePoolStateSuite) TestListStoragePoolsEmptyModel(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	pools, err := st.ListStoragePools(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(pools, tc.HasLen, 0)
}

func (s *storagePoolStateSuite) TestListStoragePoolsEmptyFiltersAndNoMatches(c *tc.C) {
	s.newStoragePool(c, "existing", "ebs", domainstorage.Attrs{"type": "fast"})
	st := NewState(s.TxnRunnerFactory())
	for _, test := range []struct {
		name string
		list func(context.Context) ([]domainstorage.StoragePool, error)
	}{
		{"empty names", func(ctx context.Context) ([]domainstorage.StoragePool, error) {
			return st.ListStoragePoolsByNames(ctx, nil)
		}},
		{"empty providers", func(ctx context.Context) ([]domainstorage.StoragePool, error) {
			return st.ListStoragePoolsByProviders(ctx, nil)
		}},
		{"combined empty names", func(ctx context.Context) ([]domainstorage.StoragePool, error) {
			return st.ListStoragePoolsByNamesAndProviders(ctx, nil, []string{"ebs"})
		}},
		{"combined empty providers", func(ctx context.Context) ([]domainstorage.StoragePool, error) {
			return st.ListStoragePoolsByNamesAndProviders(ctx, []string{"existing"}, nil)
		}},
		{"missing names", func(ctx context.Context) ([]domainstorage.StoragePool, error) {
			return st.ListStoragePoolsByNames(ctx, []string{"missing"})
		}},
		{"missing providers", func(ctx context.Context) ([]domainstorage.StoragePool, error) {
			return st.ListStoragePoolsByProviders(ctx, []string{"lxd"})
		}},
		{"combined missing names", func(ctx context.Context) ([]domainstorage.StoragePool, error) {
			return st.ListStoragePoolsByNamesAndProviders(ctx, []string{"missing"}, []string{"ebs"})
		}},
		{"combined mismatched provider", func(ctx context.Context) ([]domainstorage.StoragePool, error) {
			return st.ListStoragePoolsByNamesAndProviders(ctx, []string{"existing"}, []string{"lxd"})
		}},
	} {
		c.Logf("%s", test.name)
		pools, err := test.list(c.Context())
		c.Assert(err, tc.ErrorIsNil)
		c.Check(pools, tc.HasLen, 0)
	}
}

func (s *storagePoolStateSuite) TestGetStoragePool(c *tc.C) {
	poolUUID := s.newStoragePool(c, "fast", "ebs", domainstorage.Attrs{"type": "fast", "size": "10"})
	emptyUUID := s.newStoragePool(c, "local", "lxd", nil)
	st := NewState(s.TxnRunnerFactory())
	for _, expected := range []domainstorage.StoragePool{
		{UUID: poolUUID.String(), Name: "fast", Provider: "ebs",
			Attrs: domainstorage.Attrs{"type": "fast", "size": "10"}},
		{UUID: emptyUUID.String(), Name: "local", Provider: "lxd"},
	} {
		pool, err := st.GetStoragePool(c.Context(), domainstorage.StoragePoolUUID(expected.UUID))
		c.Assert(err, tc.ErrorIsNil)
		c.Check(pool, tc.DeepEquals, expected)
	}
}

func (s *storagePoolStateSuite) TestGetStoragePoolUUID(c *tc.C) {
	poolUUID := s.newStoragePool(c, "fast", "ebs", nil)
	s.newStoragePool(c, "other", "lxd", nil)
	st := NewState(s.TxnRunnerFactory())
	uuid, err := st.GetStoragePoolUUID(c.Context(), "fast")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid, tc.Equals, poolUUID)
}

func (s *storagePoolStateSuite) TestGetStoragePoolUUIDNotFound(c *tc.C) {
	s.newStoragePool(c, "other", "ebs", nil)
	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetStoragePoolUUID(c.Context(), "missing")
	c.Check(err, tc.ErrorIs, domainstorageerrors.StoragePoolNotFound)
}

func (s *storagePoolStateSuite) TestGetStoragePoolNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	_, err := st.GetStoragePool(c.Context(), poolUUID)
	c.Check(err, tc.ErrorIs, domainstorageerrors.StoragePoolNotFound)
}

func (s *storagePoolStateSuite) TestGetStoragePoolProvidersByNamesEmptyAndNoMatch(c *tc.C) {
	s.newStoragePool(c, "existing", "ebs", nil)
	st := NewState(s.TxnRunnerFactory())
	for _, names := range [][]string{nil, {"missing"}} {
		providers, err := st.GetStoragePoolProvidersByNames(c.Context(), names)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(providers, tc.HasLen, 0)
	}
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
	providers, err := st.GetStoragePoolProvidersByNames(c.Context(), []string{"foo", "bar", "baz", "qux"})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(providers, tc.DeepEquals, map[string]string{
		"foo": "bar",
		"bar": "foo",
	})
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

func (s *storagePoolStateSuite) TestListStoragePoolNameUUIDs(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	poolUUID1 := tc.Must(c, domainstorage.NewStoragePoolUUID)
	poolUUID2 := tc.Must(c, domainstorage.NewStoragePoolUUID)
	err := st.CreateStoragePool(ctx, domainstorageinternal.CreateStoragePool{
		Name:         "pool-a",
		ProviderType: "lxd",
		Origin:       domainstorage.StoragePoolOriginUser,
		UUID:         poolUUID1,
	})
	c.Assert(err, tc.ErrorIsNil)
	err = st.CreateStoragePool(ctx, domainstorageinternal.CreateStoragePool{
		Name:         "pool-b",
		ProviderType: "ec2",
		Origin:       domainstorage.StoragePoolOriginUser,
		UUID:         poolUUID2,
	})
	c.Assert(err, tc.ErrorIsNil)

	got, err := st.GetStoragePoolUUIDsByName(ctx, []string{"pool-b", "pool-a", "pool-a", "missing"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.SameContents, []domainstorage.StoragePoolNameUUID{
		{
			Name: "pool-a",
			UUID: poolUUID1.String(),
		},
		{
			Name: "pool-b",
			UUID: poolUUID2.String(),
		},
	})
}

func (s *storagePoolStateSuite) TestListStoragePoolNameUUIDsEmpty(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	got, err := st.GetStoragePoolUUIDsByName(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.HasLen, 0)
}

func (s *storagePoolStateSuite) TestListStoragePoolNameUUIDsNoMatch(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	got, err := st.GetStoragePoolUUIDsByName(c.Context(), []string{"unknown"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.HasLen, 0)
}

func (s *storagePoolStateSuite) TestDeleteStoragePool(c *tc.C) {
	poolUUID := s.newStoragePool(c, "unused", "ebs", map[string]string{"type": "fast"})
	remainingUUID := s.newStoragePool(c, "remaining", "lxd", domainstorage.Attrs{"pool": "default"})
	st := NewState(s.TxnRunnerFactory())
	c.Assert(st.DeleteStoragePool(c.Context(), "unused"), tc.ErrorIsNil)
	_, err := st.GetStoragePool(c.Context(), poolUUID)
	c.Check(err, tc.ErrorIs, domainstorageerrors.StoragePoolNotFound)
	var count int
	err = s.DB().QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM storage_pool_attribute WHERE storage_pool_uuid = ?", poolUUID.String(),
	).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
	pools, err := st.ListStoragePools(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(pools, tc.DeepEquals, []domainstorage.StoragePool{{
		Name: "remaining", Provider: "lxd", Attrs: domainstorage.Attrs{"pool": "default"},
	}})
	pairs, err := st.GetStoragePoolUUIDsByName(c.Context(), []string{"unused", "remaining"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(pairs, tc.DeepEquals, []domainstorage.StoragePoolNameUUID{{
		Name: "remaining", UUID: remainingUUID.String(),
	}})
}

func (s *storagePoolStateSuite) TestDeleteStoragePoolNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.DeleteStoragePool(c.Context(), "missing")
	c.Check(err, tc.ErrorIs, domainstorageerrors.StoragePoolNotFound)
}

func (s *storagePoolStateSuite) TestDeleteStoragePoolModelDefault(c *tc.C) {
	poolUUID := s.newStoragePool(c, "used", "ebs", map[string]string{"type": "fast"})
	_, err := s.DB().ExecContext(c.Context(),
		"INSERT INTO model_storage_pool (storage_kind_id, storage_pool_uuid) VALUES (0, ?)", poolUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	s.assertPoolInUse(c, poolUUID)
}

func (s *storagePoolStateSuite) TestStoragePoolInUseModelConfigBlock(c *tc.C) {
	s.assertStoragePoolModelConfigInUse(c, "storage-default-block-source")
}

func (s *storagePoolStateSuite) TestStoragePoolInUseModelConfigFilesystem(c *tc.C) {
	s.assertStoragePoolModelConfigInUse(c, "storage-default-filesystem-source")
}

func (s *storagePoolStateSuite) assertStoragePoolModelConfigInUse(c *tc.C, key string) {
	poolUUID := s.newStoragePool(c, "used", "ebs", domainstorage.Attrs{"type": "fast"})
	_, err := s.DB().ExecContext(c.Context(),
		"INSERT INTO model_config (key, value) VALUES (?, 'used')", key)
	c.Assert(err, tc.ErrorIsNil)
	s.assertPoolInUse(c, poolUUID)
	var value string
	err = s.DB().QueryRowContext(c.Context(),
		"SELECT value FROM model_config WHERE key = ?", key).Scan(&value)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(value, tc.Equals, "used")

	_, err = s.DB().ExecContext(c.Context(),
		"DELETE FROM model_config WHERE key = ?", key)
	c.Assert(err, tc.ErrorIsNil)
	st := NewState(s.TxnRunnerFactory())
	c.Assert(st.ReplaceStoragePool(c.Context(), domainstorage.StoragePool{
		UUID:     poolUUID.String(),
		Name:     "used",
		Provider: "lxd",
	}), tc.ErrorIsNil)
	c.Assert(st.DeleteStoragePool(c.Context(), "used"), tc.ErrorIsNil)
}

func (s *storagePoolStateSuite) TestStoragePoolUnusedUnrelatedModelConfig(c *tc.C) {
	poolUUID := s.newStoragePool(c, "unused", "ebs", domainstorage.Attrs{"type": "fast"})
	_, err := s.DB().ExecContext(c.Context(), `
INSERT INTO model_config (key, value)
VALUES ('storage-default-block-source', 'other'),
       ('storage-default-filesystem-source', 'other'),
       ('unrelated', 'unused')
`)
	c.Assert(err, tc.ErrorIsNil)
	st := NewState(s.TxnRunnerFactory())
	c.Assert(st.ReplaceStoragePool(c.Context(), domainstorage.StoragePool{
		UUID:     poolUUID.String(),
		Name:     "unused",
		Provider: "lxd",
	}), tc.ErrorIsNil)
	c.Assert(st.DeleteStoragePool(c.Context(), "unused"), tc.ErrorIsNil)
}

func (s *storagePoolStateSuite) TestDeleteStoragePoolStorageInstance(c *tc.C) {
	poolUUID := s.newStoragePool(c, "used", "ebs", map[string]string{"type": "fast"})
	charmUUID := s.newCharm(c)
	s.newBlockStorageInstanceForCharmWithPool(c, charmUUID, poolUUID, "data")
	s.assertPoolInUse(c, poolUUID)
}

func (s *storagePoolStateSuite) TestDeleteStoragePoolApplicationDirective(c *tc.C) {
	s.assertStorageDirectivePoolInUse(c, false)
}

func (s *storagePoolStateSuite) TestDeleteStoragePoolUnitDirective(c *tc.C) {
	s.assertStorageDirectivePoolInUse(c, true)
}

func (s *storagePoolStateSuite) assertStorageDirectivePoolInUse(c *tc.C, unitDirective bool) {
	poolUUID := s.newStoragePool(c, "used", "ebs", map[string]string{"type": "fast"})
	appUUID, charmUUID := s.newApplication(c, "test")
	_, err := s.DB().ExecContext(c.Context(), `
INSERT INTO charm_storage (charm_uuid, name, storage_kind_id, shared, count_min, count_max)
VALUES (?, 'data', 0, false, 1, 1)
`, charmUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	if unitDirective {
		unitUUID, _, _ := s.newUnitForApplication(c, appUUID)
		_, err = s.DB().ExecContext(c.Context(), `
INSERT INTO unit_storage_directive
    (unit_uuid, charm_uuid, storage_name, storage_pool_uuid, size_mib, count)
VALUES (?, ?, 'data', ?, 100, 1)
`, unitUUID.String(), charmUUID.String(), poolUUID.String())
	} else {
		_, err = s.DB().ExecContext(c.Context(), `
INSERT INTO application_storage_directive
    (application_uuid, charm_uuid, storage_name, storage_pool_uuid, size_mib, count)
VALUES (?, ?, 'data', ?, 100, 1)
`, appUUID.String(), charmUUID.String(), poolUUID.String())
	}
	c.Assert(err, tc.ErrorIsNil)
	s.assertPoolInUse(c, poolUUID)
}

func (s *storagePoolStateSuite) TestDeleteStoragePoolOperatorStorage(c *tc.C) {
	poolUUID := s.setupOperatorStoragePool(c, "caas", true)
	s.assertPoolInUse(c, poolUUID)
}

func (s *storagePoolStateSuite) TestDeleteStoragePoolOperatorStorageNoApplications(c *tc.C) {
	s.setupOperatorStoragePool(c, "caas", false)
	st := NewState(s.TxnRunnerFactory())
	c.Check(st.DeleteStoragePool(c.Context(), "used"), tc.ErrorIsNil)
}

func (s *storagePoolStateSuite) TestDeleteStoragePoolOperatorStorageIAAS(c *tc.C) {
	s.setupOperatorStoragePool(c, "iaas", true)
	st := NewState(s.TxnRunnerFactory())
	c.Check(st.DeleteStoragePool(c.Context(), "used"), tc.ErrorIsNil)
}

func (s *storagePoolStateSuite) setupOperatorStoragePool(
	c *tc.C, modelType string, withApplication bool,
) domainstorage.StoragePoolUUID {
	poolUUID := s.newStoragePool(c, "used", "ebs", map[string]string{"type": "fast"})
	_, err := s.DB().ExecContext(c.Context(), `
INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
VALUES (?, ?, 'test', 'test', ?, 'test', 'test')
`, s.ModelUUID(), s.ModelUUID(), modelType)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(c.Context(),
		"INSERT INTO model_config (key, value) VALUES ('operator-storage', 'used')")
	c.Assert(err, tc.ErrorIsNil)
	if withApplication {
		s.newApplication(c, "test")
	}
	return poolUUID
}

func (s *storagePoolStateSuite) assertPoolInUse(c *tc.C, poolUUID domainstorage.StoragePoolUUID) {
	st := NewState(s.TxnRunnerFactory())
	err := st.DeleteStoragePool(c.Context(), "used")
	c.Check(err, tc.ErrorIs, domainstorageerrors.StoragePoolInUse)
	replacement := domainstorage.StoragePool{
		UUID:     poolUUID.String(),
		Name:     "used",
		Provider: "lxd",
		Attrs:    domainstorage.Attrs{"pool": "default"},
	}
	err = st.ReplaceStoragePool(c.Context(), replacement)
	c.Check(err, tc.ErrorIs, domainstorageerrors.StoragePoolInUse)
	pool, err := st.GetStoragePool(c.Context(), poolUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(pool.Provider, tc.Equals, "ebs")
	c.Check(pool.Attrs, tc.DeepEquals, domainstorage.Attrs{"type": "fast"})

	replacement.Provider = "ebs"
	replacement.Attrs = domainstorage.Attrs{"type": "slow"}
	c.Assert(st.ReplaceStoragePool(c.Context(), replacement), tc.ErrorIsNil)
	pool, err = st.GetStoragePool(c.Context(), poolUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(pool, tc.DeepEquals, replacement)
}
