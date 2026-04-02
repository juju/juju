// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	domainlife "github.com/juju/juju/domain/life"
	domainstatus "github.com/juju/juju/domain/status"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/domain/storage/internal"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

// instanceSuite is a test suite for asserting the parts of the [Service]
// interface that relate to storage instances.
type instanceSuite struct {
	state                 *MockState
	storageRegistryGetter *MockModelStorageRegistryGetter
}

// TestInstanceSuite runs all of the tests contained within [instanceSuite].
func TestInstanceSuite(t *testing.T) {
	tc.Run(t, &instanceSuite{})
}

func (s *instanceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.storageRegistryGetter = NewMockModelStorageRegistryGetter(ctrl)

	c.Cleanup(func() {
		s.state = nil
		s.storageRegistryGetter = nil
	})
	return ctrl
}

// TestGetStorageInstanceUUIDForIDNotFound tests getting a storage instance
// uuid for a storage id and then when the storage id does not exist in the
// model the caller gets back a error satisfying
// [domainstorageerrors.StorageInstanceNotFound].
func (s *instanceSuite) TestGetStorageInstanceUUIDForIDNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stateExp := s.state.EXPECT()
	stateExp.GetStorageInstanceUUIDByID(gomock.Any(), "id1").Return(
		"", domainstorageerrors.StorageInstanceNotFound,
	)

	svc := NewService(
		s.state, loggertesting.WrapCheckLog(c), clock.WallClock, s.storageRegistryGetter,
	)
	_, err := svc.GetStorageInstanceUUIDForID(c.Context(), "id1")
	c.Check(err, tc.ErrorIs, domainstorageerrors.StorageInstanceNotFound)
}

// TestGetStorageInstanceUUIDForID is a happy path test for
// [Service.GetStorageInstanceUUIDForID].
func (s *instanceSuite) TestGetStorageInstanceUUIDForID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	stateExp := s.state.EXPECT()
	stateExp.GetStorageInstanceUUIDByID(gomock.Any(), "id1").Return(
		storageInstanceUUID, nil,
	)

	svc := NewService(
		s.state, loggertesting.WrapCheckLog(c), clock.WallClock, s.storageRegistryGetter,
	)
	uuid, err := svc.GetStorageInstanceUUIDForID(c.Context(), "id1")
	c.Check(err, tc.ErrorIsNil)
	c.Check(uuid, tc.Equals, storageInstanceUUID)
}

// TestGetStorageInstanceInfoNotFound asserts that when calling
// [Service.GetStorageInstanceInfo] with a UUID that does not exist, the caller
// gets back an error satisfying [domainstorageerrors.StorageInstanceNotFound].
func (s *instanceSuite) TestGetStorageInstanceInfoNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	notFoundUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)

	stateExp := s.state.EXPECT()
	stateExp.GetStorageInstanceInfo(c.Context(), notFoundUUID).Return(
		internal.StorageInstanceInfo{},
		domainstorageerrors.StorageInstanceNotFound,
	)

	svc := NewService(
		s.state, loggertesting.WrapCheckLog(c), clock.WallClock, s.storageRegistryGetter,
	)
	_, err := svc.GetStorageInstanceInfo(c.Context(), notFoundUUID)
	c.Check(err, tc.ErrorIs, domainstorageerrors.StorageInstanceNotFound)
}

// TestGetStorageInstanceInfoUUIDNotValid tests that
// [Service.GetStorageInstanceInfo] returns an error satisfying
// [coreerrors.NotValid] when called with an invalid storage instance UUID. The
// test verifies that UUID validation occurs before any state operations.
func (s *instanceSuite) TestGetStorageInstanceInfoUUIDNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	invalidUUID := domainstorage.StorageInstanceUUID("invalid")

	svc := NewService(
		s.state, loggertesting.WrapCheckLog(c), clock.WallClock, s.storageRegistryGetter,
	)
	_, err := svc.GetStorageInstanceInfo(c.Context(), invalidUUID)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestGetStorageInstanceInfoFilesystem tests that
// [Service.GetStorageInstanceInfo] correctly returns and transforms storage
// instance information for a filesystem-backed storage instance with an
// attachment. The test verifies that the service properly converts internal
// state representation to the public domain types, including filesystem status,
// mount point location, and unit attachment details.
func (s *instanceSuite) TestGetStorageInstanceInfoFilesystem(c *tc.C) {
	defer s.setupMocks(c).Finish()
	siUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	saUUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	fsUUID := tc.Must(c, domainstorage.NewFilesystemUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	statusTime := time.Now()

	stateExp := s.state.EXPECT()
	stateExp.GetStorageInstanceInfo(c.Context(), siUUID).Return(
		internal.StorageInstanceInfo{
			Attachments: []internal.StorageInstanceInfoAttachment{
				{
					Filesystem: &internal.StorageInstanceInfoAttachmentFilesystem{
						MountPoint: "/mnt/fs1",
					},
					Life:     domainlife.Alive,
					UnitName: "foo/1",
					UnitUUID: unitUUID,
					UUID:     saUUID,
				},
			},
			Filesystem: &internal.StorageInstanceInfoFilesystem{
				Status: &internal.StorageInstanceInfoFilesystemStatus{
					Message:   "filesystem status",
					Status:    domainstatus.StorageFilesystemStatusTypeAttached,
					UpdatedAt: &statusTime,
				},
				UUID: fsUUID,
			},
			Life:      domainlife.Alive,
			Kind:      domainstorage.StorageKindFilesystem,
			StorageID: "123",
			UnitOwner: &internal.StorageInstanceInfoUnitOwner{
				Name: "foo/1",
				UUID: unitUUID,
			},
			UUID: siUUID,
		}, nil,
	)

	svc := NewService(
		s.state, loggertesting.WrapCheckLog(c), clock.WallClock, s.storageRegistryGetter,
	)
	res, err := svc.GetStorageInstanceInfo(c.Context(), siUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, domainstorage.StorageInstanceInfo{
		FilesystemStatus: &domainstorage.StorageInstanceFilesystemStatus{
			Message: "filesystem status",
			Status:  corestatus.Attached,
			Since:   &statusTime,
			UUID:    fsUUID,
		},
		ID:         "123",
		Life:       domainlife.Alive,
		Kind:       domainstorage.StorageKindFilesystem,
		Persistent: false,
		UnitAttachments: []domainstorage.StorageInstanceUnitAttachment{
			{
				Life:     domainlife.Alive,
				Location: "/mnt/fs1",
				UnitName: "foo/1",
				UnitUUID: unitUUID,
				UUID:     saUUID,
			},
		},
		UnitOwner: &domainstorage.StorageInstanceUnitOwner{
			Name: "foo/1",
			UUID: unitUUID,
		},
		UUID: siUUID,
	})
}

// TestGetStorageInstanceInfoFilesystemVolumeBacked tests that
// [Service.GetStorageInstanceInfo] correctly returns and transforms storage
// instance information for a filesystem-backed storage instance that is also
// backed by a volume. The test verifies that the service properly converts both
// filesystem and volume status information, including device links and machine
// attachment details.
//
// We want to see that the location chosen by the service layer is not the
// volume device links but instead the mount point of the filesystem attachment.
func (s *instanceSuite) TestGetStorageInstanceInfoFilesystemVolumeBacked(c *tc.C) {
	defer s.setupMocks(c).Finish()
	siUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	saUUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	fsUUID := tc.Must(c, domainstorage.NewFilesystemUUID)
	vUUID := tc.Must(c, domainstorage.NewVolumeUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	machineUUID := tc.Must(c, coremachine.NewUUID)
	statusTime := time.Now()

	stateExp := s.state.EXPECT()
	stateExp.GetStorageInstanceInfo(c.Context(), siUUID).Return(
		internal.StorageInstanceInfo{
			Attachments: []internal.StorageInstanceInfoAttachment{
				{
					Filesystem: &internal.StorageInstanceInfoAttachmentFilesystem{
						MountPoint: "/mnt/fs1",
					},
					Life: domainlife.Alive,
					Machine: &internal.StorageInstanceInfoAttachmentMachine{
						Name: "machine-0",
						UUID: machineUUID,
					},
					Volume: &internal.StorageInstanceInfoAttachmentVolume{
						DeviceNameLinks: []string{"/dev/disk/by-id/1", "/dev/disk/123"},
					},
					UnitName: "foo/1",
					UnitUUID: unitUUID,
					UUID:     saUUID,
				},
			},
			Filesystem: &internal.StorageInstanceInfoFilesystem{
				Status: &internal.StorageInstanceInfoFilesystemStatus{
					Message:   "filesystem status",
					Status:    domainstatus.StorageFilesystemStatusTypeAttached,
					UpdatedAt: &statusTime,
				},
				UUID: fsUUID,
			},
			Life:      domainlife.Alive,
			Kind:      domainstorage.StorageKindFilesystem,
			StorageID: "123",
			UnitOwner: &internal.StorageInstanceInfoUnitOwner{
				Name: "foo/1",
				UUID: unitUUID,
			},
			UUID: siUUID,
			Volume: &internal.StorageInstanceInfoVolume{
				Status: &internal.StorageInstanceInfoVolumeStatus{
					Message:   "volume status",
					Status:    domainstatus.StorageVolumeStatusTypeAttached,
					UpdatedAt: &statusTime,
				},
				UUID: vUUID,
			},
		}, nil,
	)

	svc := NewService(
		s.state, loggertesting.WrapCheckLog(c), clock.WallClock, s.storageRegistryGetter,
	)
	res, err := svc.GetStorageInstanceInfo(c.Context(), siUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, domainstorage.StorageInstanceInfo{
		FilesystemStatus: &domainstorage.StorageInstanceFilesystemStatus{
			Message: "filesystem status",
			Status:  corestatus.Attached,
			Since:   &statusTime,
			UUID:    fsUUID,
		},
		ID:         "123",
		Life:       domainlife.Alive,
		Kind:       domainstorage.StorageKindFilesystem,
		Persistent: false,
		UnitAttachments: []domainstorage.StorageInstanceUnitAttachment{
			{
				Life:     domainlife.Alive,
				Location: "/mnt/fs1",
				MachineAttachment: &domainstorage.StorageInstanceMachineAttachment{
					MachineName: "machine-0",
					MachineUUID: machineUUID,
				},
				UnitName: "foo/1",
				UnitUUID: unitUUID,
				UUID:     saUUID,
			},
		},
		UnitOwner: &domainstorage.StorageInstanceUnitOwner{
			Name: "foo/1",
			UUID: unitUUID,
		},
		UUID: siUUID,
		VolumeStatus: &domainstorage.StorageInstanceVolumeStatus{
			Message: "volume status",
			Status:  corestatus.Attached,
			Since:   &statusTime,
			UUID:    vUUID,
		},
	})
}

// TestGetStorageInstanceInfoFilesystemMultipleUnits tests that
// [Service.GetStorageInstanceInfo] correctly returns and transforms storage
// instance information for a filesystem-backed storage instance that is
// attached to multiple units. The test verifies that the service properly
// handles multiple unit attachments and returns all attachment details.
func (s *instanceSuite) TestGetStorageInstanceInfoFilesystemMultipleUnits(c *tc.C) {
	defer s.setupMocks(c).Finish()
	siUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	saUUID1 := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	saUUID2 := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	saUUID3 := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	fsUUID := tc.Must(c, domainstorage.NewFilesystemUUID)
	unitUUID1 := tc.Must(c, coreunit.NewUUID)
	unitUUID2 := tc.Must(c, coreunit.NewUUID)
	unitUUID3 := tc.Must(c, coreunit.NewUUID)
	statusTime := time.Now()

	stateExp := s.state.EXPECT()
	stateExp.GetStorageInstanceInfo(c.Context(), siUUID).Return(
		internal.StorageInstanceInfo{
			Attachments: []internal.StorageInstanceInfoAttachment{
				{
					Filesystem: &internal.StorageInstanceInfoAttachmentFilesystem{
						MountPoint: "/mnt/fs-mount-1",
					},
					Life:     domainlife.Alive,
					UnitName: "foo/0",
					UnitUUID: unitUUID1,
					UUID:     saUUID1,
				},
				{
					Filesystem: &internal.StorageInstanceInfoAttachmentFilesystem{
						MountPoint: "/mnt/fs-mount-2",
					},
					Life:     domainlife.Alive,
					UnitName: "foo/1",
					UnitUUID: unitUUID2,
					UUID:     saUUID2,
				},
				{
					Filesystem: &internal.StorageInstanceInfoAttachmentFilesystem{
						MountPoint: "/mnt/fs-mount-3",
					},
					Life:     domainlife.Alive,
					UnitName: "foo/2",
					UnitUUID: unitUUID3,
					UUID:     saUUID3,
				},
			},
			Filesystem: &internal.StorageInstanceInfoFilesystem{
				Status: &internal.StorageInstanceInfoFilesystemStatus{
					Message:   "filesystem status",
					Status:    domainstatus.StorageFilesystemStatusTypeAttached,
					UpdatedAt: &statusTime,
				},
				UUID: fsUUID,
			},
			Life:      domainlife.Alive,
			Kind:      domainstorage.StorageKindFilesystem,
			StorageID: "123",
			UUID:      siUUID,
		}, nil,
	)

	svc := NewService(
		s.state, loggertesting.WrapCheckLog(c), clock.WallClock, s.storageRegistryGetter,
	)
	res, err := svc.GetStorageInstanceInfo(c.Context(), siUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, domainstorage.StorageInstanceInfo{
		FilesystemStatus: &domainstorage.StorageInstanceFilesystemStatus{
			Message: "filesystem status",
			Status:  corestatus.Attached,
			Since:   &statusTime,
			UUID:    fsUUID,
		},
		ID:         "123",
		Life:       domainlife.Alive,
		Kind:       domainstorage.StorageKindFilesystem,
		Persistent: false,
		UnitAttachments: []domainstorage.StorageInstanceUnitAttachment{
			{
				Life:     domainlife.Alive,
				Location: "/mnt/fs-mount-1",
				UnitName: "foo/0",
				UnitUUID: unitUUID1,
				UUID:     saUUID1,
			},
			{
				Life:     domainlife.Alive,
				Location: "/mnt/fs-mount-2",
				UnitName: "foo/1",
				UnitUUID: unitUUID2,
				UUID:     saUUID2,
			},
			{
				Life:     domainlife.Alive,
				Location: "/mnt/fs-mount-3",
				UnitName: "foo/2",
				UnitUUID: unitUUID3,
				UUID:     saUUID3,
			},
		},
		UUID: siUUID,
	})
}

// TestGetStorageInstanceInfoBlockVolume tests that
// [Service.GetStorageInstanceInfo] correctly returns and transforms storage
// instance information for a block storage instance backed by a volume and
// attached to a single unit on a machine. The test verifies that the service
// properly converts volume status and sets the location based on device links.
func (s *instanceSuite) TestGetStorageInstanceInfoBlockVolume(c *tc.C) {
	defer s.setupMocks(c).Finish()
	siUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	saUUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	vUUID := tc.Must(c, domainstorage.NewVolumeUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	machineUUID := tc.Must(c, coremachine.NewUUID)
	statusTime := time.Now()

	stateExp := s.state.EXPECT()
	stateExp.GetStorageInstanceInfo(c.Context(), siUUID).Return(
		internal.StorageInstanceInfo{
			Attachments: []internal.StorageInstanceInfoAttachment{
				{
					Life: domainlife.Alive,
					Machine: &internal.StorageInstanceInfoAttachmentMachine{
						Name: "machine-0",
						UUID: machineUUID,
					},
					Volume: &internal.StorageInstanceInfoAttachmentVolume{
						DeviceNameLinks: []string{"/dev/disk/by-id/scsi-123", "/dev/sdb"},
					},
					UnitName: "foo/0",
					UnitUUID: unitUUID,
					UUID:     saUUID,
				},
			},
			Life:      domainlife.Alive,
			Kind:      domainstorage.StorageKindBlock,
			StorageID: "456",
			UnitOwner: &internal.StorageInstanceInfoUnitOwner{
				Name: "foo/0",
				UUID: unitUUID,
			},
			UUID: siUUID,
			Volume: &internal.StorageInstanceInfoVolume{
				Persistent: true,
				Status: &internal.StorageInstanceInfoVolumeStatus{
					Message:   "volume attached",
					Status:    domainstatus.StorageVolumeStatusTypeAttached,
					UpdatedAt: &statusTime,
				},
				UUID: vUUID,
			},
		}, nil,
	)

	svc := NewService(
		s.state, loggertesting.WrapCheckLog(c), clock.WallClock, s.storageRegistryGetter,
	)
	res, err := svc.GetStorageInstanceInfo(c.Context(), siUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, domainstorage.StorageInstanceInfo{
		ID:         "456",
		Life:       domainlife.Alive,
		Kind:       domainstorage.StorageKindBlock,
		Persistent: true,
		UnitAttachments: []domainstorage.StorageInstanceUnitAttachment{
			{
				Life:     domainlife.Alive,
				Location: "/dev/disk/by-id/scsi-123",
				MachineAttachment: &domainstorage.StorageInstanceMachineAttachment{
					MachineName: "machine-0",
					MachineUUID: machineUUID,
				},
				UnitName: "foo/0",
				UnitUUID: unitUUID,
				UUID:     saUUID,
			},
		},
		UnitOwner: &domainstorage.StorageInstanceUnitOwner{
			Name: "foo/0",
			UUID: unitUUID,
		},
		UUID: siUUID,
		VolumeStatus: &domainstorage.StorageInstanceVolumeStatus{
			Message: "volume attached",
			Status:  corestatus.Attached,
			Since:   &statusTime,
			UUID:    vUUID,
		},
	})
}
