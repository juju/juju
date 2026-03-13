// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/description/v11"
	"github.com/juju/tc"

	coreblockdevice "github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	domainapplication "github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationstate "github.com/juju/juju/domain/application/state"
	blockdeviceservice "github.com/juju/juju/domain/blockdevice/service"
	blockdevicestate "github.com/juju/juju/domain/blockdevice/state"
	"github.com/juju/juju/domain/deployment"
	domainlife "github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domainstorage "github.com/juju/juju/domain/storage"
	storagemodelmigration "github.com/juju/juju/domain/storage/modelmigration"
	"github.com/juju/juju/domain/storage/service"
	"github.com/juju/juju/domain/storage/state"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningservice "github.com/juju/juju/domain/storageprovisioning/service"
	storageprovisioningstate "github.com/juju/juju/domain/storageprovisioning/state"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalstorage "github.com/juju/juju/internal/storage"
	dummystorage "github.com/juju/juju/internal/storage/provider/dummy"
)

type importSuite struct {
	schematesting.ModelSuite

	coordinator    *coremodelmigration.Coordinator
	scope          coremodelmigration.Scope
	svc            *service.Service
	provisioning   *storageprovisioningservice.Service
	registryGetter corestorage.ModelStorageRegistryGetter
	logger         logger.Logger
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	s.logger = loggertesting.WrapCheckLog(c)
	s.registryGetter = s.newStorageRegistryGetter()
	s.coordinator = coremodelmigration.NewCoordinator(s.logger)
	s.scope = coremodelmigration.NewScope(
		nil,
		s.TxnRunnerFactory(),
		nil,
		nil,
		model.UUID(s.ModelUUID()),
	)
	s.svc = service.NewService(
		state.NewState(s.TxnRunnerFactory()),
		s.logger,
		s.registryGetter,
	)
	s.provisioning = storageprovisioningservice.NewService(
		storageprovisioningstate.NewState(s.TxnRunnerFactory()),
		nil,
		s.logger,
	)

	c.Cleanup(func() {
		s.coordinator = nil
		s.scope = coremodelmigration.Scope{}
		s.svc = nil
		s.provisioning = nil
		s.registryGetter = nil
	})
}

func (s *importSuite) TestImportStorageInstancesVolumeBackedFilesystems(c *tc.C) {
	// Arrange
	entities := s.createIAASApplication(c, "postgresql", 2)
	machine0Name, machine1Name := entities.machineNames[0], entities.machineNames[1]
	machine0UUID, machine1UUID := entities.machineUUIDs[0], entities.machineUUIDs[1]
	unit0Name, unit1Name := entities.unitNames[0], entities.unitNames[1]
	unit0UUID, unit1UUID := entities.unitUUIDs[0], entities.unitUUIDs[1]

	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})
	desc.AddStoragePool(description.StoragePoolArgs{
		Name:     "ebs",
		Provider: "ebs",
	})
	desc.AddStorage(description.StorageArgs{
		ID:          "data/0",
		Name:        "data",
		Kind:        "block",
		UnitOwner:   unit0Name.String(),
		Attachments: []string{unit0Name.String(), unit1Name.String()},
		Constraints: &description.StorageInstanceConstraints{
			Pool: "ebs",
			Size: 1024,
		},
	})
	desc.AddStorage(description.StorageArgs{
		ID:          "cache/1",
		Name:        "cache",
		Kind:        "block",
		UnitOwner:   unit1Name.String(),
		Attachments: []string{unit1Name.String()},
		Constraints: &description.StorageInstanceConstraints{
			Pool: "ebs",
			Size: 2048,
		},
	})

	dataFS := desc.AddFilesystem(description.FilesystemArgs{
		ID:           "fs-data-0",
		Size:         1024,
		Storage:      "data/0",
		Pool:         "ebs",
		FilesystemID: "provider-fs-data-0",
	})
	dataFS.AddAttachment(description.FilesystemAttachmentArgs{
		HostMachine: machine0Name.String(),
		MountPoint:  "/srv/data",
	})
	cacheFS := desc.AddFilesystem(description.FilesystemArgs{
		ID:           "fs-cache-1",
		Size:         2048,
		Storage:      "cache/1",
		Pool:         "ebs",
		FilesystemID: "provider-fs-cache-1",
	})
	cacheFS.AddAttachment(description.FilesystemAttachmentArgs{
		HostMachine: machine1Name.String(),
		MountPoint:  "/srv/cache",
	})

	desc.AddVolume(description.VolumeArgs{
		ID:          "0",
		Storage:     "data/0",
		Provisioned: true,
		Persistent:  true,
		Pool:        "ebs",
		Size:        1024,
		VolumeID:    "provider-volume-data-0",
		WWN:         "wwn-data-0",
	})
	desc.AddVolume(description.VolumeArgs{
		ID:          "1",
		Storage:     "cache/1",
		Provisioned: true,
		Persistent:  true,
		Pool:        "ebs",
		Size:        2048,
		VolumeID:    "provider-volume-cache-1",
		WWN:         "wwn-cache-1",
	})

	storagemodelmigration.RegisterImport(
		s.coordinator,
		s.registryGetter,
		s.logger,
	)

	// Act
	err := s.coordinator.Perform(c.Context(), s.scope, desc)
	c.Assert(err, tc.ErrorIsNil)

	// Assert
	dataStorageUUID, err := s.svc.GetStorageInstanceUUIDForID(c.Context(), "data/0")
	c.Assert(err, tc.ErrorIsNil)

	dataAttachments, err := s.svc.GetStorageInstanceAttachments(c.Context(), dataStorageUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(dataAttachments, tc.HasLen, 2)

	cacheStorageUUID, err := s.svc.GetStorageInstanceUUIDForID(c.Context(), "cache/1")
	c.Assert(err, tc.ErrorIsNil)

	cacheAttachments, err := s.svc.GetStorageInstanceAttachments(c.Context(), cacheStorageUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cacheAttachments, tc.HasLen, 1)

	_, err = s.svc.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		c.Context(), dataStorageUUID, unit0UUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.svc.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		c.Context(), dataStorageUUID, unit1UUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.svc.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		c.Context(), cacheStorageUUID, unit1UUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	filesystems, err := s.svc.GetFilesystemsByMachines(
		c.Context(), []coremachine.UUID{machine0UUID, machine1UUID},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(filesystems, tc.HasLen, 2)

	dataFilesystem, err := s.provisioning.GetFilesystemForID(c.Context(), "fs-data-0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(dataFilesystem, tc.DeepEquals, domainstorageprovisioning.Filesystem{
		BackingVolume: &domainstorageprovisioning.FilesystemBackingVolume{
			VolumeID: "0",
		},
		FilesystemID: "fs-data-0",
		ProviderID:   "provider-fs-data-0",
		SizeMiB:      uint64(1024),
	})

	cacheFilesystem, err := s.provisioning.GetFilesystemForID(c.Context(), "fs-cache-1")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cacheFilesystem, tc.DeepEquals, domainstorageprovisioning.Filesystem{
		BackingVolume: &domainstorageprovisioning.FilesystemBackingVolume{
			VolumeID: "1",
		},
		FilesystemID: "fs-cache-1",
		ProviderID:   "provider-fs-cache-1",
		SizeMiB:      uint64(2048),
	})

	dataFilesystemAttachment, err := s.provisioning.GetFilesystemAttachmentForMachine(
		c.Context(), "fs-data-0", machine0UUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(
		dataFilesystemAttachment, tc.DeepEquals,
		domainstorageprovisioning.FilesystemAttachment{
			FilesystemID: "fs-data-0",
			MountPoint:   "/srv/data",
			ReadOnly:     false,
		},
	)

	cacheFilesystemAttachment, err := s.provisioning.GetFilesystemAttachmentForMachine(
		c.Context(), "fs-cache-1", machine1UUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(
		cacheFilesystemAttachment, tc.DeepEquals,
		domainstorageprovisioning.FilesystemAttachment{
			FilesystemID: "fs-cache-1",
			MountPoint:   "/srv/cache",
			ReadOnly:     false,
		},
	)

	dataFilesystemUUID, err := s.provisioning.GetFilesystemUUIDForID(c.Context(), "fs-data-0")
	c.Assert(err, tc.ErrorIsNil)
	dataFilesystemParams, err := s.provisioning.GetFilesystemParams(
		c.Context(), dataFilesystemUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(
		dataFilesystemParams, tc.DeepEquals,
		domainstorageprovisioning.FilesystemParams{
			Attributes: map[string]string{},
			ID:         "fs-data-0",
			Provider:   "ebs",
			ProviderID: ptr("provider-fs-data-0"),
			SizeMiB:    uint64(1024),
			BackingVolume: &domainstorageprovisioning.FilesystemBackingVolume{
				VolumeID: "0",
			},
		},
	)

	cacheFilesystemUUID, err := s.provisioning.GetFilesystemUUIDForID(c.Context(), "fs-cache-1")
	c.Assert(err, tc.ErrorIsNil)
	cacheFilesystemParams, err := s.provisioning.GetFilesystemParams(
		c.Context(), cacheFilesystemUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(
		cacheFilesystemParams, tc.DeepEquals,
		domainstorageprovisioning.FilesystemParams{
			Attributes: map[string]string{},
			ID:         "fs-cache-1",
			Provider:   "ebs",
			ProviderID: ptr("provider-fs-cache-1"),
			SizeMiB:    uint64(2048),
			BackingVolume: &domainstorageprovisioning.FilesystemBackingVolume{
				VolumeID: "1",
			},
		},
	)

	dataVolume, err := s.provisioning.GetVolumeByID(c.Context(), "0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(dataVolume, tc.DeepEquals, domainstorageprovisioning.Volume{
		VolumeID:   "0",
		ProviderID: "provider-volume-data-0",
		SizeMiB:    uint64(1024),
		HardwareID: "",
		WWN:        "wwn-data-0",
		Persistent: true,
	})

	cacheVolume, err := s.provisioning.GetVolumeByID(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cacheVolume, tc.DeepEquals, domainstorageprovisioning.Volume{
		VolumeID:   "1",
		ProviderID: "provider-volume-cache-1",
		SizeMiB:    uint64(2048),
		HardwareID: "",
		WWN:        "wwn-cache-1",
		Persistent: true,
	})

	dataVolumeUUID, err := s.provisioning.GetVolumeUUIDForID(c.Context(), "0")
	c.Assert(err, tc.ErrorIsNil)
	dataVolumeParams, err := s.provisioning.GetVolumeParams(c.Context(), dataVolumeUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(dataVolumeParams, tc.DeepEquals, domainstorageprovisioning.VolumeParams{
		Attributes:           map[string]string{},
		ID:                   "0",
		Provider:             "ebs",
		SizeMiB:              uint64(1024),
		VolumeAttachmentUUID: nil,
	})

	cacheVolumeUUID, err := s.provisioning.GetVolumeUUIDForID(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	cacheVolumeParams, err := s.provisioning.GetVolumeParams(c.Context(), cacheVolumeUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cacheVolumeParams, tc.DeepEquals, domainstorageprovisioning.VolumeParams{
		Attributes:           map[string]string{},
		ID:                   "1",
		Provider:             "ebs",
		SizeMiB:              uint64(2048),
		VolumeAttachmentUUID: nil,
	})
}

func (s *importSuite) TestImportStorageInstancesNonVolumeBackedFilesystems(c *tc.C) {
	// Arrange
	entities := s.createIAASApplication(c, "mysql", 2)
	machine0Name, machine1Name := entities.machineNames[0], entities.machineNames[1]
	machine0UUID, machine1UUID := entities.machineUUIDs[0], entities.machineUUIDs[1]
	unit0Name, unit1Name := entities.unitNames[0], entities.unitNames[1]
	unit0UUID, unit1UUID := entities.unitUUIDs[0], entities.unitUUIDs[1]

	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})
	desc.AddStoragePool(description.StoragePoolArgs{
		Name:     "ebs",
		Provider: "ebs",
	})
	desc.AddStorage(description.StorageArgs{
		ID:          "logs/0",
		Name:        "logs",
		Kind:        "filesystem",
		UnitOwner:   unit0Name.String(),
		Attachments: []string{unit0Name.String(), unit1Name.String()},
		Constraints: &description.StorageInstanceConstraints{
			Pool: "ebs",
			Size: 1024,
		},
	})
	desc.AddStorage(description.StorageArgs{
		ID:          "scratch/1",
		Name:        "scratch",
		Kind:        "filesystem",
		UnitOwner:   unit1Name.String(),
		Attachments: []string{unit1Name.String()},
		Constraints: &description.StorageInstanceConstraints{
			Pool: "ebs",
			Size: 2048,
		},
	})

	logsFS := desc.AddFilesystem(description.FilesystemArgs{
		ID:           "fs-logs-0",
		Size:         1024,
		Storage:      "logs/0",
		Pool:         "ebs",
		FilesystemID: "provider-fs-logs-0",
	})
	logsFS.AddAttachment(description.FilesystemAttachmentArgs{
		HostMachine: machine0Name.String(),
		MountPoint:  "/srv/logs",
	})
	scratchFS := desc.AddFilesystem(description.FilesystemArgs{
		ID:           "fs-scratch-1",
		Size:         2048,
		Storage:      "scratch/1",
		Pool:         "ebs",
		FilesystemID: "provider-fs-scratch-1",
	})
	scratchFS.AddAttachment(description.FilesystemAttachmentArgs{
		HostMachine: machine1Name.String(),
		MountPoint:  "/srv/scratch",
	})

	storagemodelmigration.RegisterImport(
		s.coordinator,
		s.registryGetter,
		s.logger,
	)

	// Act
	err := s.coordinator.Perform(c.Context(), s.scope, desc)
	c.Assert(err, tc.ErrorIsNil)

	// Assert
	logsStorageUUID, err := s.svc.GetStorageInstanceUUIDForID(c.Context(), "logs/0")
	c.Assert(err, tc.ErrorIsNil)

	logsStorageAttachments, err := s.svc.GetStorageInstanceAttachments(c.Context(), logsStorageUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(logsStorageAttachments, tc.HasLen, 2)

	scratchStorageUUID, err := s.svc.GetStorageInstanceUUIDForID(c.Context(), "scratch/1")
	c.Assert(err, tc.ErrorIsNil)

	scratchStorageAttachments, err := s.svc.GetStorageInstanceAttachments(c.Context(), scratchStorageUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(scratchStorageAttachments, tc.HasLen, 1)

	_, err = s.svc.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		c.Context(), logsStorageUUID, unit0UUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.svc.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		c.Context(), logsStorageUUID, unit1UUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.svc.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		c.Context(), scratchStorageUUID, unit1UUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	filesystems, err := s.svc.GetFilesystemsByMachines(
		c.Context(), []coremachine.UUID{machine0UUID, machine1UUID},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(filesystems, tc.HasLen, 2)

	logsFilesystem, err := s.provisioning.GetFilesystemForID(c.Context(), "fs-logs-0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(logsFilesystem, tc.DeepEquals, domainstorageprovisioning.Filesystem{
		BackingVolume: nil,
		FilesystemID:  "fs-logs-0",
		ProviderID:    "provider-fs-logs-0",
		SizeMiB:       uint64(1024),
	})

	scratchFilesystem, err := s.provisioning.GetFilesystemForID(c.Context(), "fs-scratch-1")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(scratchFilesystem, tc.DeepEquals, domainstorageprovisioning.Filesystem{
		BackingVolume: nil,
		FilesystemID:  "fs-scratch-1",
		ProviderID:    "provider-fs-scratch-1",
		SizeMiB:       uint64(2048),
	})

	logsFilesystemAttachment, err := s.provisioning.GetFilesystemAttachmentForMachine(
		c.Context(), "fs-logs-0", machine0UUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(
		logsFilesystemAttachment, tc.DeepEquals,
		domainstorageprovisioning.FilesystemAttachment{
			FilesystemID: "fs-logs-0",
			MountPoint:   "/srv/logs",
			ReadOnly:     false,
		},
	)

	scratchFilesystemAttachment, err := s.provisioning.GetFilesystemAttachmentForMachine(
		c.Context(), "fs-scratch-1", machine1UUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(
		scratchFilesystemAttachment, tc.DeepEquals,
		domainstorageprovisioning.FilesystemAttachment{
			FilesystemID: "fs-scratch-1",
			MountPoint:   "/srv/scratch",
			ReadOnly:     false,
		},
	)

	logsFilesystemUUID, err := s.provisioning.GetFilesystemUUIDForID(c.Context(), "fs-logs-0")
	c.Assert(err, tc.ErrorIsNil)
	logsFilesystemParams, err := s.provisioning.GetFilesystemParams(
		c.Context(), logsFilesystemUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(
		logsFilesystemParams, tc.DeepEquals,
		domainstorageprovisioning.FilesystemParams{
			Attributes:    map[string]string{},
			ID:            "fs-logs-0",
			Provider:      "ebs",
			ProviderID:    ptr("provider-fs-logs-0"),
			SizeMiB:       uint64(1024),
			BackingVolume: nil,
		},
	)

	scratchFilesystemUUID, err := s.provisioning.GetFilesystemUUIDForID(c.Context(), "fs-scratch-1")
	c.Assert(err, tc.ErrorIsNil)
	scratchFilesystemParams, err := s.provisioning.GetFilesystemParams(
		c.Context(), scratchFilesystemUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(
		scratchFilesystemParams, tc.DeepEquals,
		domainstorageprovisioning.FilesystemParams{
			Attributes:    map[string]string{},
			ID:            "fs-scratch-1",
			Provider:      "ebs",
			ProviderID:    ptr("provider-fs-scratch-1"),
			SizeMiB:       uint64(2048),
			BackingVolume: nil,
		},
	)
}

func (s *importSuite) TestImportStorageInstancesVolumesOnly(c *tc.C) {
	// Arrange
	entities := s.createIAASApplication(c, "redis", 2)
	machine0Name, machine1Name := entities.machineNames[0], entities.machineNames[1]
	machine0UUID, machine1UUID := entities.machineUUIDs[0], entities.machineUUIDs[1]
	unit0Name, unit1Name := entities.unitNames[0], entities.unitNames[1]
	unit0UUID, unit1UUID := entities.unitUUIDs[0], entities.unitUUIDs[1]

	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})
	desc.AddStoragePool(description.StoragePoolArgs{
		Name:     "ebs",
		Provider: "ebs",
	})
	desc.AddStorage(description.StorageArgs{
		ID:          "data/0",
		Name:        "data",
		Kind:        "block",
		UnitOwner:   unit0Name.String(),
		Attachments: []string{unit0Name.String(), unit1Name.String()},
		Constraints: &description.StorageInstanceConstraints{
			Pool: "ebs",
			Size: 1024,
		},
	})
	desc.AddStorage(description.StorageArgs{
		ID:          "cache/1",
		Name:        "cache",
		Kind:        "block",
		UnitOwner:   unit1Name.String(),
		Attachments: []string{unit1Name.String()},
		Constraints: &description.StorageInstanceConstraints{
			Pool: "ebs",
			Size: 2048,
		},
	})

	blockDeviceService := blockdeviceservice.NewService(
		blockdevicestate.NewState(s.TxnRunnerFactory()),
		s.logger,
	)
	err := blockDeviceService.SetBlockDevicesForMachineByName(
		c.Context(),
		machine0Name,
		[]coreblockdevice.BlockDevice{{
			DeviceName:  "xvdf",
			DeviceLinks: []string{"/dev/disk/by-id/vol-data-0"},
		}},
	)
	c.Assert(err, tc.ErrorIsNil)
	err = blockDeviceService.SetBlockDevicesForMachineByName(
		c.Context(),
		machine1Name,
		[]coreblockdevice.BlockDevice{{
			DeviceName:  "xvdf",
			DeviceLinks: []string{"/dev/disk/by-id/vol-cache-1"},
		}},
	)
	c.Assert(err, tc.ErrorIsNil)

	dataVolume := desc.AddVolume(description.VolumeArgs{
		ID:          "0",
		Storage:     "data/0",
		Provisioned: true,
		Persistent:  true,
		Pool:        "ebs",
		Size:        1024,
		VolumeID:    "provider-volume-data-0",
		WWN:         "wwn-data-0",
	})
	dataVolume.AddAttachment(description.VolumeAttachmentArgs{
		HostMachine: machine0Name.String(),
		Provisioned: true,
		ReadOnly:    false,
		DeviceName:  "xvdf",
		DeviceLink:  "/dev/disk/by-id/vol-data-0",
	})
	dataVolume.AddAttachmentPlan(description.VolumeAttachmentPlanArgs{
		Machine:    machine0Name.String(),
		DeviceType: "local",
		DeviceAttributes: map[string]string{
			"driver": "virtio",
		},
	})
	cacheVolume := desc.AddVolume(description.VolumeArgs{
		ID:          "1",
		Storage:     "cache/1",
		Provisioned: true,
		Persistent:  true,
		Pool:        "ebs",
		Size:        2048,
		VolumeID:    "provider-volume-cache-1",
		WWN:         "wwn-cache-1",
	})
	cacheVolume.AddAttachment(description.VolumeAttachmentArgs{
		HostMachine: machine1Name.String(),
		Provisioned: true,
		ReadOnly:    true,
		DeviceName:  "xvdf",
		DeviceLink:  "/dev/disk/by-id/vol-cache-1",
	})
	cacheVolume.AddAttachmentPlan(description.VolumeAttachmentPlanArgs{
		Machine:    machine1Name.String(),
		DeviceType: "iscsi",
		DeviceAttributes: map[string]string{
			"iqn": "iqn.2026-03.com.example:cache-1",
		},
	})

	storagemodelmigration.RegisterImport(
		s.coordinator,
		s.registryGetter,
		s.logger,
	)

	// Act
	err = s.coordinator.Perform(c.Context(), s.scope, desc)
	c.Assert(err, tc.ErrorIsNil)

	// Assert
	dataStorageUUID, err := s.svc.GetStorageInstanceUUIDForID(c.Context(), "data/0")
	c.Assert(err, tc.ErrorIsNil)

	dataStorageAttachments, err := s.svc.GetStorageInstanceAttachments(c.Context(), dataStorageUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(dataStorageAttachments, tc.HasLen, 2)

	cacheStorageUUID, err := s.svc.GetStorageInstanceUUIDForID(c.Context(), "cache/1")
	c.Assert(err, tc.ErrorIsNil)

	cacheStorageAttachments, err := s.svc.GetStorageInstanceAttachments(c.Context(), cacheStorageUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cacheStorageAttachments, tc.HasLen, 1)

	_, err = s.svc.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		c.Context(), dataStorageUUID, unit0UUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.svc.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		c.Context(), dataStorageUUID, unit1UUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.svc.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		c.Context(), cacheStorageUUID, unit1UUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	filesystems, err := s.svc.GetFilesystemsByMachines(
		c.Context(), []coremachine.UUID{machine0UUID, machine1UUID},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(filesystems, tc.HasLen, 0)

	volumes, err := s.svc.GetVolumesByMachines(
		c.Context(), []coremachine.UUID{machine0UUID, machine1UUID},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(volumes, tc.HasLen, 2)

	dataImportedVolume, err := s.provisioning.GetVolumeByID(c.Context(), "0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(dataImportedVolume, tc.DeepEquals, domainstorageprovisioning.Volume{
		VolumeID:   "0",
		ProviderID: "provider-volume-data-0",
		SizeMiB:    uint64(1024),
		HardwareID: "",
		WWN:        "wwn-data-0",
		Persistent: true,
	})

	cacheImportedVolume, err := s.provisioning.GetVolumeByID(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cacheImportedVolume, tc.DeepEquals, domainstorageprovisioning.Volume{
		VolumeID:   "1",
		ProviderID: "provider-volume-cache-1",
		SizeMiB:    uint64(2048),
		HardwareID: "",
		WWN:        "wwn-cache-1",
		Persistent: true,
	})

	dataVolumeAttachmentUUID, err := s.provisioning.GetVolumeAttachmentUUIDForVolumeIDMachine(
		c.Context(), "0", machine0UUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	cacheVolumeAttachmentUUID, err := s.provisioning.GetVolumeAttachmentUUIDForVolumeIDMachine(
		c.Context(), "1", machine1UUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	dataVolumeAttachment, err := s.provisioning.GetVolumeAttachment(c.Context(), dataVolumeAttachmentUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(dataVolumeAttachment, tc.DeepEquals, domainstorageprovisioning.VolumeAttachment{
		VolumeID:              "0",
		ReadOnly:              false,
		BlockDeviceName:       "xvdf",
		BlockDeviceLinks:      []string{"/dev/disk/by-id/vol-data-0"},
		BlockDeviceBusAddress: "",
	})

	cacheVolumeAttachment, err := s.provisioning.GetVolumeAttachment(c.Context(), cacheVolumeAttachmentUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cacheVolumeAttachment, tc.DeepEquals, domainstorageprovisioning.VolumeAttachment{
		VolumeID:              "1",
		ReadOnly:              true,
		BlockDeviceName:       "xvdf",
		BlockDeviceLinks:      []string{"/dev/disk/by-id/vol-cache-1"},
		BlockDeviceBusAddress: "",
	})

	dataVolumeAttachmentPlanUUID, err := s.provisioning.GetVolumeAttachmentPlanUUIDForVolumeIDMachine(
		c.Context(), "0", machine0UUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	cacheVolumeAttachmentPlanUUID, err := s.provisioning.GetVolumeAttachmentPlanUUIDForVolumeIDMachine(
		c.Context(), "1", machine1UUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	dataVolumeAttachmentPlan, err := s.provisioning.GetVolumeAttachmentPlan(
		c.Context(), dataVolumeAttachmentPlanUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(dataVolumeAttachmentPlan, tc.DeepEquals, domainstorageprovisioning.VolumeAttachmentPlan{
		Life:       domainlife.Alive,
		DeviceType: domainstorage.VolumeDeviceTypeLocal,
		DeviceAttributes: map[string]string{
			"driver": "virtio",
		},
	})

	cacheVolumeAttachmentPlan, err := s.provisioning.GetVolumeAttachmentPlan(
		c.Context(), cacheVolumeAttachmentPlanUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cacheVolumeAttachmentPlan, tc.DeepEquals, domainstorageprovisioning.VolumeAttachmentPlan{
		Life:       domainlife.Alive,
		DeviceType: domainstorage.VolumeDeviceTypeISCSI,
		DeviceAttributes: map[string]string{
			"iqn": "iqn.2026-03.com.example:cache-1",
		},
	})

	dataVolumeUUID, err := s.provisioning.GetVolumeUUIDForID(c.Context(), "0")
	c.Assert(err, tc.ErrorIsNil)
	dataVolumeParams, err := s.provisioning.GetVolumeParams(c.Context(), dataVolumeUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(dataVolumeParams, tc.DeepEquals, domainstorageprovisioning.VolumeParams{
		Attributes:           map[string]string{},
		ID:                   "0",
		Provider:             "ebs",
		SizeMiB:              uint64(1024),
		VolumeAttachmentUUID: &dataVolumeAttachmentUUID,
	})

	cacheVolumeUUID, err := s.provisioning.GetVolumeUUIDForID(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	cacheVolumeParams, err := s.provisioning.GetVolumeParams(c.Context(), cacheVolumeUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cacheVolumeParams, tc.DeepEquals, domainstorageprovisioning.VolumeParams{
		Attributes:           map[string]string{},
		ID:                   "1",
		Provider:             "ebs",
		SizeMiB:              uint64(2048),
		VolumeAttachmentUUID: &cacheVolumeAttachmentUUID,
	})
}

type createApplicationEntities struct {
	machineNames []coremachine.Name
	machineUUIDs []coremachine.UUID
	unitNames    []coreunit.Name
	unitUUIDs    []coreunit.UUID
}

func (s *importSuite) createIAASApplication(
	c *tc.C,
	appName string,
	unitCount int,
) createApplicationEntities {
	appSt := applicationstate.NewState(
		s.TxnRunnerFactory(),
		model.UUID(s.ModelUUID()),
		clock.WallClock,
		s.logger,
	)

	unitArgs := make([]domainapplication.AddIAASUnitArg, unitCount)
	for i := 0; i < unitCount; i++ {
		machineUUID := tc.Must(c, coremachine.NewUUID)
		netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
		unitArgs[i] = domainapplication.AddIAASUnitArg{
			AddUnitArg: domainapplication.AddUnitArg{
				Placement: deployment.Placement{
					Type: deployment.PlacementTypeUnset,
				},
				NetNodeUUID: netNodeUUID,
			},
			Platform:           deployment.Platform{Channel: "24.04", OSType: deployment.Ubuntu},
			MachineUUID:        machineUUID,
			MachineNetNodeUUID: netNodeUUID,
		}
	}

	appUUID, machineNames, err := appSt.CreateIAASApplication(c.Context(), appName, domainapplication.AddIAASApplicationArg{
		BaseAddApplicationArg: domainapplication.BaseAddApplicationArg{
			Platform: deployment.Platform{
				Channel:      "24.04",
				OSType:       deployment.Ubuntu,
				Architecture: architecture.AMD64,
			},
			Charm: applicationcharm.Charm{
				Metadata: applicationcharm.Metadata{
					Name: appName,
				},
				Manifest: applicationcharm.Manifest{
					Bases: []applicationcharm.Base{{
						Name: "ubuntu",
						Channel: applicationcharm.Channel{
							Track: "24.04",
							Risk:  applicationcharm.RiskStable,
						},
						Architectures: []string{"amd64"},
					}},
				},
				ReferenceName: appName,
				Source:        applicationcharm.CharmHubSource,
				Revision:      1,
				Hash:          "hash",
				Architecture:  architecture.AMD64,
			},
			CharmDownloadInfo: &applicationcharm.DownloadInfo{
				Provenance:  applicationcharm.ProvenanceDownload,
				DownloadURL: "http://example.com",
			},
		},
	}, unitArgs)
	c.Assert(err, tc.ErrorIsNil)

	machineUUIDs := make([]coremachine.UUID, len(machineNames))
	for i, machineName := range machineNames {
		machineUUID, _, err := appSt.GetMachineUUIDAndNetNodeForName(
			c.Context(),
			machineName.String(),
		)
		c.Assert(err, tc.ErrorIsNil)
		machineUUIDs[i] = machineUUID
	}

	unitNames, err := appSt.GetUnitNamesForApplication(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitNames, tc.HasLen, unitCount)

	unitUUIDs := make([]coreunit.UUID, len(unitNames))
	for i, unitName := range unitNames {
		unitUUID, err := appSt.GetUnitUUIDByName(c.Context(), unitName)
		c.Assert(err, tc.ErrorIsNil)
		unitUUIDs[i] = unitUUID
	}

	return createApplicationEntities{
		machineNames: machineNames,
		machineUUIDs: machineUUIDs,
		unitNames:    unitNames,
		unitUUIDs:    unitUUIDs,
	}
}

func (s *importSuite) newStorageRegistryGetter() corestorage.ModelStorageRegistryGetter {
	return corestorage.ConstModelStorageRegistry(func() internalstorage.ProviderRegistry {
		return internalstorage.StaticProviderRegistry{
			Providers: map[internalstorage.ProviderType]internalstorage.Provider{
				"ebs": &dummystorage.StorageProvider{
					StorageScope: internalstorage.ScopeMachine,
					IsDynamic:    true,
					IsReleasable: true,
				},
			},
		}
	})
}

func ptr[T any](v T) *T {
	return &v
}
