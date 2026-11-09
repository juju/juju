// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"errors"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/description/v12"
	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/environs/config"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	coordinator             *MockCoordinator
	service                 *MockImportService
	storageProviderRegistry *MockProviderRegistry
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
	s.storageProvider = NewMockProvider(ctrl)

	c.Cleanup(func() {
		s.coordinator = nil
		s.service = nil
		s.storageProviderRegistry = nil
		s.storageProvider = nil
	})

	return ctrl
}

func (s *importSuite) newStoragePoolImportOperation() *importStoragePoolOperation {
	return &importStoragePoolOperation{
		baseImportOperation: baseImportOperation{
			service: s.service,
		},
	}
}

func (s *importSuite) newImportOperation() *importStorageOperation {
	return &importStorageOperation{
		baseImportOperation: baseImportOperation{
			service: s.service,
		},
	}
}

func (s *importSuite) TestRegisterImportStoragePools(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterImportStoragePools(
		s.coordinator,
		nil,
		loggertesting.WrapCheckLog(c),
	)
}

func (s *importSuite) TestRegisterImportStorage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterImportStorage(
		s.coordinator,
		nil,
		loggertesting.WrapCheckLog(c),
	)
}

func (s *importSuite) TestImportEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})

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

	op := s.newStoragePoolImportOperation()
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

	op := s.newStoragePoolImportOperation()
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

	op := s.newStoragePoolImportOperation()
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

	op := s.newStoragePoolImportOperation()
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

	op := s.newStoragePoolImportOperation()
	err := op.Execute(c.Context(), model)

	c.Assert(err, tc.ErrorMatches, "setting recommended storage pools: .*recommendation failed")
}

func (s *importSuite) TestImportStorageInstances(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	expected := []domainstorage.ImportStorageInstanceParams{
		{
			PoolName:          "testpool",
			RequestedSizeMiB:  uint64(1024),
			StorageInstanceID: "multi-fs/1",
			StorageKind:       "block",
			StorageName:       "multi-fs",
			UnitName:          "unit/3",
			AttachedUnitNames: []string{"foo/0", "bar/1"},
		},
	}
	s.service.EXPECT().ImportStorageInstances(gomock.Any(), expected).Return(nil)
	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})
	model.AddStorage(description.StorageArgs{
		ID:          "multi-fs/1",
		Kind:        "block",
		UnitOwner:   "unit/3",
		Name:        "multi-fs",
		Attachments: []string{"foo/0", "bar/1"},
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

	// Act
	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestImportFilesystemsIAAS(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})
	fs1 := model.AddFilesystem(description.FilesystemArgs{
		ID:           "fs-1",
		Size:         2048,
		Storage:      "multi-fs/1",
		Pool:         "testpool",
		FilesystemID: "provider-fs-1",
	})
	fs2 := model.AddFilesystem(description.FilesystemArgs{
		ID:           "fs-2",
		Size:         4096,
		Storage:      "multi-fs/2",
		Pool:         "testpool",
		FilesystemID: "provider-fs-2",
	})

	fs1.AddAttachment(description.FilesystemAttachmentArgs{
		HostMachine: "1",
		ReadOnly:    false,
		MountPoint:  "/data",
	})
	fs2.AddAttachment(description.FilesystemAttachmentArgs{
		HostUnit:   "unit/1",
		ReadOnly:   true,
		MountPoint: "/opt",
	})

	s.service.EXPECT().ImportFilesystemsIAAS(gomock.Any(), tc.Bind(tc.SameContents, []domainstorage.ImportFilesystemParams{{
		ID:                "fs-1",
		SizeInMiB:         2048,
		StorageInstanceID: "multi-fs/1",
		PoolName:          "testpool",
		ProviderID:        "provider-fs-1",
		Attachments: []domainstorage.ImportFilesystemAttachmentsParams{{
			HostMachineName: "1",
			ReadOnly:        false,
			MountPoint:      "/data",
		}},
	}, {
		ID:                "fs-2",
		SizeInMiB:         4096,
		StorageInstanceID: "multi-fs/2",
		PoolName:          "testpool",
		ProviderID:        "provider-fs-2",
		Attachments: []domainstorage.ImportFilesystemAttachmentsParams{{
			HostUnitName: "unit/1",
			ReadOnly:     true,
			MountPoint:   "/opt",
		}},
	}})).Return(nil)

	// Act
	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportFilesystemsCAAS(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{
		Type: coremodel.CAAS.String(),
		Config: map[string]any{
			config.NameKey: "moveme",
		},
	})
	fs1 := model.AddFilesystem(description.FilesystemArgs{
		ID:           "fs-1",
		Size:         2048,
		Storage:      "multi-fs/1",
		Pool:         "kubernetes",
		FilesystemID: "753fff9e-6d0d-4d2c-b1e5-e2b3c02284f9",
		Volume:       "0",
	})
	fs1.AddAttachment(description.FilesystemAttachmentArgs{
		HostUnit:   "unit/0",
		ReadOnly:   false,
		MountPoint: "/data",
	})

	fs2 := model.AddFilesystem(description.FilesystemArgs{
		ID:           "fs-2",
		Size:         4096,
		Storage:      "multi-fs/2",
		Pool:         "kubernetes",
		FilesystemID: "deadbeef-6d0d-4d2c-b1e5-e2b3c02284f9",
		Volume:       "1",
	})
	fs2.AddAttachment(description.FilesystemAttachmentArgs{
		HostUnit:   "unit/1",
		ReadOnly:   true,
		MountPoint: "/opt",
	})

	model.AddVolume(description.VolumeArgs{
		ID:          "0",
		Storage:     "multi-fs/1",
		Provisioned: true,
		Persistent:  true,
		Pool:        "kubernetes",
		Size:        1024,
		VolumeID:    "pvc-753fff9e-6d0d-4d2c-b1e5-e2b3c02284f9",
	})
	model.AddVolume(description.VolumeArgs{
		ID:          "1",
		Storage:     "multi-fs/2",
		Provisioned: true,
		Persistent:  true,
		Pool:        "kubernetes",
		Size:        1024,
		VolumeID:    "pvc-deadbeef-6d0d-4d2c-b1e5-e2b3c02284f9",
	})

	s.service.EXPECT().ImportFilesystemsCAAS(gomock.Any(), tc.Bind(tc.SameContents, []domainstorage.ImportFilesystemParams{{
		ID:                "fs-1",
		SizeInMiB:         1024,
		StorageInstanceID: "multi-fs/1",
		PoolName:          "kubernetes",
		ProviderID:        "pvc-753fff9e-6d0d-4d2c-b1e5-e2b3c02284f9",
		Attachments: []domainstorage.ImportFilesystemAttachmentsParams{{
			ProviderID:   "753fff9e-6d0d-4d2c-b1e5-e2b3c02284f9",
			HostUnitName: "unit/0",
			ReadOnly:     false,
			MountPoint:   "/data",
		}},
	}, {
		ID:                "fs-2",
		SizeInMiB:         1024,
		StorageInstanceID: "multi-fs/2",
		PoolName:          "kubernetes",
		ProviderID:        "pvc-deadbeef-6d0d-4d2c-b1e5-e2b3c02284f9",
		Attachments: []domainstorage.ImportFilesystemAttachmentsParams{{
			ProviderID:   "deadbeef-6d0d-4d2c-b1e5-e2b3c02284f9",
			HostUnitName: "unit/1",
			ReadOnly:     true,
			MountPoint:   "/opt",
		}},
	}})).Return(nil)

	// Act
	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportVolumes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})
	volOne := model.AddVolume(description.VolumeArgs{
		ID:          "0",
		Storage:     "multi-fs/0",
		Provisioned: true,
		Persistent:  true,
		Pool:        "ebs",
		Size:        1024,
		VolumeID:    "vol-0f2829d7e5c4c0140",
		WWN:         "uuid.c2f9e696-7b12-5368-b274-0510bf1feade",
	})
	volOne.AddAttachment(description.VolumeAttachmentArgs{
		HostMachine: "0",
		DeviceName:  "xvdf",
		DeviceLink:  "/dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_vol0e8b3aed0fbee6887",
		DeviceAttributes: map[string]string{
			"iqn":  "iqn.2015-12.com.oracleiaas:5349c1a7-36b4-4d7c-85f2-059c5cd6e344",
			"port": "3260",
		},
	})
	volOne.AddAttachmentPlan(description.VolumeAttachmentPlanArgs{
		Machine:    "0",
		DeviceType: "local",
		DeviceAttributes: map[string]string{
			"iqn":  "iqn.2015-12.com.oracleiaas:5349c1a7-36b4-4d7c-85f2-059c5cd6e344",
			"port": "3260",
		},
	})
	volTwo := model.AddVolume(description.VolumeArgs{
		ID:          "1",
		Storage:     "multi-fs/1",
		Provisioned: true,
		Persistent:  true,
		Pool:        "ebs",
		Size:        1024,
		VolumeID:    "vol-08195b158e8ce069d",
		WWN:         "uuid.1c63bb59-9514-505d-8d85-275a629db6d9",
	})
	volTwo.AddAttachment(description.VolumeAttachmentArgs{
		HostMachine: "1",
		DeviceName:  "xvdf",
		DeviceLink:  "/dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_vol08195b158e8ce069d",
	})
	volTwo.AddAttachmentPlan(description.VolumeAttachmentPlanArgs{
		Machine:    "1",
		DeviceType: "iscsi",
	})

	expected := []domainstorage.ImportVolumeParams{
		{
			ID:                "0",
			StorageInstanceID: "multi-fs/0",
			Provisioned:       true,
			Persistent:        true,
			Pool:              "ebs",
			SizeMiB:           1024,
			ProviderID:        "vol-0f2829d7e5c4c0140",
			WWN:               "uuid.c2f9e696-7b12-5368-b274-0510bf1feade",
			Attachments: []domainstorage.ImportVolumeAttachmentParams{
				{
					HostMachineName: "0",
					DeviceName:      "xvdf",
					DeviceLink:      "/dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_vol0e8b3aed0fbee6887",
				},
			},
			AttachmentPlans: []domainstorage.ImportVolumeAttachmentPlanParams{
				{
					HostMachineName: "0",
					DeviceType:      "local",
					DeviceAttributes: map[string]string{
						"iqn":  "iqn.2015-12.com.oracleiaas:5349c1a7-36b4-4d7c-85f2-059c5cd6e344",
						"port": "3260",
					},
				},
			},
		}, {
			ID:                "1",
			StorageInstanceID: "multi-fs/1",
			Provisioned:       true,
			Persistent:        true,
			Pool:              "ebs",
			SizeMiB:           1024,
			ProviderID:        "vol-08195b158e8ce069d",
			WWN:               "uuid.1c63bb59-9514-505d-8d85-275a629db6d9",
			Attachments: []domainstorage.ImportVolumeAttachmentParams{
				{
					HostMachineName: "1",
					DeviceName:      "xvdf",
					DeviceLink:      "/dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_vol08195b158e8ce069d",
				},
			},
			AttachmentPlans: []domainstorage.ImportVolumeAttachmentPlanParams{
				{
					HostMachineName: "1",
					DeviceType:      "iscsi",
				},
			},
		},
	}
	s.service.EXPECT().ImportVolumes(gomock.Any(), expected).Return(nil)

	// Act
	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportVolumesZeroLength(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})

	// Act
	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportWorkloadStoragePool tests that a 3.x workload-storage model
// config nomination is represented as a Kubernetes storage pool named
// after the recorded class, so charm storage in the imported model
// provisions from the nominated class.
func (s *importSuite) TestImportWorkloadStoragePool(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{
		Type: coremodel.CAAS.String(),
		Config: map[string]any{
			"workload-storage": "ceph-rbd",
		},
	})
	ctx := c.Context()

	gomock.InOrder(
		s.service.EXPECT().
			GetStoragePoolsToImport(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, pools []description.StoragePool) (
				[]domainstorage.ImportStoragePoolParams,
				[]domainstorage.RecommendedStoragePoolParams,
				error,
			) {
				c.Assert(pools, tc.HasLen, 1)
				c.Check(pools[0].Name(), tc.Equals, "ceph-rbd")
				c.Check(pools[0].Provider(), tc.Equals, "kubernetes")
				c.Check(pools[0].Attributes(), tc.DeepEquals, map[string]any{
					"storage-class": "ceph-rbd",
				})
				return nil, nil, nil
			}),
		s.service.EXPECT().ImportStoragePools(ctx, nil).Return(nil),
		s.service.EXPECT().SetRecommendedStoragePools(ctx, nil).Return(nil),
	)

	// Act
	op := s.newStoragePoolImportOperation()
	err := op.Execute(ctx, model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportWorkloadStoragePoolRespectsDefaultSource tests that no pool is
// derived from the 3.x workload-storage nomination when the model already
// nominates its own default filesystem source.
func (s *importSuite) TestImportWorkloadStoragePoolRespectsDefaultSource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{
		Type: coremodel.CAAS.String(),
		Config: map[string]any{
			"workload-storage":                  "ceph-rbd",
			"storage-default-filesystem-source": "my-pool",
		},
	})
	ctx := c.Context()

	gomock.InOrder(
		s.service.EXPECT().
			GetStoragePoolsToImport(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, pools []description.StoragePool) (
				[]domainstorage.ImportStoragePoolParams,
				[]domainstorage.RecommendedStoragePoolParams,
				error,
			) {
				// No pool is derived from the nomination.
				c.Assert(pools, tc.HasLen, 0)
				return nil, nil, nil
			}),
		s.service.EXPECT().ImportStoragePools(ctx, nil).Return(nil),
		s.service.EXPECT().SetRecommendedStoragePools(ctx, nil).Return(nil),
	)

	// Act
	op := s.newStoragePoolImportOperation()
	err := op.Execute(ctx, model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportWorkloadStoragePoolExistingPool tests that no duplicate pool is
// derived when the model description already defines a pool of the derived
// name.
func (s *importSuite) TestImportWorkloadStoragePoolExistingPool(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{
		Type: coremodel.CAAS.String(),
		Config: map[string]any{
			"workload-storage": "ceph-rbd",
		},
	})
	model.AddStoragePool(description.StoragePoolArgs{
		Name:       "ceph-rbd",
		Provider:   "kubernetes",
		Attributes: map[string]any{"storage-class": "ceph-rbd"},
	})
	ctx := c.Context()

	gomock.InOrder(
		s.service.EXPECT().
			GetStoragePoolsToImport(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, pools []description.StoragePool) (
				[]domainstorage.ImportStoragePoolParams,
				[]domainstorage.RecommendedStoragePoolParams,
				error,
			) {
				// The description's own pool is not shadowed or duplicated.
				c.Assert(pools, tc.HasLen, 1)
				c.Check(pools[0].Attributes(), tc.DeepEquals, map[string]any{
					"storage-class": "ceph-rbd",
				})
				return nil, nil, nil
			}),
		s.service.EXPECT().ImportStoragePools(ctx, nil).Return(nil),
		s.service.EXPECT().SetRecommendedStoragePools(ctx, nil).Return(nil),
	)

	// Act
	op := s.newStoragePoolImportOperation()
	err := op.Execute(ctx, model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportWorkloadStoragePoolIAAS tests that the 3.x workload-storage
// nomination, only meaningful for Kubernetes models, does not add a pool
// to models of other types.
func (s *importSuite) TestImportWorkloadStoragePoolIAAS(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
		Config: map[string]any{
			"workload-storage": "ceph-rbd",
		},
	})
	ctx := c.Context()

	gomock.InOrder(
		s.service.EXPECT().
			GetStoragePoolsToImport(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, pools []description.StoragePool) (
				[]domainstorage.ImportStoragePoolParams,
				[]domainstorage.RecommendedStoragePoolParams,
				error,
			) {
				c.Assert(pools, tc.HasLen, 0)
				return nil, nil, nil
			}),
		s.service.EXPECT().ImportStoragePools(ctx, nil).Return(nil),
		s.service.EXPECT().SetRecommendedStoragePools(ctx, nil).Return(nil),
	)

	// Act
	op := s.newStoragePoolImportOperation()
	err := op.Execute(ctx, model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportWorkloadStoragePoolReservedName tests that a class named after
// one of the pools seeded in every Kubernetes model derives a prefixed
// pool name that does not shadow the seeded pool.
func (s *importSuite) TestImportWorkloadStoragePoolReservedName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{
		Type: coremodel.CAAS.String(),
		Config: map[string]any{
			"workload-storage": "kubernetes",
		},
	})
	ctx := c.Context()

	gomock.InOrder(
		s.service.EXPECT().
			GetStoragePoolsToImport(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, pools []description.StoragePool) (
				[]domainstorage.ImportStoragePoolParams,
				[]domainstorage.RecommendedStoragePoolParams,
				error,
			) {
				c.Assert(pools, tc.HasLen, 1)
				c.Check(pools[0].Name(), tc.Equals, "k8s-kubernetes")
				c.Check(pools[0].Attributes(), tc.DeepEquals, map[string]any{
					"storage-class": "kubernetes",
				})
				return nil, nil, nil
			}),
		s.service.EXPECT().ImportStoragePools(ctx, nil).Return(nil),
		s.service.EXPECT().SetRecommendedStoragePools(ctx, nil).Return(nil),
	)

	// Act
	op := s.newStoragePoolImportOperation()
	err := op.Execute(ctx, model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportWorkloadStoragePoolDottedClass tests that a class name that is
// not a valid pool name derives the pool the model config import's default
// filesystem source resolves to: name derived from the class, attributes
// carrying the class verbatim.
func (s *importSuite) TestImportWorkloadStoragePoolDottedClass(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{
		Type: coremodel.CAAS.String(),
		Config: map[string]any{
			"workload-storage": "ebs.csi.aws.com",
		},
	})
	ctx := c.Context()

	gomock.InOrder(
		s.service.EXPECT().
			GetStoragePoolsToImport(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, pools []description.StoragePool) (
				[]domainstorage.ImportStoragePoolParams,
				[]domainstorage.RecommendedStoragePoolParams,
				error,
			) {
				c.Assert(pools, tc.HasLen, 1)
				c.Check(pools[0].Name(), tc.Equals, "ebs-csi-aws-com")
				c.Check(pools[0].Provider(), tc.Equals, "kubernetes")
				c.Check(pools[0].Attributes(), tc.DeepEquals, map[string]any{
					"storage-class": "ebs.csi.aws.com",
				})
				return nil, nil, nil
			}),
		s.service.EXPECT().ImportStoragePools(ctx, nil).Return(nil),
		s.service.EXPECT().SetRecommendedStoragePools(ctx, nil).Return(nil),
	)

	// Act
	op := s.newStoragePoolImportOperation()
	err := op.Execute(ctx, model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportWorkloadStoragePoolForeignPoolKept tests that a pool of the
// derived name provisioning from another class is kept: nothing is
// synthesized, mirroring 3.6, where the existing pool won regardless of
// its attributes.
func (s *importSuite) TestImportWorkloadStoragePoolForeignPoolKept(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{
		Type: coremodel.CAAS.String(),
		Config: map[string]any{
			"workload-storage": "fast.ssd",
		},
	})
	model.AddStoragePool(description.StoragePoolArgs{
		Name:       "fast-ssd",
		Provider:   "kubernetes",
		Attributes: map[string]any{"storage-class": "other-sc"},
	})
	ctx := c.Context()

	gomock.InOrder(
		s.service.EXPECT().
			GetStoragePoolsToImport(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, pools []description.StoragePool) (
				[]domainstorage.ImportStoragePoolParams,
				[]domainstorage.RecommendedStoragePoolParams,
				error,
			) {
				// The description's own pool is passed through untouched;
				// no second pool is derived.
				c.Assert(pools, tc.HasLen, 1)
				c.Check(pools[0].Attributes(), tc.DeepEquals, map[string]any{
					"storage-class": "other-sc",
				})
				return nil, nil, nil
			}),
		s.service.EXPECT().ImportStoragePools(ctx, nil).Return(nil),
		s.service.EXPECT().SetRecommendedStoragePools(ctx, nil).Return(nil),
	)

	// Act
	op := s.newStoragePoolImportOperation()
	err := op.Execute(ctx, model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportWorkloadStoragePoolIgnoresOperatorStorage tests that the 3.x
// operator-storage key, routinely set on 3.x Kubernetes models, plays no
// role in the pool derived from the workload-storage nomination.
func (s *importSuite) TestImportWorkloadStoragePoolIgnoresOperatorStorage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{
		Type: coremodel.CAAS.String(),
		Config: map[string]any{
			"workload-storage": "ceph-rbd",
			"operator-storage": "otherstorage",
		},
	})
	ctx := c.Context()

	gomock.InOrder(
		s.service.EXPECT().
			GetStoragePoolsToImport(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, pools []description.StoragePool) (
				[]domainstorage.ImportStoragePoolParams,
				[]domainstorage.RecommendedStoragePoolParams,
				error,
			) {
				// Only the workload-storage pool is derived; nothing is
				// derived from operator-storage.
				c.Assert(pools, tc.HasLen, 1)
				c.Check(pools[0].Name(), tc.Equals, "ceph-rbd")
				c.Check(pools[0].Attributes(), tc.DeepEquals, map[string]any{
					"storage-class": "ceph-rbd",
				})
				return nil, nil, nil
			}),
		s.service.EXPECT().ImportStoragePools(ctx, nil).Return(nil),
		s.service.EXPECT().SetRecommendedStoragePools(ctx, nil).Return(nil),
	)

	// Act
	op := s.newStoragePoolImportOperation()
	err := op.Execute(ctx, model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}
