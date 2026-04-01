// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/description/v12"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	corestorage "github.com/juju/juju/core/storage"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/environs/config"
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

	RegisterImport(
		s.coordinator,
		corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
			return s.storageProviderRegistry
		}),
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
	s.noopStoragePoolImport()

	// Act
	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)

	// Assert
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

	poolsToImport := []domainstorage.UserStoragePoolParams{
		{
			Name:       "ebs-fast",
			Provider:   "ebs",
			Attributes: map[string]interface{}{"foo": "bar"},
		},
	}
	s.service.EXPECT().ImportStoragePools(ctx, poolsToImport)

	op := s.newImportOperation()
	err := op.Execute(ctx, model)
	c.Assert(err, tc.ErrorIsNil)
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
	s.noopStoragePoolImport()

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

	s.noopStoragePoolImport()
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

	s.noopStoragePoolImport()
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
	s.noopStoragePoolImport()

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
	s.noopStoragePoolImport()

	// Act
	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) noopStoragePoolImport() {
	s.service.EXPECT().ImportStoragePools(gomock.Any(), gomock.Any()).Return(nil)
}
