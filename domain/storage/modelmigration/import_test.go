// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/description/v11"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	corestorage "github.com/juju/juju/core/storage"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
)

type importSuite struct {
	coordinator             *MockCoordinator
	service                 *MockImportService
	storageProviderRegistry *MockProviderRegistry
	storageRegistryGetter   *MockModelStorageRegistryGetter
	storageProvider         *MockProvider
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockImportService(ctrl)
	s.storageProviderRegistry = NewMockProviderRegistry(ctrl)
	s.storageRegistryGetter = NewMockModelStorageRegistryGetter(ctrl)
	s.storageProvider = NewMockProvider(ctrl)

	c.Cleanup(func() {
		s.coordinator = nil
		s.service = nil
		s.storageProviderRegistry = nil
		s.storageRegistryGetter = nil
		s.storageProvider = nil
	})

	return ctrl
}

func (s *importSuite) newImportOperation() *importOperation {
	return &importOperation{
		storageRegistryGetter: s.storageRegistryGetter,
		service:               s.service,
	}
}

func (s *importSuite) TestRegisterImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterImport(s.coordinator, corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
		return s.storageProviderRegistry
	}), loggertesting.WrapCheckLog(c))
}

// TestNoUserDefinedStoragePools tests that when the model contains no
// user-defined storage pools, all provider default pools are imported and
// the provider's recommended pool is set on the model.
func (s *importSuite) TestNoUserDefinedStoragePools(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Empty model.
	model := description.NewModel(description.ModelArgs{})

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).Return(
		s.storageProviderRegistry, nil)
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().Return(
		[]storage.ProviderType{"lxd"}, nil,
	)

	defaultPool, _ := storage.NewConfig("lxd", "lxd", storage.Attrs{})
	zfsPool, _ := storage.NewConfig("lxd-zfs", "lxd", storage.Attrs{
		"driver":        "zfs",
		"lxd-pool":      "juju-zfs",
		"zfs.pool_name": "juju-lxd",
	})
	btrfsPool, _ := storage.NewConfig("lxd-btrfs", "lxd", storage.Attrs{
		"driver":   "btrfs",
		"lxd-pool": "juju-btrfs",
	})
	lxdDefaultPools := []*storage.Config{defaultPool, zfsPool, btrfsPool}

	gomock.InOrder(
		s.storageProviderRegistry.EXPECT().StorageProvider(storage.ProviderType("lxd")).
			Return(s.storageProvider, nil),
		s.storageProvider.EXPECT().DefaultPools().Return(lxdDefaultPools),
		s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(storage.StorageKindFilesystem).
			Return(defaultPool),
		s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(storage.StorageKindBlock).
			Return(nil),

		// Calls ImportStoragePool for 3 storage pools.
		// There are no user-defined pools.
		s.service.EXPECT().ImportStoragePool(gomock.Any(),
			domainstorage.StoragePoolUUID("16d8c090-8ef4-59b4-8e88-0bc64a0598a3"),
			"lxd", domainstorage.ProviderType("lxd"), domainstorage.StoragePoolOriginProviderDefault,
			map[string]any{}),

		s.service.EXPECT().ImportStoragePool(gomock.Any(),
			domainstorage.StoragePoolUUID("635f1873-be0b-5f07-b841-9fa02466a9f6"),
			"lxd-zfs",
			domainstorage.ProviderType("lxd"), domainstorage.StoragePoolOriginProviderDefault,
			map[string]any{
				"driver":        "zfs",
				"lxd-pool":      "juju-zfs",
				"zfs.pool_name": "juju-lxd",
			}),

		s.service.EXPECT().ImportStoragePool(gomock.Any(), gomock.Any(), "lxd-btrfs",
			domainstorage.ProviderType("lxd"), domainstorage.StoragePoolOriginProviderDefault,
			map[string]any{
				"driver":   "btrfs",
				"lxd-pool": "juju-btrfs",
			}),
	)

	recommendedPools := []domainstorage.RecommendedStoragePoolParams{
		{
			StoragePoolUUID: "16d8c090-8ef4-59b4-8e88-0bc64a0598a3",
			StorageKind:     domainstorage.StorageKindFilesystem,
		},
	}
	s.service.EXPECT().SetRecommendedStoragePools(gomock.Any(), recommendedPools)

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

// TestImport tests that both user-defined storage pools and provider default
// pools are imported into the model when no conflicts exist, and that the
// recommended storage pool is set accordingly.
func (s *importSuite) TestImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	model.AddStoragePool(description.StoragePoolArgs{
		Name:       "ebs-fast",
		Provider:   "ebs",
		Attributes: map[string]any{"foo": "bar"},
	})

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).Return(
		s.storageProviderRegistry, nil)
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().Return(
		[]storage.ProviderType{"lxd"}, nil,
	)

	defaultPool, _ := storage.NewConfig("lxd", "lxd", storage.Attrs{})
	zfsPool, _ := storage.NewConfig("lxd-zfs", "lxd", storage.Attrs{
		"driver":        "zfs",
		"lxd-pool":      "juju-zfs",
		"zfs.pool_name": "juju-lxd",
	})
	btrfsPool, _ := storage.NewConfig("lxd-btrfs", "lxd", storage.Attrs{
		"driver":   "btrfs",
		"lxd-pool": "juju-btrfs",
	})
	lxdDefaultPools := []*storage.Config{defaultPool, zfsPool, btrfsPool}

	gomock.InOrder(
		s.storageProviderRegistry.EXPECT().StorageProvider(storage.ProviderType("lxd")).
			Return(s.storageProvider, nil),
		s.storageProvider.EXPECT().DefaultPools().Return(lxdDefaultPools),
		s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(storage.StorageKindFilesystem).
			Return(defaultPool),
		s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(storage.StorageKindBlock).
			Return(nil),

		// Calls ImportStoragePool for 4 storage pools.
		// There are no duplicate pools.
		s.service.EXPECT().ImportStoragePool(gomock.Any(), domainstorage.StoragePoolUUID(""),
			"ebs-fast",
			domainstorage.ProviderType("ebs"), domainstorage.StoragePoolOriginUser,
			map[string]any{"foo": "bar"}),

		s.service.EXPECT().ImportStoragePool(gomock.Any(),
			domainstorage.StoragePoolUUID("16d8c090-8ef4-59b4-8e88-0bc64a0598a3"),
			"lxd", domainstorage.ProviderType("lxd"), domainstorage.StoragePoolOriginProviderDefault,
			map[string]any{}),

		s.service.EXPECT().ImportStoragePool(gomock.Any(),
			domainstorage.StoragePoolUUID("635f1873-be0b-5f07-b841-9fa02466a9f6"),
			"lxd-zfs",
			domainstorage.ProviderType("lxd"), domainstorage.StoragePoolOriginProviderDefault,
			map[string]any{
				"driver":        "zfs",
				"lxd-pool":      "juju-zfs",
				"zfs.pool_name": "juju-lxd",
			}),

		s.service.EXPECT().ImportStoragePool(gomock.Any(), gomock.Any(), "lxd-btrfs",
			domainstorage.ProviderType("lxd"), domainstorage.StoragePoolOriginProviderDefault,
			map[string]any{
				"driver":   "btrfs",
				"lxd-pool": "juju-btrfs",
			}),
	)

	recommendedPools := []domainstorage.RecommendedStoragePoolParams{
		{
			StoragePoolUUID: "16d8c090-8ef4-59b4-8e88-0bc64a0598a3",
			StorageKind:     domainstorage.StorageKindFilesystem,
		},
	}
	s.service.EXPECT().SetRecommendedStoragePools(gomock.Any(), recommendedPools)

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportPickUserDefinedOnDuplicate tests that when a user-defined storage
// pool has the same name and provider as a provider default pool, the
// user-defined pool is preferred and the provider default pool is skipped.
// Other non-conflicting default pools are still imported and recommendations
// are set as expected.
func (s *importSuite) TestImportPickUserDefinedOnDuplicate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	model.AddStoragePool(description.StoragePoolArgs{
		Name:       "lxd-zfs",
		Provider:   "lxd",
		Attributes: map[string]any{"foo": "bar"},
	})

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).Return(
		s.storageProviderRegistry, nil)
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().Return(
		[]storage.ProviderType{"lxd"}, nil,
	)

	defaultPool, _ := storage.NewConfig("lxd", "lxd", storage.Attrs{})
	zfsPool, _ := storage.NewConfig("lxd-zfs", "lxd", storage.Attrs{
		"driver":        "zfs",
		"lxd-pool":      "juju-zfs",
		"zfs.pool_name": "juju-lxd",
	})
	btrfsPool, _ := storage.NewConfig("lxd-btrfs", "lxd", storage.Attrs{
		"driver":   "btrfs",
		"lxd-pool": "juju-btrfs",
	})
	lxdDefaultPools := []*storage.Config{defaultPool, zfsPool, btrfsPool}
	recommendedPools := []domainstorage.RecommendedStoragePoolParams{
		{
			StoragePoolUUID: "16d8c090-8ef4-59b4-8e88-0bc64a0598a3",
			StorageKind:     domainstorage.StorageKindFilesystem,
		},
	}

	gomock.InOrder(
		s.storageProviderRegistry.EXPECT().StorageProvider(storage.ProviderType("lxd")).
			Return(s.storageProvider, nil),
		s.storageProvider.EXPECT().DefaultPools().Return(lxdDefaultPools),
		s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(storage.StorageKindFilesystem).
			Return(defaultPool),
		s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(storage.StorageKindBlock).
			Return(nil),

		// Calls ImportStoragePool for 3 storage pools.
		// This is storage pool has a duplicate name and provider, so we pick this
		// over the default storage pool of the provider.
		s.service.EXPECT().ImportStoragePool(gomock.Any(), domainstorage.StoragePoolUUID(""),
			"lxd-zfs",
			domainstorage.ProviderType("lxd"), domainstorage.StoragePoolOriginUser,
			map[string]any{"foo": "bar"}),

		s.service.EXPECT().ImportStoragePool(gomock.Any(),
			domainstorage.StoragePoolUUID("16d8c090-8ef4-59b4-8e88-0bc64a0598a3"),
			"lxd", domainstorage.ProviderType("lxd"), domainstorage.StoragePoolOriginProviderDefault,
			map[string]any{}),

		s.service.EXPECT().ImportStoragePool(gomock.Any(), gomock.Any(), "lxd-btrfs",
			domainstorage.ProviderType("lxd"), domainstorage.StoragePoolOriginProviderDefault,
			map[string]any{
				"driver":   "btrfs",
				"lxd-pool": "juju-btrfs",
			}),
		s.service.EXPECT().SetRecommendedStoragePools(gomock.Any(), recommendedPools),
	)

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}
