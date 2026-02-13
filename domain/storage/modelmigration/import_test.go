// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"errors"
	"testing"

	"github.com/juju/description/v11"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	corestorage "github.com/juju/juju/core/storage"
	domainstorage "github.com/juju/juju/domain/storage"
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

func (s *importSuite) TestImportEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})
	s.noopStoragePoolImport()

	// Act
	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestNoUserDefinedStoragePools tests that Execute imports provider default
// storage pools and sets the recommended pool when the model contains no
// user-defined storage pools.
func (s *importSuite) TestNoUserDefinedStoragePools(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})
	ctx := c.Context()

	poolsToImport := []domainstorage.ImportStoragePoolParams{
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
	}

	recommendedPools := []domainstorage.RecommendedStoragePoolParams{
		{
			StoragePoolUUID: "16d8c090-8ef4-59b4-8e88-0bc64a0598a3",
			StorageKind:     domainstorage.StorageKindFilesystem,
		},
	}

	gomock.InOrder(
		s.service.EXPECT().
			GetStoragePoolsToImport(ctx, model.StoragePools()).
			Return(poolsToImport, recommendedPools, nil),

		s.service.EXPECT().
			ImportStoragePools(ctx, poolsToImport).
			Return(nil),

		s.service.EXPECT().
			SetRecommendedStoragePools(ctx, recommendedPools).
			Return(nil),
	)

	op := s.newImportOperation()
	err := op.Execute(ctx, model)
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportStoragePools tests that Execute imports both user-defined and provider default
// storage pools and sets the recommended pools.
func (s *importSuite) TestImportStoragePools(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})
	model.AddStoragePool(description.StoragePoolArgs{
		Name:       "ebs-fast",
		Provider:   "ebs",
		Attributes: map[string]any{"foo": "bar"},
	})

	ctx := c.Context()

	poolsToImport := []domainstorage.ImportStoragePoolParams{
		{
			Name:   "ebs-fast",
			Type:   "ebs",
			Origin: domainstorage.StoragePoolOriginUser,
			Attrs:  map[string]any{"foo": "bar"},
		},
		{
			UUID:   "16d8c090-8ef4-59b4-8e88-0bc64a0598a3",
			Name:   "lxd",
			Type:   "lxd",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Attrs:  map[string]any{},
		},
	}

	recommendedPools := []domainstorage.RecommendedStoragePoolParams{
		{
			StoragePoolUUID: "16d8c090-8ef4-59b4-8e88-0bc64a0598a3",
			StorageKind:     domainstorage.StorageKindFilesystem,
		},
	}

	gomock.InOrder(
		s.service.EXPECT().
			GetStoragePoolsToImport(ctx, model.StoragePools()).
			Return(poolsToImport, recommendedPools, nil),

		s.service.EXPECT().
			ImportStoragePools(ctx, poolsToImport).
			Return(nil),

		s.service.EXPECT().
			SetRecommendedStoragePools(ctx, recommendedPools).
			Return(nil),
	)

	op := s.newImportOperation()
	err := op.Execute(ctx, model)
	c.Assert(err, tc.ErrorIsNil)
}

// TestExecuteGetStoragePoolsToImportError tests that Execute fails fast if
// GetStoragePoolsToImport returns an error, and that no further calls are made.
func (s *importSuite) TestExecuteGetStoragePoolsToImportError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})
	expectedErr := errors.New("boom")

	s.service.EXPECT().
		GetStoragePoolsToImport(gomock.Any(), model.StoragePools()).
		Return(nil, nil, expectedErr)

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)

	c.Assert(err, tc.ErrorMatches, "getting pools to import: .*boom")
}

// TestExecuteImportStoragePoolsError tests that Execute returns an error if
// ImportStoragePools fails, and that recommended storage pools are not set.
func (s *importSuite) TestExecuteImportStoragePoolsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})
	expectedErr := errors.New("import failed")

	poolsToImport := []domainstorage.ImportStoragePoolParams{
		{
			UUID:   "pool-1",
			Name:   "lxd",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Type:   "lxd",
			Attrs:  map[string]any{},
		},
	}

	recommendedPools := []domainstorage.RecommendedStoragePoolParams{
		{
			StoragePoolUUID: "pool-1",
			StorageKind:     domainstorage.StorageKindFilesystem,
		},
	}

	gomock.InOrder(
		s.service.EXPECT().
			GetStoragePoolsToImport(gomock.Any(), model.StoragePools()).
			Return(poolsToImport, recommendedPools, nil),

		s.service.EXPECT().
			ImportStoragePools(gomock.Any(), poolsToImport).
			Return(expectedErr),
	)

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)

	c.Assert(err, tc.ErrorMatches, "importing storage pools .*: .*import failed")
}

// TestExecuteSetRecommendedStoragePoolsError tests that Execute returns an error
// if setting recommended storage pools fails, even when imports succeed.
func (s *importSuite) TestExecuteSetRecommendedStoragePoolsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})
	expectedErr := errors.New("recommendation failed")

	poolsToImport := []domainstorage.ImportStoragePoolParams{
		{
			UUID:   "pool-1",
			Name:   "lxd",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Type:   "lxd",
			Attrs:  map[string]any{},
		},
	}

	recommendedPools := []domainstorage.RecommendedStoragePoolParams{
		{
			StoragePoolUUID: "pool-1",
			StorageKind:     domainstorage.StorageKindFilesystem,
		},
	}

	gomock.InOrder(
		s.service.EXPECT().
			GetStoragePoolsToImport(gomock.Any(), model.StoragePools()).
			Return(poolsToImport, recommendedPools, nil),

		s.service.EXPECT().
			ImportStoragePools(gomock.Any(), poolsToImport).
			Return(nil),

		s.service.EXPECT().
			SetRecommendedStoragePools(gomock.Any(), recommendedPools).
			Return(expectedErr),
	)

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)

	c.Assert(err, tc.ErrorMatches, "setting recommended storage pools: .*recommendation failed")
}

func (s *importSuite) TestImportStorageInstances(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	expected := []domainstorage.ImportStorageInstanceParams{
		{
			PoolName:         "testpool",
			RequestedSizeMiB: uint64(1024),
			StorageID:        "multi-fs/1",
			StorageKind:      "block",
			StorageName:      "multi-fs",
			UnitName:         "unit/3",
		},
	}
	s.noopStoragePoolImport()
	s.service.EXPECT().ImportStorageInstances(gomock.Any(), expected).Return(nil)
	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})
	model.AddStorage(description.StorageArgs{
		ID:          "multi-fs/1",
		Kind:        "block",
		UnitOwner:   "unit/3",
		Name:        "multi-fs",
		Attachments: nil,
		Constraints: &description.StorageInstanceConstraints{
			Pool: "testpool",
			Size: 1024,
		},
	})

	// Act
	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportStorageInstancesValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})
	model.AddStorage(description.StorageArgs{
		Kind:        "block",
		UnitOwner:   "unit/3",
		Name:        "multi-fs",
		Attachments: nil,
		Constraints: &description.StorageInstanceConstraints{
			Pool: "testpool",
			Size: 1024,
		},
	})
	s.noopStoragePoolImport()

	// Act
	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestImportFilesystems(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})
	model.AddFilesystem(description.FilesystemArgs{
		ID:           "fs-1",
		Size:         2048,
		Storage:      "multi-fs/1",
		Pool:         "testpool",
		FilesystemID: "provider-fs-1",
	})
	model.AddFilesystem(description.FilesystemArgs{
		ID:           "fs-2",
		Size:         4096,
		Storage:      "multi-fs/2",
		Pool:         "testpool",
		FilesystemID: "provider-fs-2",
	})

	s.noopStoragePoolImport()
	s.service.EXPECT().ImportFilesystems(gomock.Any(), tc.Bind(tc.SameContents, []domainstorage.ImportFilesystemParams{{
		ID:                "fs-1",
		SizeInMiB:         2048,
		StorageInstanceID: "multi-fs/1",
		PoolName:          "testpool",
		ProviderID:        "provider-fs-1",
	}, {
		ID:                "fs-2",
		SizeInMiB:         4096,
		StorageInstanceID: "multi-fs/2",
		PoolName:          "testpool",
		ProviderID:        "provider-fs-2",
	}})).Return(nil)

	// Act
	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) noopStoragePoolImport() {
	s.service.EXPECT().GetStoragePoolsToImport(gomock.Any(), gomock.Any()).Return(nil, nil, nil)
	s.service.EXPECT().ImportStoragePools(gomock.Any(), gomock.Any()).Return(nil)
	s.service.EXPECT().SetRecommendedStoragePools(gomock.Any(), gomock.Any()).Return(nil)
}

func (s *importSuite) TestImportVolumes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{})
	model.AddVolume(description.VolumeArgs{
		ID:          "0",
		Storage:     "multi-fs/0",
		Provisioned: true,
		Persistent:  true,
		Pool:        "ebs",
		Size:        1024,
		VolumeID:    "vol-0f2829d7e5c4c0140",
		WWN:         "uuid.c2f9e696-7b12-5368-b274-0510bf1feade",
	})

	expected := domainstorage.ImportVolumeParams{
		{
			ID:          "0",
			StorageID:   "multi-fs/0",
			Provisioned: true,
			Persistent:  true,
			Pool:        "ebs",
			SizeMiB:     1024,
			ProviderID:  "vol-0f2829d7e5c4c0140",
			WWN:         "uuid.c2f9e696-7b12-5368-b274-0510bf1feade",
		},
	}
	s.service.EXPECT().ImportVolumes(gomock.Any(), expected).Return(nil)

	// Act
	op := s.newImportOperation()
	err := op.importVolumes(c.Context(), model.Volumes())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportVolumesZeroLength(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{})

	// Act
	op := s.newImportOperation()
	err := op.importVolumes(c.Context(), model.Volumes())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}
