// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	corelife "github.com/juju/juju/core/life"
	machine "github.com/juju/juju/core/machine"
	corestatus "github.com/juju/juju/core/status"
	unit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	storagetesting "github.com/juju/juju/domain/storage/testing"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningtesting "github.com/juju/juju/domain/storageprovisioning/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/statushistory"
)

type storageServiceSuite struct {
	modelState      *MockModelState
	controllerState *MockControllerState
	statusHistory   *statusHistoryRecorder

	service *Service
}

func TestStorageServiceSuite(t *testing.T) {
	tc.Run(t, &storageServiceSuite{})
}

func (s *storageServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelState = NewMockModelState(ctrl)
	s.controllerState = NewMockControllerState(ctrl)
	s.statusHistory = &statusHistoryRecorder{}

	s.service = NewService(
		s.modelState,
		s.controllerState,
		s.statusHistory,
		func() (StatusHistoryReader, error) {
			return nil, errors.Errorf("status history reader not available")
		},
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

	c.Cleanup(func() {
		s.modelState = nil
		s.controllerState = nil
		s.statusHistory = nil
		s.service = nil
	})

	return ctrl
}

func (s *storageServiceSuite) TestSetFilesystemStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	filesystemUUID := storageprovisioningtesting.GenFilesystemUUID(c)
	s.modelState.EXPECT().GetFilesystemUUIDByID(gomock.Any(), "666").Return(filesystemUUID, nil)
	s.modelState.EXPECT().SetFilesystemStatus(gomock.Any(), filesystemUUID, status.StatusInfo[status.StorageFilesystemStatusType]{
		Status:  status.StorageFilesystemStatusTypeAttached,
		Message: "doink",
		Data:    []byte(`{"foo":"bar"}`),
		Since:   &now,
	})

	err := s.service.SetFilesystemStatus(c.Context(), "666", corestatus.StatusInfo{
		Status:  corestatus.Attached,
		Message: "doink",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.statusHistory.records, tc.DeepEquals, []statusHistoryRecord{{
		ns: statushistory.Namespace{Kind: corestatus.KindFilesystem, ID: filesystemUUID.String()},
		s: corestatus.StatusInfo{
			Status:  corestatus.Attached,
			Message: "doink",
			Data:    map[string]any{"foo": "bar"},
			Since:   &now,
		},
	}})
}

func (s *storageServiceSuite) TestSetFilesystemStatusUUIDNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetFilesystemUUIDByID(gomock.Any(), "666").Return("", storageerrors.FilesystemNotFound)

	err := s.service.SetFilesystemStatus(c.Context(), "666", corestatus.StatusInfo{
		Status: corestatus.Attached,
	})
	c.Assert(err, tc.ErrorIs, storageerrors.FilesystemNotFound)
}

func (s *storageServiceSuite) TestSetFilesystemStatusNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	filesystemUUID := storageprovisioningtesting.GenFilesystemUUID(c)
	s.modelState.EXPECT().GetFilesystemUUIDByID(gomock.Any(), "666").Return(filesystemUUID, nil)
	s.modelState.EXPECT().SetFilesystemStatus(gomock.Any(), filesystemUUID, status.StatusInfo[status.StorageFilesystemStatusType]{
		Status: status.StorageFilesystemStatusTypeAttached,
	}).Return(storageerrors.FilesystemNotFound)

	err := s.service.SetFilesystemStatus(c.Context(), "666", corestatus.StatusInfo{
		Status: corestatus.Attached,
	})
	c.Assert(err, tc.ErrorIs, storageerrors.FilesystemNotFound)
}

func (s *storageServiceSuite) TestSetFilesystemStatusInvalidStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.SetFilesystemStatus(c.Context(), "666", corestatus.StatusInfo{
		Status: "invalid",
	})
	c.Assert(err, tc.ErrorMatches, `.*unknown filesystem status "invalid"`)

	err = s.service.SetFilesystemStatus(c.Context(), "666", corestatus.StatusInfo{
		Status: corestatus.Allocating,
	})
	c.Assert(err, tc.ErrorMatches, `.*unknown filesystem status "allocating"`)
}

func (s *storageServiceSuite) TestFilesystemStatusTransitionErrorInvalid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.SetFilesystemStatus(c.Context(), "666", corestatus.StatusInfo{
		Status: "error",
	})
	c.Assert(err, tc.ErrorMatches, `cannot set status .* without message`)
}

func (s *storageServiceSuite) TestSetVolumeStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	volumeUUID := storageprovisioningtesting.GenVolumeUUID(c)
	s.modelState.EXPECT().GetVolumeUUIDByID(gomock.Any(), "666").Return(volumeUUID, nil)
	s.modelState.EXPECT().SetVolumeStatus(gomock.Any(), volumeUUID, status.StatusInfo[status.StorageVolumeStatusType]{
		Status:  status.StorageVolumeStatusTypeAttached,
		Message: "doink",
		Data:    []byte(`{"foo":"bar"}`),
		Since:   &now,
	})

	err := s.service.SetVolumeStatus(c.Context(), "666", corestatus.StatusInfo{
		Status:  corestatus.Attached,
		Message: "doink",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.statusHistory.records, tc.DeepEquals, []statusHistoryRecord{{
		ns: statushistory.Namespace{Kind: corestatus.KindVolume, ID: volumeUUID.String()},
		s: corestatus.StatusInfo{
			Status:  corestatus.Attached,
			Message: "doink",
			Data:    map[string]any{"foo": "bar"},
			Since:   &now,
		},
	}})
}

func (s *storageServiceSuite) TestSetVolumeStatusUUIDNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetVolumeUUIDByID(gomock.Any(), "666").Return("", storageerrors.VolumeNotFound)

	err := s.service.SetVolumeStatus(c.Context(), "666", corestatus.StatusInfo{
		Status: corestatus.Attached,
	})
	c.Assert(err, tc.ErrorIs, storageerrors.VolumeNotFound)
}

func (s *storageServiceSuite) TestSetVolumeStatusNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	volumeUUID := storageprovisioningtesting.GenVolumeUUID(c)
	s.modelState.EXPECT().GetVolumeUUIDByID(gomock.Any(), "666").Return(volumeUUID, nil)
	s.modelState.EXPECT().SetVolumeStatus(gomock.Any(), volumeUUID, status.StatusInfo[status.StorageVolumeStatusType]{
		Status: status.StorageVolumeStatusTypeAttached,
	}).Return(storageerrors.VolumeNotFound)

	err := s.service.SetVolumeStatus(c.Context(), "666", corestatus.StatusInfo{
		Status: corestatus.Attached,
	})
	c.Assert(err, tc.ErrorIs, storageerrors.VolumeNotFound)
}

func (s *storageServiceSuite) TestSetVolumeStatusInvalidStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.SetVolumeStatus(c.Context(), "666", corestatus.StatusInfo{
		Status: "invalid",
	})
	c.Assert(err, tc.ErrorMatches, `.*unknown volume status "invalid"`)

	err = s.service.SetVolumeStatus(c.Context(), "666", corestatus.StatusInfo{
		Status: corestatus.Allocating,
	})
	c.Assert(err, tc.ErrorMatches, `.*unknown volume status "allocating"`)
}

func (s *storageServiceSuite) TestVolumeStatusTransitionErrorInvalid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.SetVolumeStatus(c.Context(), "666", corestatus.StatusInfo{
		Status: "error",
	})
	c.Assert(err, tc.ErrorMatches, `cannot set status .* without message`)
}

func (s *storageServiceSuite) TestGetStorageInstanceStatuses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageInstanceUUID := storagetesting.GenStorageInstanceUUID(c)
	si := []status.StorageInstance{
		{
			UUID:  storageInstanceUUID,
			ID:    "12",
			Owner: ptr(unit.Name("foo/10")),
			Kind:  storage.StorageKindBlock,
			Life:  life.Alive,
		},
	}
	s.modelState.EXPECT().GetStorageInstances(gomock.Any()).Return(si, nil)
	sa := []status.StorageAttachment{
		{
			StorageInstanceUUID: storageInstanceUUID,
			Life:                life.Alive,
			Unit:                unit.Name("foo/10"),
			Machine:             ptr(machine.Name("5")),
		},
	}
	s.modelState.EXPECT().GetStorageInstanceAttachments(gomock.Any()).Return(sa, nil)

	res, err := s.service.GetStorageInstanceStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, []StorageInstance{
		{
			ID:    "12",
			Owner: ptr(unit.Name("foo/10")),
			Kind:  storage.StorageKindBlock,
			Life:  corelife.Alive,
			Attachments: map[unit.Name]StorageAttachment{
				"foo/10": {
					Life:    corelife.Alive,
					Unit:    "foo/10",
					Machine: ptr(machine.Name("5")),
				},
			},
		},
	})
}

func (s *storageServiceSuite) TestGetStorageInstanceStatusesMultiple(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageInstanceUUID0 := storagetesting.GenStorageInstanceUUID(c)
	storageInstanceUUID1 := storagetesting.GenStorageInstanceUUID(c)
	si := []status.StorageInstance{
		{
			UUID: storageInstanceUUID0,
			ID:   "0",
			Life: life.Alive,
		},
		{
			UUID: storageInstanceUUID1,
			ID:   "1",
			Life: life.Dying,
		},
	}
	s.modelState.EXPECT().GetStorageInstances(gomock.Any()).Return(si, nil)
	sa := []status.StorageAttachment{
		{
			StorageInstanceUUID: storageInstanceUUID0,
			Unit:                unit.Name("foo/0"),
			Machine:             ptr(machine.Name("0")),
			Life:                life.Alive,
		},
		{
			StorageInstanceUUID: storageInstanceUUID1,
			Unit:                unit.Name("foo/1"),
			Machine:             ptr(machine.Name("1")),
			Life:                life.Dying,
		},
		{
			StorageInstanceUUID: storageInstanceUUID1,
			Unit:                unit.Name("bar/0"),
			Machine:             ptr(machine.Name("1")),
			Life:                life.Dying,
		},
	}
	s.modelState.EXPECT().GetStorageInstanceAttachments(gomock.Any()).Return(sa, nil)

	res, err := s.service.GetStorageInstanceStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.UnorderedMatch[[]StorageInstance](tc.DeepEquals), []StorageInstance{
		{
			ID:   "0",
			Life: corelife.Alive,
			Attachments: map[unit.Name]StorageAttachment{
				"foo/0": {
					Unit:    "foo/0",
					Machine: ptr(machine.Name("0")),
					Life:    corelife.Alive,
				},
			},
		},
		{
			ID:   "1",
			Life: corelife.Dying,
			Attachments: map[unit.Name]StorageAttachment{
				"foo/1": {
					Unit:    "foo/1",
					Machine: ptr(machine.Name("1")),
					Life:    corelife.Dying,
				},
				"bar/0": {
					Unit:    "bar/0",
					Machine: ptr(machine.Name("1")),
					Life:    corelife.Dying,
				},
			},
		},
	})
}

func (s *storageServiceSuite) TestGetFilesystemStatuses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	fsUUID := storageprovisioningtesting.GenFilesystemUUID(c)
	fs := []status.Filesystem{
		{
			UUID: fsUUID,
			ID:   "1",
			Life: life.Alive,
			Status: status.StatusInfo[status.StorageFilesystemStatusType]{
				Status:  status.StorageFilesystemStatusTypeAttaching,
				Message: "attaching to the thing",
			},
			StorageID:  "data/0",
			VolumeID:   ptr("9"),
			ProviderID: "provider-foo-0",
			SizeMiB:    123,
		},
	}
	s.modelState.EXPECT().GetFilesystems(gomock.Any()).Return(fs, nil)
	fa := []status.FilesystemAttachment{
		{
			FilesystemUUID: fsUUID,
			Life:           life.Alive,
			Unit:           ptr(unit.Name("foo/0")),
			Machine:        ptr(machine.Name("0")),
			MountPoint:     "/foo/bar",
			ReadOnly:       true,
		},
	}
	s.modelState.EXPECT().GetFilesystemAttachments(gomock.Any()).Return(fa, nil)

	res, err := s.service.GetFilesystemStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, []Filesystem{
		{
			ID:   "1",
			Life: corelife.Alive,
			Status: corestatus.StatusInfo{
				Status:  corestatus.Attaching,
				Message: "attaching to the thing",
			},
			StorageID:  "data/0",
			VolumeID:   ptr("9"),
			ProviderID: "provider-foo-0",
			SizeMiB:    123,
			MachineAttachments: map[machine.Name]FilesystemAttachment{
				"0": {
					Life:       corelife.Alive,
					MountPoint: "/foo/bar",
					ReadOnly:   true,
				},
			},
			UnitAttachments: map[unit.Name]FilesystemAttachment{
				"foo/0": {
					Life:       corelife.Alive,
					MountPoint: "/foo/bar",
					ReadOnly:   true,
				},
			},
		},
	})
}

func (s *storageServiceSuite) TestGetFilesystemStatusesMultiple(c *tc.C) {
	defer s.setupMocks(c).Finish()

	fsUUID0 := storageprovisioningtesting.GenFilesystemUUID(c)
	fsUUID1 := storageprovisioningtesting.GenFilesystemUUID(c)
	fs := []status.Filesystem{
		{
			UUID: fsUUID0,
			ID:   "1",
			Life: life.Alive,
			Status: status.StatusInfo[status.StorageFilesystemStatusType]{
				Status:  status.StorageFilesystemStatusTypeAttaching,
				Message: "attaching to the thing",
			},
			StorageID:  "data/0",
			VolumeID:   ptr("9"),
			ProviderID: "provider-foo-0",
			SizeMiB:    123,
		},
		{
			UUID: fsUUID1,
			ID:   "3",
			Life: life.Alive,
			Status: status.StatusInfo[status.StorageFilesystemStatusType]{
				Status: status.StorageFilesystemStatusTypeAttached,
			},
			StorageID:  "data/4",
			ProviderID: "provider-foo-9",
			SizeMiB:    456,
		},
	}
	s.modelState.EXPECT().GetFilesystems(gomock.Any()).Return(fs, nil)
	fa := []status.FilesystemAttachment{
		{
			FilesystemUUID: fsUUID0,
			Life:           life.Alive,
			Unit:           ptr(unit.Name("foo/0")),
			Machine:        ptr(machine.Name("0")),
			MountPoint:     "/foo/bar",
			ReadOnly:       true,
		},
		{
			FilesystemUUID: fsUUID1,
			Life:           life.Alive,
			Unit:           ptr(unit.Name("foo/3")),
			Machine:        ptr(machine.Name("3")),
			MountPoint:     "/baz/x",
			ReadOnly:       true,
		},
		{
			FilesystemUUID: fsUUID1,
			Life:           life.Dying,
			Unit:           ptr(unit.Name("bar/8")),
			MountPoint:     "/baz/y",
			ReadOnly:       false,
		},
	}
	s.modelState.EXPECT().GetFilesystemAttachments(gomock.Any()).Return(fa, nil)

	res, err := s.service.GetFilesystemStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.UnorderedMatch[[]Filesystem](tc.DeepEquals), []Filesystem{
		{
			ID:   "1",
			Life: corelife.Alive,
			Status: corestatus.StatusInfo{
				Status:  corestatus.Attaching,
				Message: "attaching to the thing",
			},
			StorageID:  "data/0",
			VolumeID:   ptr("9"),
			ProviderID: "provider-foo-0",
			SizeMiB:    123,
			MachineAttachments: map[machine.Name]FilesystemAttachment{
				"0": {
					Life:       corelife.Alive,
					MountPoint: "/foo/bar",
					ReadOnly:   true,
				},
			},
			UnitAttachments: map[unit.Name]FilesystemAttachment{
				"foo/0": {
					Life:       corelife.Alive,
					MountPoint: "/foo/bar",
					ReadOnly:   true,
				},
			},
		},
		{
			ID:   "3",
			Life: corelife.Alive,
			Status: corestatus.StatusInfo{
				Status: corestatus.Attached,
			},
			StorageID:  "data/4",
			ProviderID: "provider-foo-9",
			SizeMiB:    456,
			MachineAttachments: map[machine.Name]FilesystemAttachment{
				"3": {
					Life:       corelife.Alive,
					MountPoint: "/baz/x",
					ReadOnly:   true,
				},
			},
			UnitAttachments: map[unit.Name]FilesystemAttachment{
				"foo/3": {
					Life:       corelife.Alive,
					MountPoint: "/baz/x",
					ReadOnly:   true,
				},
				"bar/8": {
					Life:       corelife.Dying,
					MountPoint: "/baz/y",
				},
			},
		},
	})
}

func (s *storageServiceSuite) TestGetVolumeStatuses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	volUUID := storageprovisioningtesting.GenVolumeUUID(c)
	vol := []status.Volume{
		{
			UUID: volUUID,
			ID:   "1",
			Life: life.Alive,
			Status: status.StatusInfo[status.StorageVolumeStatusType]{
				Status:  status.StorageVolumeStatusTypeAttaching,
				Message: "attaching to the thing",
			},
			StorageID:  "data/0",
			ProviderID: "provider-foo-0",
			SizeMiB:    123,
			HardwareID: "hw0",
			WWN:        "wwn0",
			Persistent: true,
		},
	}
	s.modelState.EXPECT().GetVolumes(gomock.Any()).Return(vol, nil)
	va := []status.VolumeAttachment{
		{
			VolumeUUID: volUUID,
			Life:       life.Alive,
			Unit:       ptr(unit.Name("foo/0")),
			Machine:    ptr(machine.Name("0")),
			ReadOnly:   true,
			DeviceName: "dvname0",
			DeviceLink: "/dev/link0",
			BusAddress: "bus-addr0",
			VolumeAttachmentPlan: &status.VolumeAttachmentPlan{
				DeviceType: storageprovisioning.PlanDeviceTypeISCSI,
				DeviceAttributes: map[string]string{
					"foo": "bar",
				},
			},
		},
	}
	s.modelState.EXPECT().GetVolumeAttachments(gomock.Any()).Return(va, nil)

	res, err := s.service.GetVolumeStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, []Volume{
		{
			ID:   "1",
			Life: corelife.Alive,
			Status: corestatus.StatusInfo{
				Status:  corestatus.Attaching,
				Message: "attaching to the thing",
			},
			StorageID:  "data/0",
			ProviderID: "provider-foo-0",
			SizeMiB:    123,
			HardwareID: "hw0",
			WWN:        "wwn0",
			Persistent: true,
			MachineAttachments: map[machine.Name]VolumeAttachment{
				"0": {
					Life:       corelife.Alive,
					ReadOnly:   true,
					DeviceName: "dvname0",
					DeviceLink: "/dev/link0",
					BusAddress: "bus-addr0",
					VolumeAttachmentPlan: &VolumeAttachmentPlan{
						DeviceType: storageprovisioning.PlanDeviceTypeISCSI,
						DeviceAttributes: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
			UnitAttachments: map[unit.Name]VolumeAttachment{
				"foo/0": {
					Life:       corelife.Alive,
					ReadOnly:   true,
					DeviceName: "dvname0",
					DeviceLink: "/dev/link0",
					BusAddress: "bus-addr0",
					VolumeAttachmentPlan: &VolumeAttachmentPlan{
						DeviceType: storageprovisioning.PlanDeviceTypeISCSI,
						DeviceAttributes: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
		},
	})
}
