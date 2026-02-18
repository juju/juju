// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"errors"
	stdtesting "testing"

	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/domain/life"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/domain/storage/internal"
	domainstorageinternal "github.com/juju/juju/domain/storage/internal"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalstorage "github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

// importSuite is a set of tests to assert the interface and contracts
// importing storage into this state package.
type importSuite struct {
	testhelpers.IsolationSuite

	service *Service

	state                   *MockState
	storageProvider         *MockProvider
	storageProviderRegistry *MockProviderRegistry
	storageRegistryGetter   *MockModelStorageRegistryGetter
}

// TestImportSuite runs all of the tests contained in
// [importSuite].
func TestImportSuite(t *stdtesting.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) TestImportStorageInstances(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	expected := []internal.ImportStorageInstanceArgs{
		{
			UUID:             tc.Must(c, uuid.NewUUID).String(),
			StorageName:      "test1",
			StorageKind:      "block",
			StorageID:        "test1/0",
			UnitName:         "unit/3",
			RequestedSizeMiB: 1024,
			PoolName:         "ebs",
		}, {
			UUID:             tc.Must(c, uuid.NewUUID).String(),
			StorageName:      "test1",
			StorageKind:      "block",
			StorageID:        "test1/2",
			UnitName:         "unit/2",
			RequestedSizeMiB: 1024,
			PoolName:         "ebs",
		},
	}
	s.state.EXPECT().ImportStorageInstances(gomock.Any(), ignoreUUIDArgsMatcher[internal.ImportStorageInstanceArgs]{
		c:        c,
		expected: expected,
	}).Return(nil)

	args := []domainstorage.ImportStorageInstanceParams{
		{
			StorageName:      "test1",
			StorageKind:      "block",
			StorageID:        "test1/0",
			UnitName:         "unit/3",
			RequestedSizeMiB: 1024,
			PoolName:         "ebs",
		}, {
			StorageName:      "test1",
			StorageKind:      "block",
			StorageID:        "test1/2",
			UnitName:         "unit/2",
			RequestedSizeMiB: 1024,
			PoolName:         "ebs",
		},
	}

	// Act
	err := s.service.ImportStorageInstances(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportStorageInstancesValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	args := []domainstorage.ImportStorageInstanceParams{
		{
			// There is not StorageID.
			StorageName:      "test1",
			StorageKind:      "block",
			UnitName:         "unit/2",
			RequestedSizeMiB: uint64(1024),
			PoolName:         "ebs",
		},
	}

	// Act
	err := s.service.ImportStorageInstances(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestImportFilesystems(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	args := []domainstorage.ImportFilesystemParams{{
		ID:                "test-1/0",
		PoolName:          "ebs",
		SizeInMiB:         1024,
		ProviderID:        "provider-test-1/0",
		StorageInstanceID: "storageinstance/1",
	}, {
		ID:                "test-2/1",
		PoolName:          "ebs-ssd",
		SizeInMiB:         2048,
		ProviderID:        "provider-test-2/1",
		StorageInstanceID: "storageinstance/2",
	}, {
		ID:         "test-3/2",
		PoolName:   "tmpfs",
		SizeInMiB:  4096,
		ProviderID: "provider-test-3/2",
		// sometimes filesystems are not associated with a storage instance
		StorageInstanceID: "",
	}}

	s.state.EXPECT().GetStoragePoolProvidersByNames(gomock.Any(), []string{"ebs", "ebs-ssd", "tmpfs"}).Return(map[string]string{
		"ebs":     "ebs",
		"ebs-ssd": "ebs",
		"tmpfs":   "tmpfs",
	}, nil)

	ebsProvider := NewMockProvider(ctrl)
	ebsProvider.EXPECT().Scope().Return(internalstorage.ScopeEnviron).AnyTimes()
	ebsProvider.EXPECT().Supports(internalstorage.StorageKindBlock).Return(true).AnyTimes()
	ebsProvider.EXPECT().Supports(internalstorage.StorageKindFilesystem).Return(false).AnyTimes()

	tmpfsProvider := NewMockProvider(ctrl)
	tmpfsProvider.EXPECT().Scope().Return(internalstorage.ScopeMachine).AnyTimes()
	tmpfsProvider.EXPECT().Supports(internalstorage.StorageKindBlock).Return(false).AnyTimes()
	tmpfsProvider.EXPECT().Supports(internalstorage.StorageKindFilesystem).Return(true).AnyTimes()

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil)
	s.storageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("ebs")).Return(ebsProvider, nil).Times(2)
	s.storageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("tmpfs")).Return(tmpfsProvider, nil)

	s.state.EXPECT().GetStorageInstanceUUIDsByIDs(gomock.Any(), []string{"storageinstance/1", "storageinstance/2"}).
		Return(map[string]string{
			"storageinstance/1": "storageinstance-uuid-1",
			"storageinstance/2": "storageinstance-uuid-2",
		}, nil)

	expected := []internal.ImportFilesystemArgs{{
		ID:                  "test-1/0",
		Life:                life.Alive,
		SizeInMiB:           1024,
		ProviderID:          "provider-test-1/0",
		StorageInstanceUUID: "storageinstance-uuid-1",
		Scope:               domainstorageprovisioning.ProvisionScopeMachine,
	}, {
		ID:                  "test-2/1",
		Life:                life.Alive,
		SizeInMiB:           2048,
		ProviderID:          "provider-test-2/1",
		StorageInstanceUUID: "storageinstance-uuid-2",
		Scope:               domainstorageprovisioning.ProvisionScopeMachine,
	}, {
		ID:         "test-3/2",
		Life:       life.Alive,
		SizeInMiB:  4096,
		ProviderID: "provider-test-3/2",
		Scope:      domainstorageprovisioning.ProvisionScopeMachine,
	}}
	s.state.EXPECT().ImportFilesystems(gomock.Any(), ignoreUUIDArgsMatcher[internal.ImportFilesystemArgs]{
		c:        c,
		expected: expected,
	}).Return(nil)

	err := s.service.ImportFilesystems(c.Context(), args)

	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportFilesystemsValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := []domainstorage.ImportFilesystemParams{{}}
	err := s.service.ImportFilesystems(c.Context(), args)

	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestImportStoragePools tests the happy path where a single storage pool
// is validated and created successfully.
func (s *importSuite) TestImportStoragePools(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil).Times(2)
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().
		Return([]internalstorage.ProviderType{"storageprovider1"}, nil)
	s.storageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("storageprovider1")).Return(s.storageProvider, nil).
		Times(2)
	s.storageProvider.EXPECT().ValidateConfig(gomock.Any())
	s.storageProvider.EXPECT().DefaultPools()
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindFilesystem)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindBlock)

	input := []domainstorage.UserStoragePoolParams{
		{
			Name:     "my-pool",
			Provider: "storageprovider1",
			Attributes: map[string]interface{}{
				"key": "val",
			},
		},
	}

	s.state.EXPECT().
		CreateStoragePool(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, pool domainstorageinternal.CreateStoragePool) error {
		c.Assert(pool.UUID, tc.IsUUID)
		c.Assert(pool.Name, tc.Equals, "my-pool")
		c.Assert(pool.ProviderType, tc.Equals, domainstorage.ProviderType("storageprovider1"))
		c.Assert(pool.Origin, tc.Equals, domainstorage.StoragePoolOriginUser)
		c.Assert(pool.Attrs, tc.DeepEquals, map[string]string{
			"key": "val",
		})
		return nil
	})
	s.state.EXPECT().SetModelStoragePools(gomock.Any(), []domainstorage.RecommendedStoragePoolArg{})

	err := s.service.ImportStoragePools(
		c.Context(),
		input,
	)

	c.Check(err, tc.ErrorIsNil)
}

// TestImportStoragePoolsMultipleSuccess tests that multiple storage pools
// are validated and created successfully when no errors occur.
// One pool is user supplied and the other is provider default.
func (s *importSuite) TestImportStoragePoolsMultipleSuccess(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	cfg, err := internalstorage.NewConfig(
		"lxd-btrfs",
		"lxd",
		internalstorage.Attrs{"b": "true"},
	)
	c.Assert(err, tc.ErrorIsNil)

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil).Times(3)
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().
		Return([]internalstorage.ProviderType{"lxd"}, nil)
	s.storageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("lxd")).Return(s.storageProvider, nil).
		Times(3)
	s.storageProvider.EXPECT().ValidateConfig(gomock.Any()).Times(2)
	s.storageProvider.EXPECT().DefaultPools().Return([]*internalstorage.Config{cfg})
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindFilesystem)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindBlock)
	s.state.EXPECT().SetModelStoragePools(gomock.Any(), []domainstorage.RecommendedStoragePoolArg{})

	gomock.InOrder(
		s.state.EXPECT().
			CreateStoragePool(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, pool domainstorageinternal.CreateStoragePool) error {
				c.Assert(pool.UUID, tc.IsUUID)
				c.Assert(pool.Name, tc.Equals, "lxd")
				c.Assert(pool.ProviderType, tc.Equals,
					domainstorage.ProviderType("lxd"))
				c.Assert(pool.Origin, tc.Equals, domainstorage.StoragePoolOriginUser)
				c.Assert(pool.Attrs, tc.DeepEquals, map[string]string{
					"a": "1",
				})
				return nil
			}),
		s.state.EXPECT().
			CreateStoragePool(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, pool domainstorageinternal.CreateStoragePool) error {
				// This is a provider default pool so we know the UUID.
				c.Assert(pool.UUID, tc.Equals,
					domainstorage.StoragePoolUUID("e1acb8b8-c978-5d53-bc22-2a0e7fd58734"))
				c.Assert(pool.Name, tc.Equals, "lxd-btrfs")
				c.Assert(pool.ProviderType, tc.Equals,
					domainstorage.ProviderType("lxd"))
				c.Assert(pool.Origin, tc.Equals,
					domainstorage.StoragePoolOriginProviderDefault)
				c.Assert(pool.Attrs, tc.DeepEquals, map[string]string{
					"b": "true",
				})
				return nil
			}),
	)
	input := []domainstorage.UserStoragePoolParams{
		{
			Name:       "lxd",
			Provider:   "lxd",
			Attributes: map[string]any{"a": 1},
		},
	}

	err = s.service.ImportStoragePools(
		c.Context(),
		input,
	)

	c.Check(err, tc.ErrorIsNil)
}

// TestImportStoragePoolsInvalidProviderType tests that an invalid provider type
// returns [domainstorageerrors.ProviderTypeInvalid].
func (s *importSuite) TestImportStoragePoolsInvalidProviderType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil)
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().
		Return([]internalstorage.ProviderType{"-invalid-provider-"}, nil)
	s.storageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("-invalid-provider-")).
		Return(s.storageProvider, nil)
	s.storageProvider.EXPECT().DefaultPools().Return([]*internalstorage.Config{})
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindFilesystem)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindBlock)

	input := []domainstorage.UserStoragePoolParams{
		{
			Name:     "my-pool",
			Provider: "-invalid-provider-",
		},
	}

	err := s.service.ImportStoragePools(
		c.Context(),
		input,
	)

	c.Check(err, tc.ErrorIs, domainstorageerrors.ProviderTypeInvalid)
}

// TestImportStoragePoolsProviderTypeNotFound tests that importing a storage
// pool for a provider not present in the registry returns
// [domainstorageerrors.ProviderTypeNotFound].
func (s *importSuite) TestImportStoragePoolsProviderTypeNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil).Times(2)
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().
		Return([]internalstorage.ProviderType{"storagep1"}, nil)
	s.storageProviderRegistry.EXPECT().
		StorageProvider(internalstorage.ProviderType("storagep1")).
		Return(s.storageProvider, nil)
	s.storageProvider.EXPECT().DefaultPools().Return([]*internalstorage.Config{})
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindFilesystem)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindBlock)
	s.storageProviderRegistry.EXPECT().
		StorageProvider(internalstorage.ProviderType("storagep1")).
		Return(nil, coreerrors.NotFound)

	input := []domainstorage.UserStoragePoolParams{
		{
			Name:     "my-pool",
			Provider: "storagep1",
		},
	}

	err := s.service.ImportStoragePools(
		c.Context(),
		input,
	)

	c.Check(err, tc.ErrorIs, domainstorageerrors.ProviderTypeNotFound)
}

// TestImportStoragePoolsProviderRegistryError tests that unexpected
// errors returned by the provider registry are propagated.
func (s *importSuite) TestImportStoragePoolsProviderRegistryError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	registryErr := errors.New("registry failure")

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil).Times(2)
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().
		Return([]internalstorage.ProviderType{"storagep1"}, nil)
	s.storageProviderRegistry.EXPECT().
		StorageProvider(internalstorage.ProviderType("storagep1")).
		Return(s.storageProvider, nil)
	s.storageProvider.EXPECT().DefaultPools().Return([]*internalstorage.Config{})
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindFilesystem)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindBlock)
	s.storageProviderRegistry.EXPECT().
		StorageProvider(internalstorage.ProviderType("storageprovider1")).
		Return(nil, registryErr)

	input := []domainstorage.UserStoragePoolParams{
		{
			Name:     "my-pool",
			Provider: "storageprovider1",
		},
	}

	err := s.service.ImportStoragePools(
		c.Context(),
		input,
	)

	c.Check(err, tc.ErrorIs, registryErr)
}

// TestImportStoragePoolsInvalidName tests that an invalid legacy storage
// pool name returns [domainstorageerrors.StoragePoolNameInvalid].
func (s *importSuite) TestImportStoragePoolsInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil)
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().
		Return([]internalstorage.ProviderType{"ebs"}, nil)
	s.storageProviderRegistry.EXPECT().
		StorageProvider(internalstorage.ProviderType("ebs")).
		Return(s.storageProvider, nil)
	s.storageProvider.EXPECT().DefaultPools().Return([]*internalstorage.Config{})
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindFilesystem)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindBlock)

	input := []domainstorage.UserStoragePoolParams{
		{
			// Must start with a letter.
			Name:     "66invalid",
			Provider: "ebs",
		},
	}
	err := s.service.ImportStoragePools(
		c.Context(),
		input,
	)

	c.Check(err, tc.ErrorIs, domainstorageerrors.StoragePoolNameInvalid)
}

// TestSetRecommendedStoragePools tests that the service correctly converts
// recommended storage pool parameters into model arguments and delegates
// persistence to the state layer without error.
func (s *importSuite) TestSetRecommendedStoragePools(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	uuid1 := tc.Must(c, domainstorage.NewStoragePoolUUID)
	uuid2 := tc.Must(c, domainstorage.NewStoragePoolUUID)

	input := []domainstorage.RecommendedStoragePoolParams{
		{
			StorageKind:     domainstorage.StorageKindFilesystem,
			StoragePoolUUID: uuid1,
		},
		{
			StorageKind:     domainstorage.StorageKindBlock,
			StoragePoolUUID: uuid2,
		},
	}

	s.state.EXPECT().SetModelStoragePools(gomock.Any(), []domainstorage.RecommendedStoragePoolArg{
		{
			StorageKind:     domainstorage.StorageKindFilesystem,
			StoragePoolUUID: uuid1,
		},
		{
			StorageKind:     domainstorage.StorageKindBlock,
			StoragePoolUUID: uuid2,
		},
	})

	err := s.service.setRecommendedStoragePools(c.Context(), input)
	c.Assert(err, tc.ErrorIsNil)
}

// TestSetRecommendedStoragePoolsPoolNotFound tests that the service propagates
// a [domainstorageerrors.StoragePoolNotFound] error returned by the state layer
// when a referenced storage pool does not exist.
func (s *importSuite) TestSetRecommendedStoragePoolsPropagatesError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	expectedErr := domainstorageerrors.StoragePoolNotFound

	s.state.EXPECT().SetModelStoragePools(gomock.Any(), gomock.Any()).Return(
		domainstorageerrors.StoragePoolNotFound)

	err := s.service.setRecommendedStoragePools(c.Context(), []domainstorage.RecommendedStoragePoolParams{
		{
			StorageKind:     domainstorage.StorageKindFilesystem,
			StoragePoolUUID: tc.Must(c, domainstorage.NewStoragePoolUUID),
		},
	})

	c.Check(err, tc.ErrorIs, expectedErr)
}

// TestGetStoragePoolsToImport tests that both user-defined storage pools
// and provider default pools are returned and no recommended storage pools are returned.
func (s *importSuite) TestGetStoragePoolsToImport(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	cfg, err := internalstorage.NewConfig(
		"lxd",
		"lxd",
		internalstorage.Attrs{"foo": "bar"},
	)
	c.Assert(err, tc.ErrorIsNil)

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil)
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().
		Return([]internalstorage.ProviderType{"lxd"}, nil)
	s.storageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("lxd")).Return(s.storageProvider, nil)
	s.storageProvider.EXPECT().DefaultPools().Return([]*internalstorage.Config{cfg})
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindFilesystem)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindBlock)

	input := []domainstorage.UserStoragePoolParams{
		{
			Name:       "custom-pool",
			Provider:   "lxd",
			Attributes: nil,
		},
	}

	pools, recommended, err := s.service.getStoragePoolsToImport(
		c.Context(),
		input,
	)

	mc := tc.NewMultiChecker()
	mc.AddExpr("_.UUID", tc.IsUUID)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(recommended, tc.HasLen, 0)
	c.Assert(pools, tc.HasLen, 2)
	c.Assert(pools[0], tc.Bind(mc, domainstorage.ImportStoragePoolParams{
		Name:  "custom-pool",
		Type:  "lxd",
		Attrs: nil,
	}))
	c.Assert(pools[1], tc.DeepEquals, domainstorage.ImportStoragePoolParams{
		UUID:   "16d8c090-8ef4-59b4-8e88-0bc64a0598a3",
		Name:   "lxd",
		Type:   "lxd",
		Origin: domainstorage.StoragePoolOriginProviderDefault,
		Attrs:  map[string]any{"foo": "bar"},
	})
}

// TestGetStoragePoolsToImportUserPoolsOnly verifies that when only user-defined
// storage pools are present and that there are no provider default pools, the
// service returns user-defined pools, generates UUIDs, and does not include provider default or
// recommended pools.
func (s *importSuite) TestGetStoragePoolsToImportUserPoolsOnly(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil)
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().
		Return([]internalstorage.ProviderType{"lxd"}, nil)
	s.storageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("lxd")).Return(s.storageProvider, nil)
	s.storageProvider.EXPECT().DefaultPools()
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindFilesystem)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindBlock)

	input := []domainstorage.UserStoragePoolParams{{
		Name:       "user-pool",
		Provider:   "lxd",
		Attributes: map[string]any{"foo": "bar"},
	}}
	pools, recommended, err := s.service.getStoragePoolsToImport(
		c.Context(),
		input,
	)

	mc := tc.NewMultiChecker()
	mc.AddExpr("_.UUID", tc.IsUUID)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(recommended, tc.HasLen, 0)
	c.Assert(pools, tc.HasLen, 1)
	c.Assert(pools[0], tc.Bind(mc, domainstorage.ImportStoragePoolParams{
		Name:   "user-pool",
		Type:   "lxd",
		Origin: domainstorage.StoragePoolOriginUser,
		Attrs:  map[string]any{"foo": "bar"},
	}))
}

// TestGetStoragePoolsToImportPickUserDefinedOnDuplicate ensures that when a user-defined storage
// pool conflicts by name and provider with a provider default pool, the user-defined
// pool is preferred and the conflicting default pool is skipped.
func (s *importSuite) TestGetStoragePoolsToImportPickUserDefinedOnDuplicate(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	cfg1, err := internalstorage.NewConfig(
		"lxd-btrfs",
		"lxd",
		nil,
	)
	c.Assert(err, tc.ErrorIsNil)
	cfg2, err := internalstorage.NewConfig(
		"lxd-zfs",
		"lxd",
		nil,
	)
	c.Assert(err, tc.ErrorIsNil)

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil)
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().
		Return([]internalstorage.ProviderType{"lxd"}, nil)
	s.storageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("lxd")).Return(s.storageProvider, nil)
	s.storageProvider.EXPECT().DefaultPools().Return([]*internalstorage.Config{
		cfg1,
		cfg2,
	})
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindFilesystem)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindBlock)

	input := []domainstorage.UserStoragePoolParams{
		// This pool conflicts with a provider default pool.
		{
			Name:       "lxd-btrfs",
			Provider:   "lxd",
			Attributes: nil,
		},
		{
			Name:       "custom-pool",
			Provider:   "lxd",
			Attributes: nil,
		},
	}

	pools, _, err := s.service.getStoragePoolsToImport(
		c.Context(),
		input,
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pools, tc.HasLen, 3)

	mc := tc.NewMultiChecker()
	mc.AddExpr("_.UUID", tc.IsUUID)

	// User pool: lxd-btrfs.
	c.Assert(pools[0], tc.Bind(mc, domainstorage.ImportStoragePoolParams{
		Name:   "lxd-btrfs",
		Type:   "lxd",
		Origin: domainstorage.StoragePoolOriginUser,
		Attrs:  nil,
	}))

	// User pool: custom-pool.
	c.Assert(pools[1], tc.Bind(mc, domainstorage.ImportStoragePoolParams{
		Name:   "custom-pool",
		Type:   "lxd",
		Origin: domainstorage.StoragePoolOriginUser,
		Attrs:  nil,
	}))

	// Provider default (non-conflicting): lxd-zfs.
	c.Assert(pools[2], tc.DeepEquals, domainstorage.ImportStoragePoolParams{
		// Use the hardcoded UUID for this pool.
		UUID:   "635f1873-be0b-5f07-b841-9fa02466a9f6",
		Name:   "lxd-zfs",
		Type:   "lxd",
		Origin: domainstorage.StoragePoolOriginProviderDefault,
		Attrs:  nil,
	})
}

// TestGetStoragePoolsToImportReturnsRecommendedPools verifies that provider default
// pools are added and recommended storage pools are returned when the registry
// supplies recommendations for specific storage kinds.
func (s *importSuite) TestGetStoragePoolsToImportReturnsRecommendedPools(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	defaultPool, _ := internalstorage.NewConfig("lxd", "lxd", internalstorage.Attrs{})
	zfsPool, _ := internalstorage.NewConfig("lxd-zfs", "lxd", internalstorage.Attrs{
		"driver":        "zfs",
		"lxd-pool":      "juju-zfs",
		"zfs.pool_name": "juju-lxd",
	})
	btrfsPool, _ := internalstorage.NewConfig("lxd-btrfs", "lxd", internalstorage.Attrs{
		"driver":   "btrfs",
		"lxd-pool": "juju-btrfs",
	})
	recommendedPoolForBlock, _ := internalstorage.NewConfig("loop", "loop",
		internalstorage.Attrs{})
	lxdDefaultPools := []*internalstorage.Config{defaultPool, zfsPool, btrfsPool}

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).Return(
		s.storageProviderRegistry, nil)
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().Return(
		[]internalstorage.ProviderType{"lxd"}, nil,
	)
	s.storageProviderRegistry.EXPECT().StorageProvider(internalstorage.ProviderType("lxd")).
		Return(s.storageProvider, nil)
	s.storageProvider.EXPECT().DefaultPools().Return(lxdDefaultPools)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(internalstorage.StorageKindFilesystem).
		Return(defaultPool)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(internalstorage.StorageKindBlock).
		Return(recommendedPoolForBlock)

	input := []domainstorage.UserStoragePoolParams{
		{
			Name:       "custom-pool",
			Provider:   "lxd",
			Attributes: map[string]any{"foo": "bar"},
		},
	}
	pools, recommended, err := s.service.getStoragePoolsToImport(
		c.Context(),
		input,
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(recommended, tc.SameContents, []domainstorage.RecommendedStoragePoolParams{
		// This is a pool with name: lxd and provider: lxd.
		{
			StoragePoolUUID: "16d8c090-8ef4-59b4-8e88-0bc64a0598a3",
			StorageKind:     domainstorage.StorageKindFilesystem,
		},
		// This is a pool with name: loop and provider: loop.
		{
			StoragePoolUUID: "baa26e04-b1f0-50d9-9bf8-4d5a78ffe6ad",
			StorageKind:     domainstorage.StorageKindBlock,
		},
	})

	mc := tc.NewMultiChecker()
	mc.AddExpr("_.UUID", tc.IsUUID)

	c.Assert(pools, tc.HasLen, 5)
	// User pool. The UUID is generated by the service so we don't know the exact
	// value, but we assert that it is a UUID.
	c.Assert(pools[0], tc.Bind(mc, domainstorage.ImportStoragePoolParams{
		Name:   "custom-pool",
		Type:   "lxd",
		Origin: domainstorage.StoragePoolOriginUser,
		Attrs:  map[string]any{"foo": "bar"},
	}))
	// The rest are provider default pools.
	c.Assert(pools[1:], tc.SameContents, []domainstorage.ImportStoragePoolParams{
		{
			UUID:   "16d8c090-8ef4-59b4-8e88-0bc64a0598a3",
			Name:   "lxd",
			Type:   "lxd",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Attrs:  map[string]any{},
		},
		{
			UUID:   "635f1873-be0b-5f07-b841-9fa02466a9f6",
			Name:   "lxd-zfs",
			Type:   "lxd",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Attrs: map[string]any{
				"driver":        "zfs",
				"lxd-pool":      "juju-zfs",
				"zfs.pool_name": "juju-lxd",
			},
		},
		{
			UUID:   "e1acb8b8-c978-5d53-bc22-2a0e7fd58734",
			Name:   "lxd-btrfs",
			Type:   "lxd",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Attrs: map[string]any{
				"driver":   "btrfs",
				"lxd-pool": "juju-btrfs",
			},
		},
		{
			UUID:   "baa26e04-b1f0-50d9-9bf8-4d5a78ffe6ad",
			Name:   "loop",
			Type:   "loop",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Attrs:  map[string]any{},
		},
	})
}

// TestGetStoragePoolsToImportExcludeConflictingUserPool tests that if there is a recommended provider default
// pool with conflicting name with a user-defined pool, then that pool will not be included
// in the recommended pools because we cannot guarantee they refer to the same pool.
func (s *importSuite) TestGetStoragePoolsToImportExcludeConflictingUserPool(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	defaultPool, _ := internalstorage.NewConfig("lxd", "lxd", internalstorage.Attrs{})
	zfsPool, _ := internalstorage.NewConfig("lxd-zfs", "lxd", internalstorage.Attrs{
		"driver":        "zfs",
		"lxd-pool":      "juju-zfs",
		"zfs.pool_name": "juju-lxd",
	})
	btrfsPool, _ := internalstorage.NewConfig("lxd-btrfs", "lxd", internalstorage.Attrs{
		"driver":   "btrfs",
		"lxd-pool": "juju-btrfs",
	})
	// This is the conflicting pool.
	recommendedPoolForBlock, _ := internalstorage.NewConfig("loop", "loop",
		internalstorage.Attrs{})
	lxdDefaultPools := []*internalstorage.Config{defaultPool, zfsPool, btrfsPool}

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).Return(
		s.storageProviderRegistry, nil)
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().Return(
		[]internalstorage.ProviderType{"lxd"}, nil,
	)
	s.storageProviderRegistry.EXPECT().StorageProvider(internalstorage.ProviderType("lxd")).
		Return(s.storageProvider, nil)
	s.storageProvider.EXPECT().DefaultPools().Return(lxdDefaultPools)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(internalstorage.StorageKindFilesystem).
		Return(defaultPool)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(internalstorage.StorageKindBlock).
		Return(recommendedPoolForBlock)

	// This has the same name as the provider loop pool but we cannot guarantee
	// they are refer to the same instance.
	input := []domainstorage.UserStoragePoolParams{
		{
			Name:       "loop",
			Provider:   "loop",
			Attributes: map[string]any{"foo": "bar"},
		},
	}
	pools, recommended, err := s.service.getStoragePoolsToImport(
		c.Context(),
		input,
	)

	c.Assert(err, tc.ErrorIsNil)
	// We assert that only the lxd pool is recommended.
	c.Assert(recommended, tc.DeepEquals, []domainstorage.RecommendedStoragePoolParams{
		// This is a pool name: lxd and provider: lxd.
		{
			StoragePoolUUID: "16d8c090-8ef4-59b4-8e88-0bc64a0598a3",
			StorageKind:     domainstorage.StorageKindFilesystem,
		},
	})

	mc := tc.NewMultiChecker()
	mc.AddExpr("_.UUID", tc.IsUUID)

	c.Assert(pools, tc.HasLen, 4)
	// User pool. The UUID is generated by the service so we don't know the exact
	// value, but we assert that it is a UUID.
	c.Assert(pools[0], tc.Bind(mc, domainstorage.ImportStoragePoolParams{
		Name:   "loop",
		Type:   "loop",
		Origin: domainstorage.StoragePoolOriginUser,
		Attrs:  map[string]any{"foo": "bar"},
	}))
	// The rest are provider default pools.
	c.Assert(pools[1:], tc.SameContents, []domainstorage.ImportStoragePoolParams{
		{
			UUID:   "16d8c090-8ef4-59b4-8e88-0bc64a0598a3",
			Name:   "lxd",
			Type:   "lxd",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Attrs:  map[string]any{},
		},
		{
			UUID:   "635f1873-be0b-5f07-b841-9fa02466a9f6",
			Name:   "lxd-zfs",
			Type:   "lxd",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Attrs: map[string]any{
				"driver":        "zfs",
				"lxd-pool":      "juju-zfs",
				"zfs.pool_name": "juju-lxd",
			},
		},
		{
			UUID:   "e1acb8b8-c978-5d53-bc22-2a0e7fd58734",
			Name:   "lxd-btrfs",
			Type:   "lxd",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Attrs: map[string]any{
				"driver":   "btrfs",
				"lxd-pool": "juju-btrfs",
			},
		},
	})
}

// TestGetStoragePoolsToImportRegistryGetterError asserts that an error propagated
// correctly when the storage provider registry returns an error.
func (s *importSuite) TestGetStoragePoolsToImportRegistryGetterError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	expectedErr := errors.New("registry down")

	s.storageRegistryGetter.EXPECT().
		GetStorageRegistry(gomock.Any()).
		Return(nil, expectedErr)

	_, _, err := s.service.getStoragePoolsToImport(c.Context(), nil)

	c.Assert(err, tc.ErrorMatches,
		`getting storage provider registry for model: registry down`)
}

// TestGetStoragePoolsToImportProviderTypesError asserts that an error is propagated
// correctly when fetching storage provider types returns an error.
func (s *importSuite) TestGetStoragePoolsToImportProviderTypesError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.storageRegistryGetter.EXPECT().
		GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil)

	s.storageProviderRegistry.EXPECT().
		StorageProviderTypes().
		Return(nil, errors.New("types boom"))

	_, _, err := s.service.getStoragePoolsToImport(c.Context(), nil)

	c.Assert(err, tc.ErrorMatches,
		`getting storage provider types for model storage registry: types boom`)
}

// TestGetStoragePoolsToImportStorageProviderError asserts that an error is propagated
// correctly when fetching a specific storage provider returns an error.
func (s *importSuite) TestGetStoragePoolsToImportStorageProviderError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.storageRegistryGetter.EXPECT().
		GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil)

	s.storageProviderRegistry.EXPECT().
		StorageProviderTypes().
		Return([]internalstorage.ProviderType{"lxd"}, nil)

	s.storageProviderRegistry.EXPECT().
		StorageProvider(internalstorage.ProviderType("lxd")).
		Return(nil, errors.New("provider boom"))

	_, _, err := s.service.getStoragePoolsToImport(c.Context(), nil)

	c.Assert(err, tc.ErrorMatches,
		`getting storage provider "lxd" from registry: provider boom`)
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.storageProviderRegistry = NewMockProviderRegistry(ctrl)
	s.storageRegistryGetter = NewMockModelStorageRegistryGetter(ctrl)
	s.storageProvider = NewMockProvider(ctrl)

	s.service = NewService(
		s.state, loggertesting.WrapCheckLog(c), s.storageRegistryGetter,
	)

	c.Cleanup(func() {
		s.state = nil
		s.service = nil
		s.storageProviderRegistry = nil
		s.storageRegistryGetter = nil
		s.storageProvider = nil
	})

	return ctrl
}

type ignoreUUIDArgsMatcher[T any] struct {
	c        *tc.C
	expected []T
}

func (m ignoreUUIDArgsMatcher[T]) Matches(arg any) bool {
	obtained, ok := arg.([]T)
	if !ok {
		return false
	}
	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.UUID`, tc.IsNonZeroUUID)
	return m.c.Check(obtained, tc.UnorderedMatch[[]T](mc), m.expected)
}

func (m ignoreUUIDArgsMatcher[T]) String() string {
	return "matches if the input slice matches expectation."
}
