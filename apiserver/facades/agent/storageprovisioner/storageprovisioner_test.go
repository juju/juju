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
	"github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
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
		Size:         1000,
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
	machineUUID := machinetesting.GenUUID(c)

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
	machineUUID := machinetesting.GenUUID(c)

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
	machineUUID := machinetesting.GenUUID(c)

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
	machineUUID := machinetesting.GenUUID(c)

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
	unitUUID := unittesting.GenUnitUUID(c)

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
	unitUUID := unittesting.GenUnitUUID(c)

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
	unitUUID := unittesting.GenUnitUUID(c)

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
	unitUUID := unittesting.GenUnitUUID(c)

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
	machineUUID := machinetesting.GenUUID(c)

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
	machineUUID := machinetesting.GenUUID(c)

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
	machineUUID := machinetesting.GenUUID(c)
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

	machineUUID := machinetesting.GenUUID(c)
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

	machineUUID := machinetesting.GenUUID(c)
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
