// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	stdtesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/blockdevice"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainblockdevice "github.com/juju/juju/domain/blockdevice"
	domainlife "github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	storageprovisioningtesting "github.com/juju/juju/domain/storageprovisioning/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type provisionerSuite struct {
	authorizer *apiservertesting.FakeAuthorizer

	watcherRegistry            *facademocks.MockWatcherRegistry
	storageProvisioningService *MockStorageProvisioningService
	machineService             *MockMachineService
	applicationService         *MockApplicationService
	blockDeviceService         *MockBlockDeviceService

	api *StorageProvisionerAPIv4

	machineName    machine.Name
	modelUUID      model.UUID
	controllerUUID string
}

func TestProvisionerSuite(t *stdtesting.T) {
	tc.Run(t, &provisionerSuite{})
}

func (s *provisionerSuite) setupAPI(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.machineName = machine.Name("0")
	s.modelUUID = modeltesting.GenModelUUID(c)
	s.controllerUUID = coretesting.ControllerTag.Id()

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag(s.machineName.String()),
		Controller: true,
	}

	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)
	s.storageProvisioningService = NewMockStorageProvisioningService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.blockDeviceService = NewMockBlockDeviceService(ctrl)

	var err error
	s.api, err = NewStorageProvisionerAPIv4(
		c.Context(),
		s.watcherRegistry,
		testclock.NewClock(time.Now()),
		s.blockDeviceService,
		s.machineService,
		s.applicationService,
		s.authorizer,
		nil, // storageProviderRegistry
		nil, // storageService
		nil, // statusService
		s.storageProvisioningService,
		loggertesting.WrapCheckLog(c),
		s.modelUUID,
		s.controllerUUID,
	)
	c.Assert(err, tc.IsNil)

	c.Cleanup(func() {
		s.authorizer = nil
		s.watcherRegistry = nil
		s.storageProvisioningService = nil
		s.api = nil
	})

	return ctrl
}

func (s *provisionerSuite) TestVolumes(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")

	vol := storageprovisioning.Volume{
		VolumeID:   "123",
		ProviderID: "vol-1234",
		HardwareID: "hwid",
		WWN:        "wwn",
		Persistent: true,
		SizeMiB:    1000,
	}

	s.storageProvisioningService.EXPECT().GetVolumeByID(
		gomock.Any(), tag.Id()).Return(vol, nil)

	result, err := s.api.Volumes(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.VolumeResults{
		Results: []params.VolumeResult{
			{
				Result: params.Volume{
					VolumeTag: tag.String(),
					Info: params.VolumeInfo{
						ProviderId: "vol-1234",
						HardwareId: "hwid",
						WWN:        "wwn",
						Persistent: true,
						SizeMiB:    1000,
					},
				},
			},
		},
	})
}

func (s *provisionerSuite) TestVolumesNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")

	s.storageProvisioningService.EXPECT().GetVolumeByID(
		gomock.Any(), tag.Id(),
	).Return(
		storageprovisioning.Volume{},
		storageprovisioningerrors.VolumeNotFound,
	)

	results, err := s.api.Volumes(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestVolumesNotProvisioned(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")

	vol := storageprovisioning.Volume{
		ProviderID: "fs-1234",
	}

	s.storageProvisioningService.EXPECT().
		GetVolumeByID(gomock.Any(), tag.Id()).
		Return(vol, nil)

	results, err := s.api.Volumes(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotProvisioned)
}

func (s *provisionerSuite) TestVolumeAttachmentsForMachine(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	machineUUID := machinetesting.GenUUID(c)
	vaUUID := storageprovisioningtesting.GenVolumeAttachmentUUID(c)

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().
		GetVolumeAttachmentUUIDForVolumeIDMachine(gomock.Any(), tag.Id(), machineUUID).
		Return(vaUUID, nil)
	s.storageProvisioningService.EXPECT().GetVolumeAttachment(gomock.Any(), vaUUID).
		Return(storageprovisioning.VolumeAttachment{
			VolumeID:              "123",
			BlockDeviceName:       "blk",
			BlockDeviceLinks:      []string{"/dev/disk/by-id/blocky"},
			BlockDeviceBusAddress: "blk:addr:f00",
			ReadOnly:              true,
		}, nil)

	result, err := s.api.VolumeAttachments(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.VolumeAttachmentResults{
		Results: []params.VolumeAttachmentResult{
			{
				Result: params.VolumeAttachment{
					VolumeTag:  tag.String(),
					MachineTag: names.NewMachineTag(s.machineName.String()).String(),
					Info: params.VolumeAttachmentInfo{
						DeviceName: "blk",
						DeviceLink: "/dev/disk/by-id/blocky",
						BusAddress: "blk:addr:f00",
						ReadOnly:   true,
						PlanInfo:   nil,
					},
				},
			},
		},
	})
}

func (s *provisionerSuite) TestVolumeAttachmentsForMachineNotProvisioned(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	machineUUID := machinetesting.GenUUID(c)
	vaUUID := storageprovisioningtesting.GenVolumeAttachmentUUID(c)

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().
		GetVolumeAttachmentUUIDForVolumeIDMachine(gomock.Any(), tag.Id(), machineUUID).
		Return(vaUUID, nil)
	s.storageProvisioningService.EXPECT().GetVolumeAttachment(gomock.Any(), vaUUID).
		Return(storageprovisioning.VolumeAttachment{
			VolumeID: "fs-1234",
			ReadOnly: true,
		}, nil)

	result, err := s.api.VolumeAttachments(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotProvisioned)
}

func (s *provisionerSuite) TestVolumeAttachmentsForMachineAttachmentNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	machineUUID := machinetesting.GenUUID(c)
	vaUUID := storageprovisioningtesting.GenVolumeAttachmentUUID(c)

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().
		GetVolumeAttachmentUUIDForVolumeIDMachine(gomock.Any(), tag.Id(), machineUUID).
		Return(vaUUID, nil)
	s.storageProvisioningService.EXPECT().GetVolumeAttachment(
		gomock.Any(), vaUUID,
	).Return(
		storageprovisioning.VolumeAttachment{},
		storageprovisioningerrors.VolumeAttachmentNotFound,
	)

	result, err := s.api.VolumeAttachments(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestVolumeAttachmentsForMachineVolumeNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	machineUUID := machinetesting.GenUUID(c)

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().
		GetVolumeAttachmentUUIDForVolumeIDMachine(
			gomock.Any(), tag.Id(), machineUUID,
		).Return("", storageprovisioningerrors.VolumeNotFound)

	result, err := s.api.VolumeAttachments(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestVolumeAttachmentsForMachineMachineNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return("", machineerrors.MachineNotFound)

	result, err := s.api.VolumeAttachments(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestVolumeBlockDevices(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	machineUUID := machinetesting.GenUUID(c)
	vaUUID := storageprovisioningtesting.GenVolumeAttachmentUUID(c)
	bdUUID := tc.Must(c, domainblockdevice.NewBlockDeviceUUID)

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().
		GetVolumeAttachmentUUIDForVolumeIDMachine(gomock.Any(), tag.Id(), machineUUID).
		Return(vaUUID, nil)
	s.storageProvisioningService.EXPECT().GetBlockDeviceForVolumeAttachment(gomock.Any(), vaUUID).
		Return(bdUUID, nil)

	s.blockDeviceService.EXPECT().GetBlockDevice(gomock.Any(), bdUUID).Return(blockdevice.BlockDevice{
		DeviceName: "blk",
		DeviceLinks: []string{
			"/dev/blocky",
			"/dev/sda",
		},
		FilesystemLabel: "lbl",
		FilesystemUUID:  "the devices uuid",
		HardwareId:      "hwid",
		WWN:             "wwn",
		BusAddress:      "blk:addr:foo",
		SizeMiB:         123,
		FilesystemType:  "ext4",
		InUse:           true,
		MountPoint:      "/mnt/blocky",
		SerialId:        "bl0cky",
	}, nil)

	result, err := s.api.VolumeBlockDevices(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.BlockDeviceResults{
		Results: []params.BlockDeviceResult{
			{
				Result: params.BlockDevice{
					DeviceName: "blk",
					DeviceLinks: []string{
						"/dev/blocky",
						"/dev/sda",
					},
					Label:          "lbl",
					UUID:           "the devices uuid",
					HardwareId:     "hwid",
					WWN:            "wwn",
					BusAddress:     "blk:addr:foo",
					SizeMiB:        123,
					FilesystemType: "ext4",
					InUse:          true,
					MountPoint:     "/mnt/blocky",
					SerialId:       "bl0cky",
				},
			},
		},
	})
}

func (s *provisionerSuite) TestVolumeBlockDevicesNotProvisioned(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	machineUUID := machinetesting.GenUUID(c)
	vaUUID := storageprovisioningtesting.GenVolumeAttachmentUUID(c)
	bdUUID := tc.Must(c, domainblockdevice.NewBlockDeviceUUID)

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().
		GetVolumeAttachmentUUIDForVolumeIDMachine(gomock.Any(), tag.Id(), machineUUID).
		Return(vaUUID, nil)
	s.storageProvisioningService.EXPECT().GetBlockDeviceForVolumeAttachment(gomock.Any(), vaUUID).
		Return(bdUUID, nil)

	s.blockDeviceService.EXPECT().GetBlockDevice(
		gomock.Any(), bdUUID).Return(blockdevice.BlockDevice{}, nil)

	result, err := s.api.VolumeBlockDevices(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotProvisioned)
}

func (s *provisionerSuite) TestVolumeBlockDevicesNotProvisionedWithoutBlockDevice(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	machineUUID := machinetesting.GenUUID(c)
	vaUUID := storageprovisioningtesting.GenVolumeAttachmentUUID(c)

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().
		GetVolumeAttachmentUUIDForVolumeIDMachine(gomock.Any(), tag.Id(), machineUUID).
		Return(vaUUID, nil)
	s.storageProvisioningService.EXPECT().GetBlockDeviceForVolumeAttachment(gomock.Any(), vaUUID).
		Return("", storageprovisioningerrors.VolumeAttachmentWithoutBlockDevice)

	result, err := s.api.VolumeBlockDevices(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotProvisioned)
}

func (s *provisionerSuite) TestVolumeBlockDevicesAttachmentNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	machineUUID := machinetesting.GenUUID(c)
	vaUUID := storageprovisioningtesting.GenVolumeAttachmentUUID(c)

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().
		GetVolumeAttachmentUUIDForVolumeIDMachine(gomock.Any(), tag.Id(), machineUUID).
		Return(vaUUID, nil)
	s.storageProvisioningService.EXPECT().GetBlockDeviceForVolumeAttachment(
		gomock.Any(), vaUUID,
	).Return("", storageprovisioningerrors.VolumeAttachmentNotFound)

	result, err := s.api.VolumeBlockDevices(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestVolumeBlockDevicesNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	machineUUID := machinetesting.GenUUID(c)

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().
		GetVolumeAttachmentUUIDForVolumeIDMachine(
			gomock.Any(), tag.Id(), machineUUID,
		).Return("", storageprovisioningerrors.VolumeNotFound)

	result, err := s.api.VolumeBlockDevices(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestVolumeBlockDevicesMachineNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return("", machineerrors.MachineNotFound)

	result, err := s.api.VolumeBlockDevices(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestFilesystems(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")

	fs := storageprovisioning.Filesystem{
		BackingVolume: &storageprovisioning.FilesystemBackingVolume{
			VolumeID: "456",
		},
		FilesystemID: "123",
		ProviderID:   "fs-1234",
		SizeMiB:      1000,
	}

	s.storageProvisioningService.EXPECT().CheckFilesystemForIDExists(
		gomock.Any(), tag.Id(),
	).Return(true, nil)
	s.storageProvisioningService.EXPECT().
		GetFilesystemForID(gomock.Any(), tag.Id()).
		Return(fs, nil)

	result, err := s.api.Filesystems(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.FilesystemResults{
		Results: []params.FilesystemResult{
			{
				Result: params.Filesystem{
					FilesystemTag: tag.String(),
					VolumeTag:     names.NewVolumeTag("456").String(),
					Info: params.FilesystemInfo{
						ProviderId: "fs-1234",
						SizeMiB:    1000,
					},
				},
			},
		},
	})
}

func (s *provisionerSuite) TestFilesystemsNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")

	s.storageProvisioningService.EXPECT().CheckFilesystemForIDExists(
		gomock.Any(), tag.Id(),
	).Return(true, nil)
	s.storageProvisioningService.EXPECT().
		GetFilesystemForID(gomock.Any(), tag.Id()).
		Return(storageprovisioning.Filesystem{}, storageprovisioningerrors.FilesystemNotFound)

	results, err := s.api.Filesystems(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestFilesystemsNotProvisioned(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")

	fs := storageprovisioning.Filesystem{
		BackingVolume: &storageprovisioning.FilesystemBackingVolume{
			VolumeID: "123",
		},
		ProviderID: "fs-1234",
	}

	s.storageProvisioningService.EXPECT().CheckFilesystemForIDExists(
		gomock.Any(), tag.Id(),
	).Return(true, nil)
	s.storageProvisioningService.EXPECT().
		GetFilesystemForID(gomock.Any(), tag.Id()).
		Return(fs, nil)

	results, err := s.api.Filesystems(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotProvisioned)
}

func (s *provisionerSuite) TestFilesystemAttachmentsForMachine(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	machineUUID := machinetesting.GenUUID(c)

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().
		GetFilesystemAttachmentForMachine(gomock.Any(), tag.Id(), machineUUID).
		Return(storageprovisioning.FilesystemAttachment{
			FilesystemID: "fs-1234",
			MountPoint:   "/mnt/foo",
			ReadOnly:     true,
		}, nil)

	result, err := s.api.FilesystemAttachments(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.FilesystemAttachmentResults{
		Results: []params.FilesystemAttachmentResult{
			{
				Result: params.FilesystemAttachment{
					FilesystemTag: tag.String(),
					MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
					Info: params.FilesystemAttachmentInfo{
						MountPoint: "/mnt/foo",
						ReadOnly:   true,
					},
				},
			},
		},
	})
}

func (s *provisionerSuite) TestFilesystemAttachmentsForMachineNotProvisioned(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	machineUUID := machinetesting.GenUUID(c)

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().
		GetFilesystemAttachmentForMachine(gomock.Any(), tag.Id(), machineUUID).
		Return(storageprovisioning.FilesystemAttachment{
			FilesystemID: "fs-1234",
			ReadOnly:     true,
		}, nil)

	result, err := s.api.FilesystemAttachments(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotProvisioned)
}

func (s *provisionerSuite) TestFilesystemAttachmentsForMachineAttachmentNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	machineUUID := machinetesting.GenUUID(c)

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().
		GetFilesystemAttachmentForMachine(gomock.Any(), tag.Id(), machineUUID).
		Return(storageprovisioning.FilesystemAttachment{}, storageprovisioningerrors.FilesystemAttachmentNotFound)

	result, err := s.api.FilesystemAttachments(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestFilesystemAttachmentsForMachineFilesystemNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	machineUUID := machinetesting.GenUUID(c)

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().
		GetFilesystemAttachmentForMachine(gomock.Any(), tag.Id(), machineUUID).
		Return(storageprovisioning.FilesystemAttachment{}, storageprovisioningerrors.FilesystemNotFound)

	result, err := s.api.FilesystemAttachments(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestFilesystemAttachmentsForMachineMachineNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return("", machineerrors.MachineNotFound)

	result, err := s.api.FilesystemAttachments(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestFilesystemAttachmentsForUnit(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	unitTag := names.NewUnitTag("mysql/666")
	unitUUID := unittesting.GenUnitUUID(c)

	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return(unitUUID, nil)
	s.storageProvisioningService.EXPECT().
		GetFilesystemAttachmentForUnit(gomock.Any(), tag.Id(), unitUUID).
		Return(storageprovisioning.FilesystemAttachment{
			FilesystemID: "fs-1234",
			MountPoint:   "/mnt/foo",
			ReadOnly:     true,
		}, nil)

	result, err := s.api.FilesystemAttachments(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    unitTag.String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.FilesystemAttachmentResults{
		Results: []params.FilesystemAttachmentResult{
			{
				Result: params.FilesystemAttachment{
					FilesystemTag: tag.String(),
					MachineTag:    unitTag.String(),
					Info: params.FilesystemAttachmentInfo{
						MountPoint: "/mnt/foo",
						ReadOnly:   true,
					},
				},
			},
		},
	})
}

func (s *provisionerSuite) TestFilesystemAttachmentsForUnitNotProvisioned(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	unitTag := names.NewUnitTag("mysql/666")
	unitUUID := unittesting.GenUnitUUID(c)

	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return(unitUUID, nil)
	s.storageProvisioningService.EXPECT().
		GetFilesystemAttachmentForUnit(gomock.Any(), tag.Id(), unitUUID).
		Return(storageprovisioning.FilesystemAttachment{
			FilesystemID: "fs-1234",
			ReadOnly:     true,
		}, nil)

	result, err := s.api.FilesystemAttachments(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    unitTag.String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotProvisioned)
}

func (s *provisionerSuite) TestFilesystemAttachmentsForUnitAttachmentNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	unitTag := names.NewUnitTag("mysql/666")
	unitUUID := unittesting.GenUnitUUID(c)

	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return(unitUUID, nil)
	s.storageProvisioningService.EXPECT().
		GetFilesystemAttachmentForUnit(gomock.Any(), tag.Id(), unitUUID).
		Return(storageprovisioning.FilesystemAttachment{}, storageprovisioningerrors.FilesystemAttachmentNotFound)

	result, err := s.api.FilesystemAttachments(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    unitTag.String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestFilesystemAttachmentsForUnitFilesystemNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	unitTag := names.NewUnitTag("mysql/666")
	unitUUID := unittesting.GenUnitUUID(c)

	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return(unitUUID, nil)
	s.storageProvisioningService.EXPECT().
		GetFilesystemAttachmentForUnit(gomock.Any(), tag.Id(), unitUUID).
		Return(storageprovisioning.FilesystemAttachment{}, storageprovisioningerrors.FilesystemNotFound)

	result, err := s.api.FilesystemAttachments(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    unitTag.String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestFilesystemAttachmentsForUnitUnitNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	unitTag := names.NewUnitTag("mysql/666")

	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return("", applicationerrors.UnitNotFound)

	result, err := s.api.FilesystemAttachments(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    unitTag.String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

// TestFilesystemParamsNotFound tests that when asking for the params of a
// filesystem which does not exist in the model results in a permission error
// to the caller.
func (s *provisionerSuite) TestFilesystemParamsNotFound(c *tc.C) {
	defer s.setupAPI(c).Finish()

	tag := names.NewFilesystemTag("123")

	s.storageProvisioningService.EXPECT().GetStorageResourceTagsForModel(
		gomock.Any(),
	).Return(map[string]string{}, nil).AnyTimes()
	s.storageProvisioningService.EXPECT().CheckFilesystemForIDExists(
		gomock.Any(), tag.Id(),
	).Return(true, nil)
	s.storageProvisioningService.EXPECT().GetFilesystemUUIDForID(
		gomock.Any(), tag.Id(),
	).Return("", storageprovisioningerrors.FilesystemNotFound)

	results, err := s.api.FilesystemParams(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		},
	})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error.Code, tc.Equals, params.CodeUnauthorized)
}

// TestFilesystemParamsNotFoundWithUUID tests that when asking for the params of
// a filesystem which does not exist in the model results in a permission error
// to the caller.
func (s *provisionerSuite) TestFilesystemParamsNotFoundWithUUID(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	fsUUID := storageprovisioningtesting.GenFilesystemUUID(c)

	s.storageProvisioningService.EXPECT().GetStorageResourceTagsForModel(
		gomock.Any(),
	).Return(map[string]string{}, nil).AnyTimes()
	s.storageProvisioningService.EXPECT().CheckFilesystemForIDExists(
		gomock.Any(), tag.Id(),
	).Return(true, nil)
	s.storageProvisioningService.EXPECT().GetFilesystemUUIDForID(
		gomock.Any(), tag.Id(),
	).Return(fsUUID, nil)
	s.storageProvisioningService.EXPECT().GetFilesystemParams(
		gomock.Any(), fsUUID,
	).Return(storageprovisioning.FilesystemParams{}, storageprovisioningerrors.FilesystemNotFound)

	results, err := s.api.FilesystemParams(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		},
	})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error.Code, tc.Equals, params.CodeUnauthorized)
}

func (s *provisionerSuite) TestFilesystemParams(c *tc.C) {
	defer s.setupAPI(c).Finish()

	tag := names.NewFilesystemTag("123")
	fsUUID := storageprovisioningtesting.GenFilesystemUUID(c)

	s.storageProvisioningService.EXPECT().GetStorageResourceTagsForModel(
		gomock.Any(),
	).Return(map[string]string{
		"tag1": "value1",
	}, nil)
	s.storageProvisioningService.EXPECT().CheckFilesystemForIDExists(
		gomock.Any(), tag.Id(),
	).Return(true, nil)
	s.storageProvisioningService.EXPECT().GetFilesystemUUIDForID(
		gomock.Any(), tag.Id(),
	).Return(fsUUID, nil)
	s.storageProvisioningService.EXPECT().GetFilesystemParams(
		gomock.Any(), fsUUID,
	).Return(storageprovisioning.FilesystemParams{
		Attributes: map[string]string{
			"foo": "bar",
		},
		ID:       "fs-id123",
		Provider: "myprovider",
		SizeMiB:  10,
	}, nil)

	results, err := s.api.FilesystemParams(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		},
	})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Result, tc.DeepEquals, params.FilesystemParams{
		Attributes: map[string]any{
			"foo": "bar",
		},
		FilesystemTag: tag.String(),
		SizeMiB:       10,
		Provider:      "myprovider",
		Tags: map[string]string{
			"tag1": "value1",
		},
	})
}

// TestFilesystemAttachmentParamsMachineNotFound tests the case where the
// filesystem params are requested
func (s *provisionerSuite) TestFilesystemAttachmentParamsMachineNotFound(c *tc.C) {
	defer s.setupAPI(c).Finish()

	tag := names.NewFilesystemTag("123")
	machineTag := names.NewMachineTag("123")
	machineUUID := machinetesting.GenUUID(c)

	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(machineTag.Id())).Return(
		machineUUID, nil,
	)
	s.storageProvisioningService.EXPECT().GetFilesystemAttachmentUUIDForFilesystemIDMachine(
		gomock.Any(), tag.Id(), machineUUID,
	).Return("", machineerrors.MachineNotFound)

	results, err := s.api.FilesystemAttachmentParams(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				AttachmentTag: tag.String(),
				MachineTag:    machineTag.String(),
			},
		},
	})

	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestFilesystemAttachmentParamsUnitNotFound(c *tc.C) {
	defer s.setupAPI(c).Finish()

	tag := names.NewFilesystemTag("123")
	unitTag := names.NewUnitTag("foo/123")
	unitUUID := unittesting.GenUnitUUID(c)

	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("foo/123")).Return(
		unitUUID, nil,
	)
	s.storageProvisioningService.EXPECT().GetFilesystemAttachmentUUIDForFilesystemIDUnit(
		gomock.Any(), tag.Id(), unitUUID,
	).Return("", applicationerrors.UnitNotFound)

	results, err := s.api.FilesystemAttachmentParams(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				AttachmentTag: tag.String(),
				MachineTag:    unitTag.String(),
			},
		},
	})

	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestFilesystemAttachmentParamsFilesystemNotFound(c *tc.C) {
	defer s.setupAPI(c).Finish()

	tag := names.NewFilesystemTag("123")
	unitTag := names.NewUnitTag("foo/123")
	unitUUID := unittesting.GenUnitUUID(c)

	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("foo/123")).Return(
		unitUUID, nil,
	)
	s.storageProvisioningService.EXPECT().GetFilesystemAttachmentUUIDForFilesystemIDUnit(
		gomock.Any(), tag.Id(), unitUUID,
	).Return("", storageprovisioningerrors.FilesystemNotFound)

	results, err := s.api.FilesystemAttachmentParams(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				AttachmentTag: tag.String(),
				MachineTag:    unitTag.String(),
			},
		},
	})

	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestFilesystemAttachmentParams(c *tc.C) {
	defer s.setupAPI(c).Finish()

	tag := names.NewFilesystemTag("123")
	unitTag := names.NewUnitTag("foo/123")
	unitUUID := unittesting.GenUnitUUID(c)
	fsaUUID := storageprovisioningtesting.GenFilesystemAttachmentUUID(c)

	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("foo/123")).Return(
		unitUUID, nil,
	)
	s.storageProvisioningService.EXPECT().GetFilesystemAttachmentUUIDForFilesystemIDUnit(
		gomock.Any(), tag.Id(), unitUUID,
	).Return(fsaUUID, nil)
	s.storageProvisioningService.EXPECT().GetFilesystemAttachmentParams(
		gomock.Any(), fsaUUID,
	).Return(
		storageprovisioning.FilesystemAttachmentParams{
			MachineInstanceID: "12",
			Provider:          "myprovider",
			ProviderID:        "env-123",
			MountPoint:        "/var/foo",
			ReadOnly:          true,
		}, nil,
	)

	results, err := s.api.FilesystemAttachmentParams(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				AttachmentTag: tag.String(),
				MachineTag:    unitTag.String(),
			},
		},
	})

	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Result, tc.DeepEquals, params.FilesystemAttachmentParams{
		FilesystemTag: tag.String(),
		MachineTag:    unitTag.String(),
		ProviderId:    "env-123",
		InstanceId:    "12",
		Provider:      "myprovider",
		MountPoint:    "/var/foo",
		ReadOnly:      true,
	})
}

func (s *provisionerSuite) TestWatchMachines(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	machineChanged := make(chan struct{}, 1)
	machineChanged <- struct{}{}

	sourceWatcher := watchertest.NewMockNotifyWatcher(machineChanged)
	machineUUID := machinetesting.GenUUID(c)

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.machineService.EXPECT().
		WatchMachineCloudInstances(gomock.Any(), machineUUID).
		Return(sourceWatcher, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return("66", nil)

	results, err := s.api.WatchMachines(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewMachineTag(s.machineName.String()).String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.IsNil)
	c.Assert(result.NotifyWatcherId, tc.Equals, "66")
}

func (s *provisionerSuite) TestWatchMachinesNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return("", machineerrors.MachineNotFound)

	results, err := s.api.WatchMachines(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewMachineTag(s.machineName.String()).String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestVolumeAttachmentParamsMachineNotFound(c *tc.C) {
	defer s.setupAPI(c).Finish()

	tag := names.NewVolumeTag("123")
	machineTag := names.NewMachineTag("123")
	machineUUID := machinetesting.GenUUID(c)

	s.machineService.EXPECT().GetMachineUUID(
		gomock.Any(), machine.Name(machineTag.Id())).Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().GetVolumeAttachmentUUIDForVolumeIDMachine(
		gomock.Any(), tag.Id(), machineUUID).Return(
		"", machineerrors.MachineNotFound)

	results, err := s.api.VolumeAttachmentParams(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				AttachmentTag: tag.String(),
				MachineTag:    machineTag.String(),
			},
		},
	})

	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestVolumeAttachmentParamsVolumeNotFound(c *tc.C) {
	defer s.setupAPI(c).Finish()

	tag := names.NewVolumeTag("123")
	machineTag := names.NewMachineTag("123")
	machineUUID := machinetesting.GenUUID(c)

	s.machineService.EXPECT().GetMachineUUID(
		gomock.Any(), machine.Name(machineTag.Id())).Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().GetVolumeAttachmentUUIDForVolumeIDMachine(
		gomock.Any(), tag.Id(), machineUUID).Return(
		"", storageprovisioningerrors.VolumeNotFound)

	results, err := s.api.VolumeAttachmentParams(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				AttachmentTag: tag.String(),
				MachineTag:    machineTag.String(),
			},
		},
	})

	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestVolumeAttachmentParams(c *tc.C) {
	defer s.setupAPI(c).Finish()

	tag := names.NewVolumeTag("123")
	machineTag := names.NewMachineTag("11")
	machineUUID := machinetesting.GenUUID(c)
	vaUUID := storageprovisioningtesting.GenVolumeAttachmentUUID(c)

	s.machineService.EXPECT().GetMachineUUID(
		gomock.Any(), machine.Name("11")).Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().GetVolumeAttachmentUUIDForVolumeIDMachine(
		gomock.Any(), tag.Id(), machineUUID).Return(vaUUID, nil)
	s.storageProvisioningService.EXPECT().GetVolumeAttachmentParams(
		gomock.Any(), vaUUID,
	).Return(
		storageprovisioning.VolumeAttachmentParams{
			MachineInstanceID: "12",
			Provider:          "myprovider",
			ProviderID:        "env-123",
			ReadOnly:          true,
		}, nil,
	)

	results, err := s.api.VolumeAttachmentParams(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				AttachmentTag: tag.String(),
				MachineTag:    machineTag.String(),
			},
		},
	})

	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Result, tc.DeepEquals, params.VolumeAttachmentParams{
		VolumeTag:  tag.String(),
		MachineTag: machineTag.String(),
		ProviderId: "env-123",
		InstanceId: "12",
		Provider:   "myprovider",
		ReadOnly:   true,
	})
}

// TestVolumeParamsNotFound tests that when asking for the params of a volume
// which does not exist in the model results in a permission error
// to the caller.
func (s *provisionerSuite) TestVolumeParamsNotFound(c *tc.C) {
	defer s.setupAPI(c).Finish()

	tag := names.NewVolumeTag("123")

	s.storageProvisioningService.EXPECT().GetStorageResourceTagsForModel(
		gomock.Any()).Return(map[string]string{}, nil).AnyTimes()
	s.storageProvisioningService.EXPECT().GetVolumeUUIDForID(
		gomock.Any(), tag.Id()).Return(
		"",
		storageprovisioningerrors.VolumeNotFound,
	)

	results, err := s.api.VolumeParams(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		},
	})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error.Code, tc.Equals, params.CodeUnauthorized)
}

// TestVolumeParamsNotFoundWithUUID tests that when asking for the params of a
// volume which does not exist in the model results in a permission error to the
// caller.
func (s *provisionerSuite) TestVolumeParamsNotFoundWithUUID(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	volUUID := storageprovisioningtesting.GenVolumeUUID(c)

	s.storageProvisioningService.EXPECT().GetStorageResourceTagsForModel(
		gomock.Any()).Return(map[string]string{}, nil).AnyTimes()
	s.storageProvisioningService.EXPECT().GetVolumeUUIDForID(
		gomock.Any(), tag.Id()).Return(volUUID, nil)
	s.storageProvisioningService.EXPECT().GetVolumeParams(
		gomock.Any(), volUUID).Return(
		storageprovisioning.VolumeParams{},
		storageprovisioningerrors.VolumeNotFound,
	)

	results, err := s.api.VolumeParams(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		},
	})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error.Code, tc.Equals, params.CodeUnauthorized)
}

func (s *provisionerSuite) TestVolumeParams(c *tc.C) {
	defer s.setupAPI(c).Finish()

	tag := names.NewVolumeTag("123")
	volUUID := storageprovisioningtesting.GenVolumeUUID(c)

	s.storageProvisioningService.EXPECT().GetStorageResourceTagsForModel(
		gomock.Any(),
	).Return(map[string]string{
		"tag1": "value1",
	}, nil)
	s.storageProvisioningService.EXPECT().GetVolumeUUIDForID(
		gomock.Any(), tag.Id(),
	).Return(volUUID, nil)
	s.storageProvisioningService.EXPECT().GetVolumeParams(
		gomock.Any(), volUUID,
	).Return(storageprovisioning.VolumeParams{
		Attributes: map[string]string{
			"foo": "bar",
		},
		ID:       "vol-id123",
		Provider: "myprovider",
		SizeMiB:  10,
	}, nil)

	results, err := s.api.VolumeParams(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		},
	})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Result, tc.DeepEquals, params.VolumeParams{
		Attributes: map[string]any{
			"foo": "bar",
		},
		VolumeTag: tag.String(),
		SizeMiB:   10,
		Provider:  "myprovider",
		Tags: map[string]string{
			"tag1": "value1",
		},
	})
}

func (s *provisionerSuite) TestWatchVolumesForModel(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	volumeChanged := make(chan []string, 1)
	volumeChanged <- []string{"vol1", "vol2"}

	sourceWatcher := watchertest.NewMockStringsWatcher(volumeChanged)

	s.storageProvisioningService.EXPECT().
		WatchModelProvisionedVolumes(gomock.Any()).
		Return(sourceWatcher, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return("66", nil)

	results, err := s.api.WatchVolumes(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewModelTag(s.modelUUID.String()).String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.IsNil)
	c.Assert(result.StringsWatcherId, tc.Equals, "66")
	c.Assert(result.Changes, tc.DeepEquals, []string{"vol1", "vol2"})
}

func (s *provisionerSuite) TestWatchVolumesForMachine(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	volumeChanged := make(chan []string, 1)
	volumeChanged <- []string{"vol1", "vol2"}

	sourceWatcher := watchertest.NewMockStringsWatcher(volumeChanged)
	machineUUID := machinetesting.GenUUID(c)

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().
		WatchMachineProvisionedVolumes(gomock.Any(), machineUUID).
		Return(sourceWatcher, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return("66", nil)

	results, err := s.api.WatchVolumes(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewMachineTag(s.machineName.String()).String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.IsNil)
	c.Assert(result.StringsWatcherId, tc.Equals, "66")
	c.Assert(result.Changes, tc.DeepEquals, []string{"vol1", "vol2"})
}

func (s *provisionerSuite) TestWatchVolumesForMachineNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return("", machineerrors.MachineNotFound)

	results, err := s.api.WatchVolumes(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewMachineTag(s.machineName.String()).String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestWatchFilesystemsForModel(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	filesystemChanged := make(chan []string, 1)
	filesystemChanged <- []string{"1", "2"}

	sourceWatcher := watchertest.NewMockStringsWatcher(filesystemChanged)

	s.storageProvisioningService.EXPECT().
		WatchModelProvisionedFilesystems(gomock.Any()).
		Return(sourceWatcher, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return("66", nil)

	results, err := s.api.WatchFilesystems(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewModelTag(s.modelUUID.String()).String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.IsNil)
	c.Assert(result.StringsWatcherId, tc.Equals, "66")
	c.Assert(result.Changes, tc.DeepEquals, []string{"1", "2"})
}

func (s *provisionerSuite) TestWatchFilesystemsForMachine(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	filesystemChanged := make(chan []string, 1)
	filesystemChanged <- []string{"1", "2"}

	sourceWatcher := watchertest.NewMockStringsWatcher(filesystemChanged)
	machineUUID := machinetesting.GenUUID(c)

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().
		WatchMachineProvisionedFilesystems(gomock.Any(), machineUUID).
		Return(sourceWatcher, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return("66", nil)

	results, err := s.api.WatchFilesystems(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewMachineTag(s.machineName.String()).String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.IsNil)
	c.Assert(result.StringsWatcherId, tc.Equals, "66")
	c.Assert(result.Changes, tc.DeepEquals, []string{"1", "2"})
}

func (s *provisionerSuite) TestWatchFilesystemsForMachineNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return("", machineerrors.MachineNotFound)

	results, err := s.api.WatchFilesystems(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewMachineTag(s.machineName.String()).String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestWatchVolumeAttachmentPlans(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	attachmentChanged := make(chan []string, 1)
	attachmentChanged <- []string{"1", "2"}
	machineUUID := machinetesting.GenUUID(c)
	sourceWatcher := watchertest.NewMockStringsWatcher(attachmentChanged)

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().
		WatchVolumeAttachmentPlans(gomock.Any(), machineUUID).
		Return(sourceWatcher, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return("66", nil)

	results, err := s.api.WatchVolumeAttachmentPlans(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewMachineTag(s.machineName.String()).String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.IsNil)
	c.Assert(result.MachineStorageIdsWatcherId, tc.Equals, "66")
	c.Assert(result.Changes, tc.SameContents, []params.MachineStorageId{
		{
			MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
			AttachmentTag: names.NewVolumeTag("1").String(),
		},
		{
			MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
			AttachmentTag: names.NewVolumeTag("2").String(),
		},
	})
}

func (s *provisionerSuite) TestWatchVolumeAttachmentPlansMachineNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return("", machineerrors.MachineNotFound)

	results, err := s.api.WatchVolumeAttachmentPlans(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewMachineTag(s.machineName.String()).String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestWatchVolumeAttachmentsForMachine(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	attachmentChanged := make(chan []string, 1)
	attachmentChanged <- []string{"volume-attachment-uuid-1", "volume-attachment-uuid-2"}
	sourceWatcher := watchertest.NewMockStringsWatcher(attachmentChanged)

	machineUUID := machinetesting.GenUUID(c)
	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().
		WatchMachineProvisionedVolumeAttachments(gomock.Any(), machineUUID).
		Return(sourceWatcher, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return("66", nil)

	s.storageProvisioningService.EXPECT().
		GetVolumeAttachmentIDs(gomock.Any(), []string{"volume-attachment-uuid-1", "volume-attachment-uuid-2"}).
		Return(map[string]storageprovisioning.VolumeAttachmentID{
			"volume-attachment-uuid-1": {
				MachineName: &s.machineName,
				VolumeID:    "1",
			},
			"volume-attachment-uuid-2": {
				MachineName: &s.machineName,
				VolumeID:    "2",
			},
		}, nil)

	results, err := s.api.WatchVolumeAttachments(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewMachineTag(s.machineName.String()).String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.IsNil)
	c.Assert(result.MachineStorageIdsWatcherId, tc.Equals, "66")
	c.Assert(result.Changes, tc.SameContents, []params.MachineStorageId{
		{
			MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
			AttachmentTag: names.NewVolumeTag("1").String(),
		},
		{
			MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
			AttachmentTag: names.NewVolumeTag("2").String(),
		},
	})
}
func (s *provisionerSuite) TestWatchVolumeAttachmentsForMachineNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return("", machineerrors.MachineNotFound)
	results, err := s.api.WatchVolumeAttachments(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewMachineTag(s.machineName.String()).String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestWatchVolumeAttachmentsForModel(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	attachmentChanged := make(chan []string, 1)
	attachmentChanged <- []string{"volume-attachment-uuid-1", "volume-attachment-uuid-2"}
	sourceWatcher := watchertest.NewMockStringsWatcher(attachmentChanged)

	s.storageProvisioningService.EXPECT().
		WatchModelProvisionedVolumeAttachments(gomock.Any()).
		Return(sourceWatcher, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return("66", nil)

	s.storageProvisioningService.EXPECT().
		GetVolumeAttachmentIDs(gomock.Any(), []string{"volume-attachment-uuid-1", "volume-attachment-uuid-2"}).
		Return(map[string]storageprovisioning.VolumeAttachmentID{
			"volume-attachment-uuid-1": {
				UnitName: ptr(coreunit.Name("foo/1")),
				VolumeID: "1",
			},
			"volume-attachment-uuid-2": {
				UnitName: ptr(coreunit.Name("foo/2")),
				VolumeID: "2",
			},
		}, nil)

	results, err := s.api.WatchVolumeAttachments(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewModelTag(s.modelUUID.String()).String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.IsNil)
	c.Assert(result.MachineStorageIdsWatcherId, tc.Equals, "66")
	c.Assert(result.Changes, tc.SameContents, []params.MachineStorageId{
		{
			MachineTag:    names.NewUnitTag("foo/1").String(),
			AttachmentTag: names.NewVolumeTag("1").String(),
		},
		{
			MachineTag:    names.NewUnitTag("foo/2").String(),
			AttachmentTag: names.NewVolumeTag("2").String(),
		},
	})
}

func (s *provisionerSuite) TestWatchFilesystemAttachmentsForMachine(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	attachmentChanged := make(chan []string, 1)
	attachmentChanged <- []string{"filesystem-attachment-uuid-1", "filesystem-attachment-uuid-2"}
	sourceWatcher := watchertest.NewMockStringsWatcher(attachmentChanged)

	machineUUID := machinetesting.GenUUID(c)
	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().
		WatchMachineProvisionedFilesystemAttachments(gomock.Any(), machineUUID).
		Return(sourceWatcher, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return("66", nil)

	s.storageProvisioningService.EXPECT().
		GetFilesystemAttachmentIDs(gomock.Any(), []string{"filesystem-attachment-uuid-1", "filesystem-attachment-uuid-2"}).
		Return(map[string]storageprovisioning.FilesystemAttachmentID{
			"filesystem-attachment-uuid-1": {
				MachineName:  &s.machineName,
				FilesystemID: "1",
			},
			"filesystem-attachment-uuid-2": {
				MachineName:  &s.machineName,
				FilesystemID: "2",
			},
		}, nil)

	results, err := s.api.WatchFilesystemAttachments(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewMachineTag(s.machineName.String()).String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.IsNil)
	c.Assert(result.MachineStorageIdsWatcherId, tc.Equals, "66")
	c.Assert(result.Changes, tc.SameContents, []params.MachineStorageId{
		{
			MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
			AttachmentTag: names.NewFilesystemTag("1").String(),
		},
		{
			MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
			AttachmentTag: names.NewFilesystemTag("2").String(),
		},
	})
}

func (s *provisionerSuite) TestWatchFilesystemAttachmentsForMachineNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return("", machineerrors.MachineNotFound)

	results, err := s.api.WatchFilesystemAttachments(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewMachineTag(s.machineName.String()).String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestWatchFilesystemAttachmentsForModel(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	attachmentChanged := make(chan []string, 1)
	attachmentChanged <- []string{"filesystem-attachment-uuid-1", "filesystem-attachment-uuid-2"}
	sourceWatcher := watchertest.NewMockStringsWatcher(attachmentChanged)

	s.storageProvisioningService.EXPECT().
		WatchModelProvisionedFilesystemAttachments(gomock.Any()).
		Return(sourceWatcher, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return("66", nil)

	s.storageProvisioningService.EXPECT().
		GetFilesystemAttachmentIDs(gomock.Any(), []string{"filesystem-attachment-uuid-1", "filesystem-attachment-uuid-2"}).
		Return(map[string]storageprovisioning.FilesystemAttachmentID{
			"filesystem-attachment-uuid-1": {
				UnitName:     ptr(coreunit.Name("foo/1")),
				FilesystemID: "1",
			},
			"filesystem-attachment-uuid-2": {
				UnitName:     ptr(coreunit.Name("foo/2")),
				FilesystemID: "2",
			},
		}, nil)

	results, err := s.api.WatchFilesystemAttachments(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewModelTag(s.modelUUID.String()).String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.IsNil)
	c.Assert(result.MachineStorageIdsWatcherId, tc.Equals, "66")
	c.Assert(result.Changes, tc.SameContents, []params.MachineStorageId{
		{
			MachineTag:    names.NewUnitTag("foo/1").String(),
			AttachmentTag: names.NewFilesystemTag("1").String(),
		},
		{
			MachineTag:    names.NewUnitTag("foo/2").String(),
			AttachmentTag: names.NewFilesystemTag("2").String(),
		},
	})
}

func (s *provisionerSuite) TestLifeForVolume(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	volumeUUID := storageprovisioningtesting.GenVolumeUUID(c)

	s.storageProvisioningService.EXPECT().GetVolumeUUIDForID(
		gomock.Any(), tag.Id(),
	).Return(volumeUUID, nil)
	s.storageProvisioningService.EXPECT().GetVolumeLife(
		gomock.Any(), volumeUUID,
	).Return(domainlife.Alive, nil)

	result, err := s.api.Life(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{
				Life: corelife.Alive,
			},
		},
	})
}

func (s *provisionerSuite) TestLifeForVolumeWithUUIDNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")

	s.storageProvisioningService.EXPECT().GetVolumeUUIDForID(
		gomock.Any(), tag.Id(),
	).Return("", storageprovisioningerrors.VolumeNotFound)

	result, err := s.api.Life(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestLifeForVolumeWithVolumeNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	volumeUUID := storageprovisioningtesting.GenVolumeUUID(c)

	s.storageProvisioningService.EXPECT().GetVolumeUUIDForID(
		gomock.Any(), tag.Id(),
	).Return(volumeUUID, nil)
	s.storageProvisioningService.EXPECT().GetVolumeLife(
		gomock.Any(), volumeUUID,
	).Return(-1, storageprovisioningerrors.VolumeNotFound)

	result, err := s.api.Life(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestLifeForFilesystem(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	filesystemUUID := storageprovisioningtesting.GenFilesystemUUID(c)

	s.storageProvisioningService.EXPECT().CheckFilesystemForIDExists(
		gomock.Any(), tag.Id(),
	).Return(true, nil)

	s.storageProvisioningService.EXPECT().GetFilesystemUUIDForID(
		gomock.Any(), tag.Id(),
	).Return(filesystemUUID, nil)
	s.storageProvisioningService.EXPECT().GetFilesystemLife(
		gomock.Any(), filesystemUUID,
	).Return(domainlife.Alive, nil)

	result, err := s.api.Life(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{
				Life: corelife.Alive,
			},
		},
	})
}

func (s *provisionerSuite) TestLifeForFilesystemWithUUIDNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")

	s.storageProvisioningService.EXPECT().CheckFilesystemForIDExists(
		gomock.Any(), tag.Id(),
	).Return(true, nil)

	s.storageProvisioningService.EXPECT().GetFilesystemUUIDForID(
		gomock.Any(), tag.Id(),
	).Return("", storageprovisioningerrors.FilesystemNotFound)

	result, err := s.api.Life(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestLifeForFilesystemWithFilesystemNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	filesystemUUID := storageprovisioningtesting.GenFilesystemUUID(c)

	s.storageProvisioningService.EXPECT().CheckFilesystemForIDExists(
		gomock.Any(), tag.Id(),
	).Return(true, nil)

	s.storageProvisioningService.EXPECT().GetFilesystemUUIDForID(
		gomock.Any(), tag.Id(),
	).Return(filesystemUUID, nil)
	s.storageProvisioningService.EXPECT().GetFilesystemLife(
		gomock.Any(), filesystemUUID,
	).Return(-1, storageprovisioningerrors.FilesystemNotFound)

	result, err := s.api.Life(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestAttachmentLifeForFilesystemMachine(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	machineUUID := machinetesting.GenUUID(c)
	filesystemAttachmentUUID := storageprovisioningtesting.GenFilesystemAttachmentUUID(c)

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().GetFilesystemAttachmentUUIDForFilesystemIDMachine(
		gomock.Any(), tag.Id(), machineUUID,
	).Return(filesystemAttachmentUUID, nil)
	s.storageProvisioningService.EXPECT().
		GetFilesystemAttachmentLife(gomock.Any(), filesystemAttachmentUUID).
		Return(domainlife.Alive, nil)

	result, err := s.api.AttachmentLife(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{
				Life: corelife.Alive,
			},
		},
	})
}

func (s *provisionerSuite) TestAttachmentLifeForFilesystemMachineWithMachineNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return("", machineerrors.MachineNotFound)

	result, err := s.api.AttachmentLife(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestAttachmentLifeForFilesystemMachineWithFilesystemAttachmentNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	machineUUID := machinetesting.GenUUID(c)

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().GetFilesystemAttachmentUUIDForFilesystemIDMachine(
		gomock.Any(), tag.Id(), machineUUID,
	).Return("", storageprovisioningerrors.FilesystemAttachmentNotFound)

	result, err := s.api.AttachmentLife(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestAttachmentLifeForFilesystemMachineWithFilesystemNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	machineUUID := machinetesting.GenUUID(c)

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().GetFilesystemAttachmentUUIDForFilesystemIDMachine(
		gomock.Any(), tag.Id(), machineUUID,
	).Return("", storageprovisioningerrors.FilesystemNotFound)

	result, err := s.api.AttachmentLife(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestAttachmentLifeForFilesystemMachineWithFilesystemAttachmentNotFound2(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	machineUUID := machinetesting.GenUUID(c)
	filesystemAttachmentUUID := storageprovisioningtesting.GenFilesystemAttachmentUUID(c)

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().GetFilesystemAttachmentUUIDForFilesystemIDMachine(
		gomock.Any(), tag.Id(), machineUUID,
	).Return(filesystemAttachmentUUID, nil)
	s.storageProvisioningService.EXPECT().
		GetFilesystemAttachmentLife(gomock.Any(), filesystemAttachmentUUID).
		Return(-1, storageprovisioningerrors.FilesystemAttachmentNotFound)

	result, err := s.api.AttachmentLife(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestAttachmentLifeForFilesystemMachineWithFilesystemNotFound2(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	machineUUID := machinetesting.GenUUID(c)
	filesystemAttachmentUUID := storageprovisioningtesting.GenFilesystemAttachmentUUID(c)

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().GetFilesystemAttachmentUUIDForFilesystemIDMachine(
		gomock.Any(), tag.Id(), machineUUID,
	).Return(filesystemAttachmentUUID, nil)
	s.storageProvisioningService.EXPECT().
		GetFilesystemAttachmentLife(gomock.Any(), filesystemAttachmentUUID).
		Return(-1, storageprovisioningerrors.FilesystemAttachmentNotFound)

	result, err := s.api.AttachmentLife(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestAttachmentLifeForFilesystemUnit(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	unitTag := names.NewUnitTag("mysql/666")
	unitUUID := unittesting.GenUnitUUID(c)
	filesystemAttachmentUUID := storageprovisioningtesting.GenFilesystemAttachmentUUID(c)

	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return(unitUUID, nil)
	s.storageProvisioningService.EXPECT().GetFilesystemAttachmentUUIDForFilesystemIDUnit(
		gomock.Any(), tag.Id(), unitUUID,
	).Return(filesystemAttachmentUUID, nil)
	s.storageProvisioningService.EXPECT().
		GetFilesystemAttachmentLife(gomock.Any(), filesystemAttachmentUUID).
		Return(domainlife.Alive, nil)

	result, err := s.api.AttachmentLife(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    unitTag.String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{
				Life: corelife.Alive,
			},
		},
	})
}

func (s *provisionerSuite) TestAttachmentLifeForFilesystemUnitWithUnitNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	unitTag := names.NewUnitTag("mysql/666")

	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return("", applicationerrors.UnitNotFound)

	result, err := s.api.AttachmentLife(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    unitTag.String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestAttachmentLifeForFilesystemUnitWithFilesystemAttachmentNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	unitTag := names.NewUnitTag("mysql/666")
	unitUUID := unittesting.GenUnitUUID(c)

	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return(unitUUID, nil)
	s.storageProvisioningService.EXPECT().GetFilesystemAttachmentUUIDForFilesystemIDUnit(
		gomock.Any(), tag.Id(), unitUUID,
	).Return("", storageprovisioningerrors.FilesystemAttachmentNotFound)

	result, err := s.api.AttachmentLife(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    unitTag.String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestAttachmentLifeForFilesystemUnitWithFilesystemNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	unitTag := names.NewUnitTag("mysql/666")
	unitUUID := unittesting.GenUnitUUID(c)
	filesystemAttachmentUUID := storageprovisioningtesting.GenFilesystemAttachmentUUID(c)

	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return(unitUUID, nil)
	s.storageProvisioningService.EXPECT().GetFilesystemAttachmentUUIDForFilesystemIDUnit(
		gomock.Any(), tag.Id(), unitUUID,
	).Return(filesystemAttachmentUUID, nil)
	s.storageProvisioningService.EXPECT().
		GetFilesystemAttachmentLife(gomock.Any(), filesystemAttachmentUUID).
		Return(-1, storageprovisioningerrors.FilesystemAttachmentNotFound)

	result, err := s.api.AttachmentLife(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    unitTag.String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestAttachmentLifeForFilesystemUnitWithFilesystemAttachmentNotFound2(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	unitTag := names.NewUnitTag("mysql/666")
	unitUUID := unittesting.GenUnitUUID(c)
	filesystemAttachmentUUID := storageprovisioningtesting.GenFilesystemAttachmentUUID(c)

	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return(unitUUID, nil)
	s.storageProvisioningService.EXPECT().GetFilesystemAttachmentUUIDForFilesystemIDUnit(
		gomock.Any(), tag.Id(), unitUUID,
	).Return(filesystemAttachmentUUID, nil)
	s.storageProvisioningService.EXPECT().
		GetFilesystemAttachmentLife(gomock.Any(), filesystemAttachmentUUID).
		Return(-1, storageprovisioningerrors.FilesystemAttachmentNotFound)

	result, err := s.api.AttachmentLife(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    unitTag.String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestAttachmentLifeForFilesystemUnitWithFilesystemNotFound2(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	unitTag := names.NewUnitTag("mysql/666")
	unitUUID := unittesting.GenUnitUUID(c)
	filesystemAttachmentUUID := storageprovisioningtesting.GenFilesystemAttachmentUUID(c)

	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return(unitUUID, nil)
	s.storageProvisioningService.EXPECT().GetFilesystemAttachmentUUIDForFilesystemIDUnit(
		gomock.Any(), tag.Id(), unitUUID,
	).Return(filesystemAttachmentUUID, nil)
	s.storageProvisioningService.EXPECT().
		GetFilesystemAttachmentLife(gomock.Any(), filesystemAttachmentUUID).
		Return(-1, storageprovisioningerrors.FilesystemAttachmentNotFound)

	result, err := s.api.AttachmentLife(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    unitTag.String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestAttachmentLifeForVolumeMachine(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	machineUUID := machinetesting.GenUUID(c)
	volumeAttachmentUUID := storageprovisioningtesting.GenVolumeAttachmentUUID(c)

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().GetVolumeAttachmentUUIDForVolumeIDMachine(
		gomock.Any(), tag.Id(), machineUUID,
	).Return(volumeAttachmentUUID, nil)
	s.storageProvisioningService.EXPECT().
		GetVolumeAttachmentLife(gomock.Any(), volumeAttachmentUUID).
		Return(domainlife.Alive, nil)

	result, err := s.api.AttachmentLife(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{
				Life: corelife.Alive,
			},
		},
	})
}
func (s *provisionerSuite) TestAttachmentLifeForVolumeMachineWithMachineNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return("", machineerrors.MachineNotFound)

	result, err := s.api.AttachmentLife(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestAttachmentLifeForVolumeMachineWithVolumeAttachmentNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	machineUUID := machinetesting.GenUUID(c)

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().GetVolumeAttachmentUUIDForVolumeIDMachine(
		gomock.Any(), tag.Id(), machineUUID,
	).Return("", storageprovisioningerrors.VolumeAttachmentNotFound)

	result, err := s.api.AttachmentLife(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestAttachmentLifeForVolumeMachineWithVolumeNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	machineUUID := machinetesting.GenUUID(c)

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().GetVolumeAttachmentUUIDForVolumeIDMachine(
		gomock.Any(), tag.Id(), machineUUID,
	).Return("", storageprovisioningerrors.VolumeNotFound)

	result, err := s.api.AttachmentLife(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestAttachmentLifeForVolumeMachineWithVolumeAttachmentNotFound2(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	machineUUID := machinetesting.GenUUID(c)
	volumeAttachmentUUID := storageprovisioningtesting.GenVolumeAttachmentUUID(c)

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().GetVolumeAttachmentUUIDForVolumeIDMachine(
		gomock.Any(), tag.Id(), machineUUID,
	).Return(volumeAttachmentUUID, nil)
	s.storageProvisioningService.EXPECT().
		GetVolumeAttachmentLife(gomock.Any(), volumeAttachmentUUID).
		Return(-1, storageprovisioningerrors.VolumeAttachmentNotFound)

	result, err := s.api.AttachmentLife(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestAttachmentLifeForVolumeMachineWithVolumeNotFound2(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	machineUUID := machinetesting.GenUUID(c)
	volumeAttachmentUUID := storageprovisioningtesting.GenVolumeAttachmentUUID(c)

	s.machineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.storageProvisioningService.EXPECT().GetVolumeAttachmentUUIDForVolumeIDMachine(
		gomock.Any(), tag.Id(), machineUUID,
	).Return(volumeAttachmentUUID, nil)
	s.storageProvisioningService.EXPECT().
		GetVolumeAttachmentLife(gomock.Any(), volumeAttachmentUUID).
		Return(-1, storageprovisioningerrors.VolumeAttachmentNotFound)

	result, err := s.api.AttachmentLife(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    names.NewMachineTag(s.machineName.String()).String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestAttachmentLifeForVolumeUnit(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	unitTag := names.NewUnitTag("mysql/666")

	result, err := s.api.AttachmentLife(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{
				MachineTag:    unitTag.String(),
				AttachmentTag: tag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error.Code, tc.Equals, params.CodeNotImplemented)
}

func (s *provisionerSuite) TestSetFilesystemInfo(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")

	info := storageprovisioning.FilesystemProvisionedInfo{
		ProviderID: "fs-123",
		SizeMiB:    100,
	}
	svc := s.storageProvisioningService
	svc.EXPECT().CheckFilesystemForIDExists(gomock.Any(), "123").Return(true, nil)
	svc.EXPECT().SetFilesystemProvisionedInfo(gomock.Any(), "123", info).Return(nil)

	result, err := s.api.SetFilesystemInfo(c.Context(), params.Filesystems{
		Filesystems: []params.Filesystem{
			{
				FilesystemTag: tag.String(),
				Info: params.FilesystemInfo{
					ProviderId: "fs-123",
					SizeMiB:    100,
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.IsNil)
}

func (s *provisionerSuite) TestSetFilesystemInfoWithBackingVolume(c *tc.C) {
	c.Skip("skipped until volume backed filesystems are supported")

	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	fsTag := names.NewFilesystemTag("123")
	volTag := names.NewVolumeTag("456")

	info := storageprovisioning.FilesystemProvisionedInfo{
		ProviderID: "fs-123",
		SizeMiB:    100,
	}
	svc := s.storageProvisioningService
	svc.EXPECT().CheckFilesystemForIDExists(gomock.Any(), "123").Return(true, nil)
	svc.EXPECT().SetFilesystemProvisionedInfo(gomock.Any(), "123", info).Return(nil)

	result, err := s.api.SetFilesystemInfo(c.Context(), params.Filesystems{
		Filesystems: []params.Filesystem{
			{
				FilesystemTag: fsTag.String(),
				VolumeTag:     volTag.String(),
				Info: params.FilesystemInfo{
					ProviderId: "fs-123",
					SizeMiB:    100,
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.IsNil)
}

func (s *provisionerSuite) TestSetFilesystemInfoNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	info := storageprovisioning.FilesystemProvisionedInfo{
		ProviderID: "fs-123",
		SizeMiB:    100,
	}

	svc := s.storageProvisioningService
	svc.EXPECT().CheckFilesystemForIDExists(gomock.Any(), "123").Return(true, nil)
	svc.EXPECT().SetFilesystemProvisionedInfo(gomock.Any(), "123", info).Return(
		storageprovisioningerrors.FilesystemNotFound)

	result, err := s.api.SetFilesystemInfo(c.Context(), params.Filesystems{
		Filesystems: []params.Filesystem{
			{
				FilesystemTag: tag.String(),
				Info: params.FilesystemInfo{
					ProviderId: "fs-123",
					SizeMiB:    100,
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.NotNil)
	c.Assert(result.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestSetFilesystemInfoNoPool(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")

	svc := s.storageProvisioningService
	svc.EXPECT().CheckFilesystemForIDExists(gomock.Any(), "123").Return(true, nil)

	result, err := s.api.SetFilesystemInfo(c.Context(), params.Filesystems{
		Filesystems: []params.Filesystem{
			{
				FilesystemTag: tag.String(),
				Info: params.FilesystemInfo{
					ProviderId: "fs-123",
					SizeMiB:    100,
					Pool:       "not allowed",
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.NotNil)
}

func (s *provisionerSuite) TestSetVolumeInfo(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")

	info := storageprovisioning.VolumeProvisionedInfo{
		ProviderID: "fs-123",
		SizeMiB:    100,
		HardwareID: "abc",
		WWN:        "xyz",
		Persistent: true,
	}
	svc := s.storageProvisioningService
	svc.EXPECT().SetVolumeProvisionedInfo(gomock.Any(), "123", info).Return(nil)

	result, err := s.api.SetVolumeInfo(c.Context(), params.Volumes{
		Volumes: []params.Volume{
			{
				VolumeTag: tag.String(),
				Info: params.VolumeInfo{
					ProviderId: "fs-123",
					SizeMiB:    100,
					HardwareId: "abc",
					WWN:        "xyz",
					Persistent: true,
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.IsNil)
}

func (s *provisionerSuite) TestSetVolumeInfoNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")

	info := storageprovisioning.VolumeProvisionedInfo{
		ProviderID: "fs-123",
		SizeMiB:    100,
		HardwareID: "abc",
		WWN:        "xyz",
		Persistent: true,
	}
	svc := s.storageProvisioningService
	svc.EXPECT().SetVolumeProvisionedInfo(gomock.Any(), "123", info).Return(
		storageprovisioningerrors.VolumeNotFound)

	result, err := s.api.SetVolumeInfo(c.Context(), params.Volumes{
		Volumes: []params.Volume{
			{
				VolumeTag: tag.String(),
				Info: params.VolumeInfo{
					ProviderId: "fs-123",
					SizeMiB:    100,
					HardwareId: "abc",
					WWN:        "xyz",
					Persistent: true,
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.NotNil)
	c.Assert(result.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *provisionerSuite) TestSetVolumeInfoNoPool(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")

	result, err := s.api.SetVolumeInfo(c.Context(), params.Volumes{
		Volumes: []params.Volume{
			{
				VolumeTag: tag.String(),
				Info: params.VolumeInfo{
					ProviderId: "fs-123",
					SizeMiB:    100,
					HardwareId: "abc",
					WWN:        "xyz",
					Persistent: true,
					Pool:       "not allowed",
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.NotNil)
}

func (s *provisionerSuite) TestSetVolumeAttachmentInfo(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	machineTag := names.NewMachineTag("5")
	machineUUID := machinetesting.GenUUID(c)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(),
		machine.Name(machineTag.Id())).Return(machineUUID, nil).AnyTimes()
	volAttachUUID := storageprovisioningtesting.GenVolumeAttachmentUUID(c)
	volAttachPlanUUID := storageprovisioningtesting.GenVolumeAttachmentPlanUUID(c)
	bdUUID := tc.Must(c, domainblockdevice.NewBlockDeviceUUID)
	info := storageprovisioning.VolumeAttachmentProvisionedInfo{
		ReadOnly:        true,
		BlockDeviceUUID: &bdUUID,
	}
	planInfo := storageprovisioning.VolumeAttachmentPlanProvisionedInfo{
		DeviceType: storageprovisioning.PlanDeviceTypeISCSI,
		DeviceAttributes: map[string]string{
			"a": "b",
		},
	}
	blockDevice := blockdevice.BlockDevice{
		DeviceName:  "x",
		DeviceLinks: []string{"y"},
		BusAddress:  "z",
	}

	s.blockDeviceService.EXPECT().MatchOrCreateBlockDevice(gomock.Any(),
		machineUUID, blockDevice).Return(bdUUID, nil)

	svc := s.storageProvisioningService
	svc.EXPECT().GetVolumeAttachmentUUIDForVolumeIDMachine(gomock.Any(),
		tag.Id(), machineUUID).Return(volAttachUUID, nil)
	svc.EXPECT().GetVolumeAttachmentPlanUUIDForVolumeIDMachine(
		gomock.Any(), tag.Id(), machineUUID,
	).Return(volAttachPlanUUID, nil)
	svc.EXPECT().SetVolumeAttachmentProvisionedInfo(gomock.Any(),
		volAttachUUID, info).Return(nil)
	svc.EXPECT().SetVolumeAttachmentPlanProvisionedInfo(
		gomock.Any(), volAttachPlanUUID, planInfo,
	).Return(nil)

	result, err := s.api.SetVolumeAttachmentInfo(c.Context(), params.VolumeAttachments{
		VolumeAttachments: []params.VolumeAttachment{
			{
				VolumeTag:  tag.String(),
				MachineTag: machineTag.String(),
				Info: params.VolumeAttachmentInfo{
					DeviceName: "x",
					DeviceLink: "y",
					BusAddress: "z",
					ReadOnly:   true,
					PlanInfo: &params.VolumeAttachmentPlanInfo{
						DeviceType: storage.DeviceTypeISCSI,
						DeviceAttributes: map[string]string{
							"a": "b",
						},
					},
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.IsNil)
}

func (s *provisionerSuite) TestSetVolumeAttachmentInfoErrors(c *tc.C) {
	// TODO(storage): test when volume attachments are missing or volume attach-
	// ment plans are missing (when plan info specified)
}

func (s *provisionerSuite) TestGetVolumeAttachmentPlan(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	machineTag := names.NewMachineTag("5")
	machineUUID := machinetesting.GenUUID(c)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(),
		machine.Name(machineTag.Id())).Return(machineUUID, nil)
	volAttachPlanUUID := storageprovisioningtesting.GenVolumeAttachmentPlanUUID(c)

	attrs := map[string]string{
		"a": "x",
		"b": "y",
		"c": "z",
	}
	vap := storageprovisioning.VolumeAttachmentPlan{
		Life:             domainlife.Dying,
		DeviceType:       storageprovisioning.PlanDeviceTypeISCSI,
		DeviceAttributes: attrs,
	}
	svc := s.storageProvisioningService
	svc.EXPECT().GetVolumeAttachmentPlanUUIDForVolumeIDMachine(
		gomock.Any(), tag.Id(), machineUUID,
	).Return(volAttachPlanUUID, nil)
	svc.EXPECT().GetVolumeAttachmentPlan(gomock.Any(), volAttachPlanUUID).Return(
		vap, nil,
	)

	result, err := s.api.VolumeAttachmentPlans(c.Context(), params.MachineStorageIds{
		Ids: []params.MachineStorageId{
			{MachineTag: machineTag.String(), AttachmentTag: tag.String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.IsNil)
	c.Assert(result.Results[0].Result, tc.DeepEquals, params.VolumeAttachmentPlan{
		VolumeTag:  tag.String(),
		MachineTag: machineTag.String(),
		Life:       corelife.Dying,
		PlanInfo: params.VolumeAttachmentPlanInfo{
			DeviceType:       storage.DeviceTypeISCSI,
			DeviceAttributes: attrs,
		},
	})
}

func (s *provisionerSuite) TestGetVolumeAttachmentPlanErrors(c *tc.C) {
	// TODO(storage): test to ensure get volume attachment plan errors when
	// there is no machine or the volume attachment plan does not exist.
}

func (s *provisionerSuite) TestCreateVolumeAttachmentPlan(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	machineTag := names.NewMachineTag("5")
	machineUUID := machinetesting.GenUUID(c)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(),
		machine.Name(machineTag.Id()),
	).Return(machineUUID, nil)
	volAttachUUID := storageprovisioningtesting.GenVolumeAttachmentUUID(c)

	attrs := map[string]string{
		"a": "x",
		"b": "y",
		"c": "z",
	}
	svc := s.storageProvisioningService

	svc.EXPECT().GetVolumeAttachmentUUIDForVolumeIDMachine(
		gomock.Any(), tag.Id(), machineUUID,
	).Return(volAttachUUID, nil)

	svc.EXPECT().CreateVolumeAttachmentPlan(
		gomock.Any(),
		volAttachUUID,
		storageprovisioning.PlanDeviceTypeISCSI,
		attrs,
	).Return(storageprovisioningtesting.GenVolumeAttachmentPlanUUID(c), nil)

	result, err := s.api.CreateVolumeAttachmentPlans(c.Context(), params.VolumeAttachmentPlans{
		VolumeAttachmentPlans: []params.VolumeAttachmentPlan{
			{
				VolumeTag:  tag.String(),
				MachineTag: machineTag.String(),
				PlanInfo: params.VolumeAttachmentPlanInfo{
					DeviceType:       storage.DeviceTypeISCSI,
					DeviceAttributes: attrs,
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.IsNil)
}

func (s *provisionerSuite) TestCreateVolumeAttachmentPlanErrors(c *tc.C) {
	// TODO(storage): test to ensure create volume attachment plan errors when
	// there is no machine or the volume attachment does not exist for which the
	// plan is being created for.
}

func (s *provisionerSuite) TestSetVolumeAttachmentPlanBlockInfo(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	machineTag := names.NewMachineTag("5")
	machineUUID := machinetesting.GenUUID(c)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(),
		machine.Name(machineTag.Id()),
	).Return(machineUUID, nil).AnyTimes()
	volumeAttachPlanUUID := storageprovisioningtesting.GenVolumeAttachmentPlanUUID(c)

	bdUUID := tc.Must(c, domainblockdevice.NewBlockDeviceUUID)
	blockDeviceInfo := blockdevice.BlockDevice{
		DeviceName:      "a",
		DeviceLinks:     []string{"b"},
		FilesystemLabel: "c",
		FilesystemUUID:  "d",
		HardwareId:      "e",
		SizeMiB:         0xf,
		WWN:             "h",
		BusAddress:      "i",
		FilesystemType:  "j",
		InUse:           true,
		MountPoint:      "k",
		SerialId:        "l",
	}

	s.blockDeviceService.EXPECT().MatchOrCreateBlockDevice(gomock.Any(),
		machineUUID, blockDeviceInfo).Return(bdUUID, nil)

	svc := s.storageProvisioningService
	svc.EXPECT().GetVolumeAttachmentPlanUUIDForVolumeIDMachine(
		gomock.Any(), tag.Id(), machineUUID,
	).Return(volumeAttachPlanUUID, nil)
	svc.EXPECT().SetVolumeAttachmentPlanProvisionedBlockDevice(
		gomock.Any(),
		volumeAttachPlanUUID,
		bdUUID,
	).Return(nil)

	result, err := s.api.SetVolumeAttachmentPlanBlockInfo(c.Context(), params.VolumeAttachmentPlans{
		VolumeAttachmentPlans: []params.VolumeAttachmentPlan{
			{
				VolumeTag:  tag.String(),
				MachineTag: machineTag.String(),
				BlockDevice: &params.BlockDevice{
					DeviceName:     "a",
					DeviceLinks:    []string{"b"},
					Label:          "c",
					UUID:           "d",
					HardwareId:     "e",
					SizeMiB:        0xf,
					WWN:            "h",
					BusAddress:     "i",
					FilesystemType: "j",
					InUse:          true,
					MountPoint:     "k",
					SerialId:       "l",
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.IsNil)
}

func (s *provisionerSuite) TestSetVolumeAttachmentPlanBlockInfoInvalid(c *tc.C) {
	// TODO(storage): test to ensure when set volume attachment plan block info
	// is called with arguments that are unsettable values (e.g. plan info and
	// life), that it errors.
}

func (s *provisionerSuite) TestSetVolumeAttachmentPlanBlockInfoErrors(c *tc.C) {
	// TODO(storage): test to ensure that set volume attachment plan block info
	// errors when the volume attachment plan has not been created.
}

func (s *provisionerSuite) TestSetFilesystemAttachmentInfoMachine(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	machineTag := names.NewMachineTag("5")
	machineUUID := machinetesting.GenUUID(c)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(),
		machine.Name(machineTag.Id())).Return(machineUUID, nil)

	info := storageprovisioning.FilesystemAttachmentProvisionedInfo{
		MountPoint: "x",
		ReadOnly:   true,
	}
	svc := s.storageProvisioningService
	svc.EXPECT().SetFilesystemAttachmentProvisionedInfoForMachine(gomock.Any(),
		tag.Id(), machineUUID, info).Return(nil)

	result, err := s.api.SetFilesystemAttachmentInfo(c.Context(), params.FilesystemAttachments{
		FilesystemAttachments: []params.FilesystemAttachment{
			{
				FilesystemTag: tag.String(),
				MachineTag:    machineTag.String(),
				Info: params.FilesystemAttachmentInfo{
					MountPoint: "x",
					ReadOnly:   true,
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.IsNil)
}

func (s *provisionerSuite) TestSetFilesystemAttachmentInfoMachineErrors(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	machineTag := names.NewMachineTag("5")
	machineUUID := machinetesting.GenUUID(c)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(),
		machine.Name(machineTag.Id())).Return(machineUUID, nil)

	info := storageprovisioning.FilesystemAttachmentProvisionedInfo{
		MountPoint: "x",
		ReadOnly:   true,
	}
	svc := s.storageProvisioningService
	svc.EXPECT().SetFilesystemAttachmentProvisionedInfoForMachine(gomock.Any(),
		tag.Id(), machineUUID, info).
		Return(storageprovisioningerrors.FilesystemAttachmentNotFound)

	result, err := s.api.SetFilesystemAttachmentInfo(c.Context(), params.FilesystemAttachments{
		FilesystemAttachments: []params.FilesystemAttachment{
			{
				FilesystemTag: tag.String(),
				MachineTag:    machineTag.String(),
				Info: params.FilesystemAttachmentInfo{
					MountPoint: "x",
					ReadOnly:   true,
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.NotNil)
}

func (s *provisionerSuite) TestSetFilesystemAttachmentInfoUnit(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	unitTag := names.NewUnitTag("app/5")
	unitUUID := unittesting.GenUnitUUID(c)
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(),
		coreunit.Name(unitTag.Id())).Return(unitUUID, nil)

	info := storageprovisioning.FilesystemAttachmentProvisionedInfo{
		MountPoint: "x",
		ReadOnly:   true,
	}
	svc := s.storageProvisioningService
	svc.EXPECT().SetFilesystemAttachmentProvisionedInfoForUnit(gomock.Any(),
		tag.Id(), unitUUID, info).Return(nil)

	result, err := s.api.SetFilesystemAttachmentInfo(c.Context(), params.FilesystemAttachments{
		FilesystemAttachments: []params.FilesystemAttachment{
			{
				FilesystemTag: tag.String(),
				MachineTag:    unitTag.String(),
				Info: params.FilesystemAttachmentInfo{
					MountPoint: "x",
					ReadOnly:   true,
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.IsNil)
}

func (s *provisionerSuite) TestSetFilesystemAttachmentInfoUnitErrors(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")
	unitTag := names.NewUnitTag("app/5")
	unitUUID := unittesting.GenUnitUUID(c)
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(),
		coreunit.Name(unitTag.Id())).Return(unitUUID, nil)

	info := storageprovisioning.FilesystemAttachmentProvisionedInfo{
		MountPoint: "x",
		ReadOnly:   true,
	}
	svc := s.storageProvisioningService
	svc.EXPECT().SetFilesystemAttachmentProvisionedInfoForUnit(gomock.Any(),
		tag.Id(), unitUUID, info).
		Return(storageprovisioningerrors.FilesystemAttachmentNotFound)

	result, err := s.api.SetFilesystemAttachmentInfo(c.Context(), params.FilesystemAttachments{
		FilesystemAttachments: []params.FilesystemAttachment{
			{
				FilesystemTag: tag.String(),
				MachineTag:    unitTag.String(),
				Info: params.FilesystemAttachmentInfo{
					MountPoint: "x",
					ReadOnly:   true,
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.NotNil)
}
