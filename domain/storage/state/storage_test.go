// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/status"
	statusstate "github.com/juju/juju/domain/status/state"
	"github.com/juju/juju/domain/storage"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type storageSuite struct {
	baseSuite
}

func TestStorageSuite(t *testing.T) {
	tc.Run(t, &storageSuite{})
}

func (s *storageSuite) TestListStorageInstancesWithEmpty(c *tc.C) {

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	result, err := st.ListStorageInstances(c.Context())
	c.Assert(err, tc.ErrorIs, nil)
	c.Assert(result, tc.HasLen, 0)
}

func (s *storageSuite) TestListStorageInstances(c *tc.C) {

	ch0 := s.newCharm(c)
	s.newCharmStorage(c, ch0, "blk", storage.StorageKindBlock)
	s.newCharmStorage(c, ch0, "fs", storage.StorageKindFilesystem)

	blkPoolUUID := s.newStoragePool(c, "blkpool", "blkpool", nil)
	fsPoolUUID := s.newStoragePool(c, "fspool", "fspool", nil)

	// Block storage instance with no attachments.
	_, instanceID0 := s.newStorageInstance(c, ch0, "blk", blkPoolUUID, storage.StorageKindBlock)

	// Filesystem storage instance with no attachments.
	_, instanceID1 := s.newStorageInstance(c, ch0, "fs", fsPoolUUID, storage.StorageKindFilesystem)

	// Block storage instance attached to a unit.
	a2 := s.newApplication(c, "foo", ch0)
	netNodeUUID2 := s.newNetNode(c)
	u2, u2n := s.newUnitWithNetNode(c, a2, netNodeUUID2)
	s2, instanceID2 := s.newStorageInstance(c, ch0, "blk", blkPoolUUID, storage.StorageKindBlock)
	s.newStorageAttachment(c, s2, u2)
	v2, _ := s.newVolume(c)
	s.changeVolumeInfo(c, v2, "vol-123", 1234, "hwid", "wwn", true)
	s.newStorageInstanceVolume(c, s2, v2)
	s.newVolumeAttachment(c, v2, netNodeUUID2)
	s.newStorageUnitOwner(c, s2, u2)

	// Filesystem storage instance attached to a unit.
	a3 := s.newApplication(c, "bar", ch0)
	netNodeUUID3 := s.newNetNode(c)
	u3, u3n := s.newUnitWithNetNode(c, a3, netNodeUUID3)
	s3, _ := s.newStorageInstance(c, ch0, "fs", fsPoolUUID, storage.StorageKindFilesystem)
	instanceID3 := s.getStorageID(c, s3)
	s.newStorageAttachment(c, s3, u3)
	s.newStorageUnitOwner(c, s3, u3)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	result, err := st.ListStorageInstances(c.Context())
	c.Assert(err, tc.ErrorIs, nil)
	c.Assert(result, tc.SameContents, []storage.StorageInstanceDetails{
		{ID: instanceID0, Kind: storage.StorageKindBlock, Life: life.Alive, Persistent: false},
		{ID: instanceID1, Kind: storage.StorageKindFilesystem, Life: life.Alive, Persistent: false},
		{ID: instanceID2, Owner: &u2n, Kind: storage.StorageKindBlock, Life: life.Alive, Persistent: true},
		{ID: instanceID3, Owner: &u3n, Kind: storage.StorageKindFilesystem, Life: life.Alive, Persistent: false},
	})
}

func (s *storageSuite) TestListVolumeWithAttachmentsWithEmpty(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// No storage instance IDs.
	result, err := st.ListVolumeWithAttachments(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, map[string]VolumeDetails{})

}

func (s *storageSuite) TestListVolumeWithAttachments(c *tc.C) {
	statusstate := statusstate.NewModelState(
		s.TxnRunnerFactory(),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

	ch0 := s.newCharm(c)
	s.newCharmStorage(c, ch0, "blk", storage.StorageKindBlock)
	s.newCharmStorage(c, ch0, "fs", storage.StorageKindFilesystem)

	blkPoolUUID := s.newStoragePool(c, "blkpool", "blkpool", nil)
	fsPoolUUID := s.newStoragePool(c, "fspool", "fspool", nil)

	// Filesystem storage instance with no attachments.
	sFs, _ := s.newStorageInstance(c, ch0, "fs", fsPoolUUID, storage.StorageKindFilesystem)
	instanceIDWithFS := s.getStorageID(c, sFs)

	// Volume attachment to a unit with no block device.
	a0 := s.newApplication(c, "foo", ch0)
	nn0 := s.newNetNode(c)
	u0, u0n := s.newUnitWithNetNode(c, a0, nn0)
	s0, instanceID0 := s.newStorageInstance(c, ch0, "blk", blkPoolUUID, storage.StorageKindBlock)
	s.newStorageAttachment(c, s0, u0)
	v0, _ := s.newVolume(c)
	s.changeVolumeInfo(c, v0, "vol-123", 1234, "hwid", "wwn", true)
	s.newStorageInstanceVolume(c, s0, v0)
	s.newVolumeAttachment(c, v0, nn0)
	s.newStorageUnitOwner(c, s0, u0)
	statusstate.SetVolumeStatus(c.Context(), v0, status.StatusInfo[status.StorageVolumeStatusType]{
		Status:  status.StorageVolumeStatusTypeAttaching,
		Message: "attaching the volumez",
	})

	// Volume attachment to a unit and machine with no block device.
	a1 := s.newApplication(c, "bar", ch0)
	nn1 := s.newNetNode(c)
	_, m1n := s.newMachineWithNetNode(c, nn1)
	u1, u1n := s.newUnitWithNetNode(c, a1, nn1)
	s1, instanceID1 := s.newStorageInstance(c, ch0, "blk", blkPoolUUID, storage.StorageKindBlock)
	s.newStorageAttachment(c, s1, u1)
	v1, _ := s.newVolume(c)
	s.newStorageInstanceVolume(c, s1, v1)
	s.newVolumeAttachment(c, v1, nn1)
	s.newStorageUnitOwner(c, s1, u1)

	// Volume attachment to a unit and machine with block device.
	a2 := s.newApplication(c, "baz", ch0)
	nn2 := s.newNetNode(c)
	m2, m2n := s.newMachineWithNetNode(c, nn2)
	u2, u2n := s.newUnitWithNetNode(c, a2, nn2)
	s2, instanceID2 := s.newStorageInstance(c, ch0, "blk", blkPoolUUID, storage.StorageKindBlock)
	s.newStorageAttachment(c, s2, u2)
	v2, _ := s.newVolume(c)
	s.newStorageInstanceVolume(c, s2, v2)
	v2a := s.newVolumeAttachment(c, v2, nn2)
	bd0 := s.newBlockDevice(c, m2, "blocky", "blocky-hw-id", "blocky-bus-addr", []string{
		"/dev/blocky",
		"/dev/disk/by-id/blocky",
	})
	s.changeVolumeAttachmentInfo(c, v2a, bd0, true)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	result, err := st.ListVolumeWithAttachments(c.Context(),
		instanceID0,
		instanceID1,
		instanceIDWithFS, // filesystem is ignored.
		instanceID2,
	)
	c.Assert(err, tc.ErrorIs, nil)
	c.Assert(result, tc.DeepEquals, map[string]VolumeDetails{
		instanceID0: {
			StorageID: instanceID0,
			Status: status.StatusInfo[status.StorageVolumeStatusType]{
				Status:  status.StorageVolumeStatusTypeAttaching,
				Message: "attaching the volumez",
			},
			Attachments: []VolumeAttachmentDetails{
				{
					AttachmentDetails: AttachmentDetails{
						Life: life.Alive,
						Unit: u0n,
					},
				},
			},
		},
		instanceID1: {
			StorageID: instanceID1,
			Status: status.StatusInfo[status.StorageVolumeStatusType]{
				Status: status.StorageVolumeStatusTypePending,
			},
			Attachments: []VolumeAttachmentDetails{
				{
					AttachmentDetails: AttachmentDetails{
						Life:    life.Alive,
						Unit:    u1n,
						Machine: &m1n,
					},
				},
			},
		},
		instanceID2: {
			StorageID: instanceID2,
			Status: status.StatusInfo[status.StorageVolumeStatusType]{
				Status: status.StorageVolumeStatusTypePending,
			},
			Attachments: []VolumeAttachmentDetails{
				{
					AttachmentDetails: AttachmentDetails{
						Life:    life.Alive,
						Unit:    u2n,
						Machine: &m2n,
					},
					BlockDeviceUUID: bd0,
				},
			},
		},
	})
}

func (s *storageSuite) TestStorageListFilesystemAttachmentsWithEmpty(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// No storage instance IDs.
	result, err := st.ListFilesystemWithAttachments(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, map[string]FilesystemDetails{})
}

func (s *storageSuite) TestStorageListFilesystemAttachments(c *tc.C) {
	ch0 := s.newCharm(c)
	s.newCharmStorage(c, ch0, "blk", storage.StorageKindBlock)
	s.newCharmStorage(c, ch0, "fs", storage.StorageKindFilesystem)

	blkPoolUUID := s.newStoragePool(c, "blkpool", "blkpool", nil)
	fsPoolUUID := s.newStoragePool(c, "fspool", "fspool", nil)

	// Volume storage instance with no attachments.
	sVol, _ := s.newStorageInstance(c, ch0, "blk", blkPoolUUID, storage.StorageKindBlock)
	instanceIDWithVol := s.getStorageID(c, sVol)

	// Filesystem attachment to a unit.
	a0 := s.newApplication(c, "foo", ch0)
	nn0 := s.newNetNode(c)
	u0, u0n := s.newUnitWithNetNode(c, a0, nn0)
	s0, instanceID0 := s.newStorageInstance(c, ch0, "fs", fsPoolUUID, storage.StorageKindFilesystem)
	s.newStorageAttachment(c, s0, u0)
	s.newStorageUnitOwner(c, s0, u0)
	fs0, _ := s.newFilesystemWithStatus(c, status.StorageFilesystemStatusTypePending)
	fsa0 := s.newFilesystemAttachment(c, fs0, nn0)
	s.changeFilesystemAttachmentInfo(c, fsa0, "/mnt/foo", false)
	s.newStorageInstanceFilesystem(c, s0, fs0)

	// Filesystem attachment to a unit and machine.
	a1 := s.newApplication(c, "bar", ch0)
	nn1 := s.newNetNode(c)
	_, m1n := s.newMachineWithNetNode(c, nn1)
	u1, u1n := s.newUnitWithNetNode(c, a1, nn1)
	s1, instanceID1 := s.newStorageInstance(c, ch0, "fs", fsPoolUUID, storage.StorageKindFilesystem)
	s.newStorageAttachment(c, s1, u1)
	s.newStorageUnitOwner(c, s1, u1)
	fs1, _ := s.newFilesystemWithStatus(c, status.StorageFilesystemStatusTypeAttaching)
	fsa1 := s.newFilesystemAttachment(c, fs1, nn1)
	s.changeFilesystemAttachmentInfo(c, fsa1, "/mnt/bar", false)
	s.newStorageInstanceFilesystem(c, s1, fs1)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	result, err := st.ListFilesystemWithAttachments(c.Context(),
		instanceID0,
		instanceID1,
		instanceIDWithVol, // volume is ignored.
	)
	c.Assert(err, tc.ErrorIs, nil)
	c.Assert(result, tc.DeepEquals, map[string]FilesystemDetails{
		instanceID0: {
			StorageID: instanceID0,
			Status: status.StatusInfo[status.StorageFilesystemStatusType]{
				Status: status.StorageFilesystemStatusTypePending,
			},
			Attachments: []FilesystemAttachmentDetails{
				{
					AttachmentDetails: AttachmentDetails{
						Life: life.Alive,
						Unit: u0n,
					},
					MountPoint: "/mnt/foo",
				},
			},
		},
		instanceID1: {
			StorageID: instanceID1,
			Status: status.StatusInfo[status.StorageFilesystemStatusType]{
				Status: status.StorageFilesystemStatusTypeAttaching,
			},
			Attachments: []FilesystemAttachmentDetails{
				{
					AttachmentDetails: AttachmentDetails{
						Life:    life.Alive,
						Unit:    u1n,
						Machine: &m1n,
					},
					MountPoint: "/mnt/bar",
				},
			},
		},
	})
}
