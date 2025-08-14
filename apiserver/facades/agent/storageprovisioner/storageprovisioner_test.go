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
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/watchertest"
	applicationerrors "github.com/juju/juju/domain/application/errors"
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

	watcherRegistry                *facademocks.MockWatcherRegistry
	mockStorageProvisioningService *MockStorageProvisioningService
	mockMachineService             *MockMachineService
	mockApplicationService         *MockApplicationService

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
	s.mockStorageProvisioningService = NewMockStorageProvisioningService(ctrl)
	s.mockMachineService = NewMockMachineService(ctrl)
	s.mockApplicationService = NewMockApplicationService(ctrl)

	var err error
	s.api, err = NewStorageProvisionerAPIv4(
		c.Context(),
		s.watcherRegistry,
		testclock.NewClock(time.Now()),
		nil, // blockDeviceService
		nil, // configService
		s.mockMachineService,
		s.mockApplicationService,
		s.authorizer,
		nil, // storageProviderRegistry
		nil, // storageService
		nil, // statusService
		s.mockStorageProvisioningService,
		loggertesting.WrapCheckLog(c),
		s.modelUUID,
		s.controllerUUID,
	)
	c.Assert(err, tc.IsNil)

	c.Cleanup(func() {
		s.authorizer = nil
		s.watcherRegistry = nil
		s.mockStorageProvisioningService = nil
		s.api = nil
	})

	return ctrl
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

	s.mockStorageProvisioningService.EXPECT().CheckFilesystemForIDExists(
		gomock.Any(), tag.Id(),
	).Return(true, nil)
	s.mockStorageProvisioningService.EXPECT().
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
						Size:       1000,
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

	s.mockStorageProvisioningService.EXPECT().CheckFilesystemForIDExists(
		gomock.Any(), tag.Id(),
	).Return(true, nil)
	s.mockStorageProvisioningService.EXPECT().
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

	s.mockStorageProvisioningService.EXPECT().CheckFilesystemForIDExists(
		gomock.Any(), tag.Id(),
	).Return(true, nil)
	s.mockStorageProvisioningService.EXPECT().
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
	machineUUID := machine.GenUUID(c)

	s.mockMachineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.mockStorageProvisioningService.EXPECT().
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
	machineUUID := machine.GenUUID(c)

	s.mockMachineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.mockStorageProvisioningService.EXPECT().
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
	machineUUID := machine.GenUUID(c)

	s.mockMachineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.mockStorageProvisioningService.EXPECT().
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
	machineUUID := machine.GenUUID(c)

	s.mockMachineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.mockStorageProvisioningService.EXPECT().
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

	s.mockMachineService.EXPECT().
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
	unitUUID := coreunit.GenUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().
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
	unitUUID := coreunit.GenUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().
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
	unitUUID := coreunit.GenUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().
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
	unitUUID := coreunit.GenUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().
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

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return("", applicationerrors.UnitNotFound)

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

func (s *provisionerSuite) TestWatchVolumesForModel(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	volumeChanged := make(chan []string, 1)
	volumeChanged <- []string{"vol1", "vol2"}

	sourceWatcher := watchertest.NewMockStringsWatcher(volumeChanged)

	s.mockStorageProvisioningService.EXPECT().
		WatchModelProvisionedVolumes(gomock.Any()).
		Return(sourceWatcher, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("66", nil)

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
	machineUUID := machine.GenUUID(c)

	s.mockMachineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.mockStorageProvisioningService.EXPECT().
		WatchMachineProvisionedVolumes(gomock.Any(), machineUUID).
		Return(sourceWatcher, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("66", nil)

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

	s.mockMachineService.EXPECT().
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

	s.mockStorageProvisioningService.EXPECT().
		WatchModelProvisionedFilesystems(gomock.Any()).
		Return(sourceWatcher, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("66", nil)

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
	machineUUID := machine.GenUUID(c)

	s.mockMachineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.mockStorageProvisioningService.EXPECT().
		WatchMachineProvisionedFilesystems(gomock.Any(), machineUUID).
		Return(sourceWatcher, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("66", nil)

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

	s.mockMachineService.EXPECT().
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
	machineUUID := machine.GenUUID(c)
	sourceWatcher := watchertest.NewMockStringsWatcher(attachmentChanged)

	s.mockMachineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.mockStorageProvisioningService.EXPECT().
		WatchVolumeAttachmentPlans(gomock.Any(), machineUUID).
		Return(sourceWatcher, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("66", nil)

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

	s.mockMachineService.EXPECT().
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

	machineUUID := machine.GenUUID(c)
	s.mockMachineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.mockStorageProvisioningService.EXPECT().
		WatchMachineProvisionedVolumeAttachments(gomock.Any(), machineUUID).
		Return(sourceWatcher, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("66", nil)

	s.mockStorageProvisioningService.EXPECT().
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

	s.mockMachineService.EXPECT().
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

	s.mockStorageProvisioningService.EXPECT().
		WatchModelProvisionedVolumeAttachments(gomock.Any()).
		Return(sourceWatcher, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("66", nil)

	s.mockStorageProvisioningService.EXPECT().
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

	machineUUID := machine.GenUUID(c)
	s.mockMachineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.mockStorageProvisioningService.EXPECT().
		WatchMachineProvisionedFilesystemAttachments(gomock.Any(), machineUUID).
		Return(sourceWatcher, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("66", nil)

	s.mockStorageProvisioningService.EXPECT().
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

	s.mockMachineService.EXPECT().
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

	s.mockStorageProvisioningService.EXPECT().
		WatchModelProvisionedFilesystemAttachments(gomock.Any()).
		Return(sourceWatcher, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("66", nil)

	s.mockStorageProvisioningService.EXPECT().
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

	s.mockStorageProvisioningService.EXPECT().GetVolumeUUIDForID(
		gomock.Any(), tag.Id(),
	).Return(volumeUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetVolumeLife(
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

	s.mockStorageProvisioningService.EXPECT().GetVolumeUUIDForID(
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

	s.mockStorageProvisioningService.EXPECT().GetVolumeUUIDForID(
		gomock.Any(), tag.Id(),
	).Return(volumeUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetVolumeLife(
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

	s.mockStorageProvisioningService.EXPECT().CheckFilesystemForIDExists(
		gomock.Any(), tag.Id(),
	).Return(true, nil)

	s.mockStorageProvisioningService.EXPECT().GetFilesystemUUIDForID(
		gomock.Any(), tag.Id(),
	).Return(filesystemUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetFilesystemLife(
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

	s.mockStorageProvisioningService.EXPECT().CheckFilesystemForIDExists(
		gomock.Any(), tag.Id(),
	).Return(true, nil)

	s.mockStorageProvisioningService.EXPECT().GetFilesystemUUIDForID(
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

	s.mockStorageProvisioningService.EXPECT().CheckFilesystemForIDExists(
		gomock.Any(), tag.Id(),
	).Return(true, nil)

	s.mockStorageProvisioningService.EXPECT().GetFilesystemUUIDForID(
		gomock.Any(), tag.Id(),
	).Return(filesystemUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetFilesystemLife(
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
	machineUUID := machine.GenUUID(c)
	filesystemAttachmentUUID := storageprovisioningtesting.GenFilesystemAttachmentUUID(c)

	s.mockMachineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetFilesystemAttachmentUUIDForFilesystemIDMachine(
		gomock.Any(), tag.Id(), machineUUID,
	).Return(filesystemAttachmentUUID, nil)
	s.mockStorageProvisioningService.EXPECT().
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

	s.mockMachineService.EXPECT().
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
	machineUUID := machine.GenUUID(c)

	s.mockMachineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetFilesystemAttachmentUUIDForFilesystemIDMachine(
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
	machineUUID := machine.GenUUID(c)

	s.mockMachineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetFilesystemAttachmentUUIDForFilesystemIDMachine(
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
	machineUUID := machine.GenUUID(c)
	filesystemAttachmentUUID := storageprovisioningtesting.GenFilesystemAttachmentUUID(c)

	s.mockMachineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetFilesystemAttachmentUUIDForFilesystemIDMachine(
		gomock.Any(), tag.Id(), machineUUID,
	).Return(filesystemAttachmentUUID, nil)
	s.mockStorageProvisioningService.EXPECT().
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
	machineUUID := machine.GenUUID(c)
	filesystemAttachmentUUID := storageprovisioningtesting.GenFilesystemAttachmentUUID(c)

	s.mockMachineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetFilesystemAttachmentUUIDForFilesystemIDMachine(
		gomock.Any(), tag.Id(), machineUUID,
	).Return(filesystemAttachmentUUID, nil)
	s.mockStorageProvisioningService.EXPECT().
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
	unitUUID := coreunit.GenUUID(c)
	filesystemAttachmentUUID := storageprovisioningtesting.GenFilesystemAttachmentUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetFilesystemAttachmentUUIDForFilesystemIDUnit(
		gomock.Any(), tag.Id(), unitUUID,
	).Return(filesystemAttachmentUUID, nil)
	s.mockStorageProvisioningService.EXPECT().
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

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return("", applicationerrors.UnitNotFound)

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
	unitUUID := coreunit.GenUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetFilesystemAttachmentUUIDForFilesystemIDUnit(
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
	unitUUID := coreunit.GenUUID(c)
	filesystemAttachmentUUID := storageprovisioningtesting.GenFilesystemAttachmentUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetFilesystemAttachmentUUIDForFilesystemIDUnit(
		gomock.Any(), tag.Id(), unitUUID,
	).Return(filesystemAttachmentUUID, nil)
	s.mockStorageProvisioningService.EXPECT().
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
	unitUUID := coreunit.GenUUID(c)
	filesystemAttachmentUUID := storageprovisioningtesting.GenFilesystemAttachmentUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetFilesystemAttachmentUUIDForFilesystemIDUnit(
		gomock.Any(), tag.Id(), unitUUID,
	).Return(filesystemAttachmentUUID, nil)
	s.mockStorageProvisioningService.EXPECT().
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
	unitUUID := coreunit.GenUUID(c)
	filesystemAttachmentUUID := storageprovisioningtesting.GenFilesystemAttachmentUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetFilesystemAttachmentUUIDForFilesystemIDUnit(
		gomock.Any(), tag.Id(), unitUUID,
	).Return(filesystemAttachmentUUID, nil)
	s.mockStorageProvisioningService.EXPECT().
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
	machineUUID := machine.GenUUID(c)
	volumeAttachmentUUID := storageprovisioningtesting.GenVolumeAttachmentUUID(c)

	s.mockMachineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetVolumeAttachmentUUIDForVolumeIDMachine(
		gomock.Any(), tag.Id(), machineUUID,
	).Return(volumeAttachmentUUID, nil)
	s.mockStorageProvisioningService.EXPECT().
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

	s.mockMachineService.EXPECT().
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
	machineUUID := machine.GenUUID(c)

	s.mockMachineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetVolumeAttachmentUUIDForVolumeIDMachine(
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
	machineUUID := machine.GenUUID(c)

	s.mockMachineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetVolumeAttachmentUUIDForVolumeIDMachine(
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
	machineUUID := machine.GenUUID(c)
	volumeAttachmentUUID := storageprovisioningtesting.GenVolumeAttachmentUUID(c)

	s.mockMachineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetVolumeAttachmentUUIDForVolumeIDMachine(
		gomock.Any(), tag.Id(), machineUUID,
	).Return(volumeAttachmentUUID, nil)
	s.mockStorageProvisioningService.EXPECT().
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
	machineUUID := machine.GenUUID(c)
	volumeAttachmentUUID := storageprovisioningtesting.GenVolumeAttachmentUUID(c)

	s.mockMachineService.EXPECT().
		GetMachineUUID(gomock.Any(), s.machineName).
		Return(machineUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetVolumeAttachmentUUIDForVolumeIDMachine(
		gomock.Any(), tag.Id(), machineUUID,
	).Return(volumeAttachmentUUID, nil)
	s.mockStorageProvisioningService.EXPECT().
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
	unitUUID := coreunit.GenUUID(c)
	volumeAttachmentUUID := storageprovisioningtesting.GenVolumeAttachmentUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetVolumeAttachmentUUIDForVolumeIDUnit(
		gomock.Any(), tag.Id(), unitUUID,
	).Return(volumeAttachmentUUID, nil)
	s.mockStorageProvisioningService.EXPECT().
		GetVolumeAttachmentLife(gomock.Any(), volumeAttachmentUUID).
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

func (s *provisionerSuite) TestAttachmentLifeForVolumeUnitWithUnitNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	unitTag := names.NewUnitTag("mysql/666")

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return("", applicationerrors.UnitNotFound)

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

func (s *provisionerSuite) TestAttachmentLifeForVolumeUnitWithVolumeAttachmentNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	unitTag := names.NewUnitTag("mysql/666")
	unitUUID := coreunit.GenUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetVolumeAttachmentUUIDForVolumeIDUnit(
		gomock.Any(), tag.Id(), unitUUID,
	).Return("", storageprovisioningerrors.VolumeAttachmentNotFound)

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

func (s *provisionerSuite) TestAttachmentLifeForVolumeUnitWithVolumeNotFound(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	unitTag := names.NewUnitTag("mysql/666")
	unitUUID := coreunit.GenUUID(c)
	volumeAttachmentUUID := storageprovisioningtesting.GenVolumeAttachmentUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetVolumeAttachmentUUIDForVolumeIDUnit(
		gomock.Any(), tag.Id(), unitUUID,
	).Return(volumeAttachmentUUID, nil)
	s.mockStorageProvisioningService.EXPECT().
		GetVolumeAttachmentLife(gomock.Any(), volumeAttachmentUUID).
		Return(-1, storageprovisioningerrors.VolumeAttachmentNotFound)

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

func (s *provisionerSuite) TestAttachmentLifeForVolumeUnitWithVolumeAttachmentNotFound2(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	unitTag := names.NewUnitTag("mysql/666")
	unitUUID := coreunit.GenUUID(c)
	volumeAttachmentUUID := storageprovisioningtesting.GenVolumeAttachmentUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetVolumeAttachmentUUIDForVolumeIDUnit(
		gomock.Any(), tag.Id(), unitUUID,
	).Return(volumeAttachmentUUID, nil)
	s.mockStorageProvisioningService.EXPECT().
		GetVolumeAttachmentLife(gomock.Any(), volumeAttachmentUUID).
		Return(-1, storageprovisioningerrors.VolumeAttachmentNotFound)

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

func (s *provisionerSuite) TestAttachmentLifeForVolumeUnitWithVolumeNotFound2(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	unitTag := names.NewUnitTag("mysql/666")
	unitUUID := coreunit.GenUUID(c)
	volumeAttachmentUUID := storageprovisioningtesting.GenVolumeAttachmentUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("mysql/666")).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetVolumeAttachmentUUIDForVolumeIDUnit(
		gomock.Any(), tag.Id(), unitUUID,
	).Return(volumeAttachmentUUID, nil)
	s.mockStorageProvisioningService.EXPECT().
		GetVolumeAttachmentLife(gomock.Any(), volumeAttachmentUUID).
		Return(-1, storageprovisioningerrors.VolumeAttachmentNotFound)

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

func (s *provisionerSuite) TestSetFilesystemInfo(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewFilesystemTag("123")

	info := storageprovisioning.FilesystemProvisionedInfo{
		ProviderID: "fs-123",
		SizeMiB:    100,
	}
	svc := s.mockStorageProvisioningService
	svc.EXPECT().CheckFilesystemForIDExists(gomock.Any(), "123").Return(true, nil)
	svc.EXPECT().SetFilesystemProvisionedInfo(gomock.Any(), "123", info).Return(nil)

	result, err := s.api.SetFilesystemInfo(c.Context(), params.Filesystems{
		Filesystems: []params.Filesystem{
			{
				FilesystemTag: tag.String(),
				Info: params.FilesystemInfo{
					ProviderId: "fs-123",
					Size:       100,
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
	svc := s.mockStorageProvisioningService
	svc.EXPECT().CheckFilesystemForIDExists(gomock.Any(), "123").Return(true, nil)
	svc.EXPECT().SetFilesystemProvisionedInfo(gomock.Any(), "123", info).Return(nil)

	result, err := s.api.SetFilesystemInfo(c.Context(), params.Filesystems{
		Filesystems: []params.Filesystem{
			{
				FilesystemTag: fsTag.String(),
				VolumeTag:     volTag.String(),
				Info: params.FilesystemInfo{
					ProviderId: "fs-123",
					Size:       100,
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

	svc := s.mockStorageProvisioningService
	svc.EXPECT().CheckFilesystemForIDExists(gomock.Any(), "123").Return(true, nil)
	svc.EXPECT().SetFilesystemProvisionedInfo(gomock.Any(), "123", info).Return(
		storageprovisioningerrors.FilesystemNotFound)

	result, err := s.api.SetFilesystemInfo(c.Context(), params.Filesystems{
		Filesystems: []params.Filesystem{
			{
				FilesystemTag: tag.String(),
				Info: params.FilesystemInfo{
					ProviderId: "fs-123",
					Size:       100,
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

	svc := s.mockStorageProvisioningService
	svc.EXPECT().CheckFilesystemForIDExists(gomock.Any(), "123").Return(true, nil)

	result, err := s.api.SetFilesystemInfo(c.Context(), params.Filesystems{
		Filesystems: []params.Filesystem{
			{
				FilesystemTag: tag.String(),
				Info: params.FilesystemInfo{
					ProviderId: "fs-123",
					Size:       100,
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
	svc := s.mockStorageProvisioningService
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
	svc := s.mockStorageProvisioningService
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
	machineUUID := machine.GenUUID(c)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(),
		machine.Name(machineTag.Id())).Return(machineUUID, nil)
	volAttachUUID := storageprovisioningtesting.GenVolumeAttachmentUUID(c)
	info := storageprovisioning.VolumeAttachmentProvisionedInfo{
		ReadOnly:              true,
		BlockDeviceName:       "x",
		BlockDeviceLink:       "y",
		BlockDeviceBusAddress: "z",
	}
	planInfo := storageprovisioning.VolumeAttachmentPlanProvisionedInfo{
		DeviceType: "iscsi",
		DeviceAttributes: map[string]string{
			"a": "b",
		},
	}

	svc := s.mockStorageProvisioningService
	svc.EXPECT().GetVolumeAttachmentUUIDForVolumeIDMachine(gomock.Any(),
		tag.Id(), machineUUID).Return(volAttachUUID, nil)
	svc.EXPECT().SetVolumeAttachmentProvisionedInfo(gomock.Any(),
		volAttachUUID, info).Return(nil)
	svc.EXPECT().SetVolumeAttachmentPlanProvisionedInfo(gomock.Any(), tag.Id(),
		machineUUID, planInfo).Return(nil)

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

func (s *provisionerSuite) TestSetVolumeAttachmentPlanBlockInfo(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	tag := names.NewVolumeTag("123")
	machineTag := names.NewMachineTag("5")
	machineUUID := machine.GenUUID(c)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(),
		machine.Name(machineTag.Id())).Return(machineUUID, nil)

	blockDeviceInfo := blockdevice.BlockDevice{
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
	}
	svc := s.mockStorageProvisioningService
	svc.EXPECT().SetVolumeAttachmentPlanProvisionedBlockDevice(gomock.Any(), tag.Id(), machineUUID,
		blockDeviceInfo).Return(nil)

	result, err := s.api.SetVolumeAttachmentPlanBlockInfo(c.Context(), params.VolumeAttachmentPlans{
		VolumeAttachmentPlans: []params.VolumeAttachmentPlan{
			{
				VolumeTag:  tag.String(),
				MachineTag: machineTag.String(),
				BlockDevice: params.BlockDevice{
					DeviceName:     "a",
					DeviceLinks:    []string{"b"},
					Label:          "c",
					UUID:           "d",
					HardwareId:     "e",
					Size:           0xf,
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
	machineUUID := machine.GenUUID(c)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(),
		machine.Name(machineTag.Id())).Return(machineUUID, nil)

	info := storageprovisioning.FilesystemAttachmentProvisionedInfo{
		MountPoint: "x",
		ReadOnly:   true,
	}
	svc := s.mockStorageProvisioningService
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
	machineUUID := machine.GenUUID(c)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(),
		machine.Name(machineTag.Id())).Return(machineUUID, nil)

	info := storageprovisioning.FilesystemAttachmentProvisionedInfo{
		MountPoint: "x",
		ReadOnly:   true,
	}
	svc := s.mockStorageProvisioningService
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
	unitUUID := coreunit.GenUUID(c)
	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(),
		coreunit.Name(unitTag.Id())).Return(unitUUID, nil)

	info := storageprovisioning.FilesystemAttachmentProvisionedInfo{
		MountPoint: "x",
		ReadOnly:   true,
	}
	svc := s.mockStorageProvisioningService
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
	unitUUID := coreunit.GenUUID(c)
	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(),
		coreunit.Name(unitTag.Id())).Return(unitUUID, nil)

	info := storageprovisioning.FilesystemAttachmentProvisionedInfo{
		MountPoint: "x",
		ReadOnly:   true,
	}
	svc := s.mockStorageProvisioningService
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
