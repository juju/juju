// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/status"
	statuserrors "github.com/juju/juju/domain/status/errors"
	"github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningtesting "github.com/juju/juju/domain/storageprovisioning/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type storageStatusSuite struct {
	baseStorageSuite
}

type storageSuite struct {
	baseStorageSuite
	modelState *ModelState
}

func TestStorageStatusSuite(t *testing.T) {
	tc.Run(t, &storageStatusSuite{})
}

func TestStorageSuite(t *testing.T) {
	tc.Run(t, &storageSuite{})
}

func (s *storageSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	s.modelState = NewModelState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

func (s *storageSuite) assertFilesystemStatus(
	c *tc.C,
	filesystemUUID storageprovisioning.FilesystemUUID,
	expected status.StatusInfo[status.StorageFilesystemStatusType],
) {
	ctx := c.Context()

	var got status.StatusInfo[status.StorageFilesystemStatusType]
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
SELECT status_id, message, updated_at FROM storage_filesystem_status
WHERE filesystem_uuid=?`, filesystemUUID).Scan(
			&got.Status, &got.Message, &got.Since,
		)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, expected)
}

func (s *storageSuite) assertVolumeStatus(
	c *tc.C,
	volumeUUID storageprovisioning.VolumeUUID,
	expected status.StatusInfo[status.StorageVolumeStatusType]) {
	ctx := c.Context()

	var got status.StatusInfo[status.StorageVolumeStatusType]
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
SELECT status_id, message, updated_at FROM storage_volume_status
WHERE volume_uuid=?`, volumeUUID).Scan(
			&got.Status, &got.Message, &got.Since,
		)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, expected)
}

func (s *storageSuite) TestSetFilesystemStatus(c *tc.C) {
	filesystemUUID, _ := s.newFilesystemWithStatus(
		c, status.StorageFilesystemStatusTypePending,
	)

	now := time.Now().UTC()
	expected := status.StatusInfo[status.StorageFilesystemStatusType]{
		Status:  status.StorageFilesystemStatusTypeAttached,
		Message: "message",
		Since:   ptr(now),
	}

	err := s.modelState.SetFilesystemStatus(c.Context(), filesystemUUID, expected)
	c.Assert(err, tc.ErrorIsNil)
	s.assertFilesystemStatus(c, filesystemUUID, expected)
}

func (s *storageSuite) TestSetFilesystemStatusInitialMissing(c *tc.C) {
	filesystemUUID, _ := s.newFilesystem(c)

	now := time.Now().UTC()
	expected := status.StatusInfo[status.StorageFilesystemStatusType]{
		Status:  status.StorageFilesystemStatusTypeAttached,
		Message: "message",
		Since:   ptr(now),
	}

	err := s.modelState.SetFilesystemStatus(c.Context(), filesystemUUID, expected)
	c.Assert(err, tc.ErrorIsNil)
	s.assertFilesystemStatus(c, filesystemUUID, expected)
}

func (s *storageSuite) TestSetFilesystemStatusMultipleTimes(c *tc.C) {
	filesystemUUID, _ := s.newFilesystemWithStatus(
		c, status.StorageFilesystemStatusTypePending,
	)

	err := s.modelState.SetFilesystemStatus(c.Context(), filesystemUUID, status.StatusInfo[status.StorageFilesystemStatusType]{
		Status:  status.StorageFilesystemStatusTypeAttaching,
		Message: "waiting",
		Since:   ptr(time.Now().UTC()),
	})
	c.Assert(err, tc.ErrorIsNil)

	now := time.Now().UTC()
	expected := status.StatusInfo[status.StorageFilesystemStatusType]{
		Status:  status.StorageFilesystemStatusTypeAttached,
		Message: "attached with 60MB",
		Since:   ptr(now),
	}

	err = s.modelState.SetFilesystemStatus(c.Context(), filesystemUUID, expected)
	c.Assert(err, tc.ErrorIsNil)

	s.assertFilesystemStatus(c, filesystemUUID, expected)
}

func (s *storageSuite) TestSetFilesystemStatusFilesystemNotFound(c *tc.C) {
	now := time.Now().UTC()
	expected := status.StatusInfo[status.StorageFilesystemStatusType]{
		Status:  status.StorageFilesystemStatusTypeAttaching,
		Message: "waiting",
		Since:   ptr(now),
	}

	uuid := storageprovisioningtesting.GenFilesystemUUID(c)
	err := s.modelState.SetFilesystemStatus(c.Context(), uuid, expected)
	c.Assert(err, tc.ErrorIs, storageerrors.FilesystemNotFound)
}

func (s *storageSuite) TestSetFilesystemStatusInvalidStatus(c *tc.C) {
	filesystemUUID, _ := s.newFilesystemWithStatus(
		c, status.StorageFilesystemStatusTypePending,
	)

	expected := status.StatusInfo[status.StorageFilesystemStatusType]{
		Status: status.StorageFilesystemStatusType(99),
	}

	err := s.modelState.SetFilesystemStatus(c.Context(), filesystemUUID, expected)
	c.Assert(err, tc.ErrorMatches, `.*unknown status.*`)
}

// TestSetFilesystemStatusPendingWhenProvisioned ensures that a filesystem
// status cannot be transitionted to pending when the filesystem is considered
// to have been provisioned within the model.
//
// TODO (tlm): This is testing logic that is broken. The code that this is
// testing assumes a filesystem is provisioned when it is associated with a
// storage instance. This is always the case and a filesystem
// is currnetly considered provisioned when the provisioning information has
// been set by the storage provisioning worker.
//
// What most likely needs to happen in a follow up fix is that the filesystem
// needs to carry a "provisioned" flag that is set or the state machine that is
// status should consider this a valid status.
func (s *storageSuite) TestSetFilesystemStatusPendingWhenProvisioned(c *tc.C) {
	ch0 := s.newCharm(c)
	blkPoolUUID := s.newStoragePool(c, "blkpool", "blkpool", nil)
	s0, _ := s.newStorageInstance(c, ch0, "blk", blkPoolUUID, storage.StorageKindBlock)
	fsUUID, _ := s.newFilesystemWithStatus(
		c, status.StorageFilesystemStatusTypePending,
	)
	s.newStorageInstanceFilesystem(c, s0, fsUUID)

	now := time.Now().UTC()
	sts := status.StatusInfo[status.StorageFilesystemStatusType]{
		Status: status.StorageFilesystemStatusTypeAttached,
		Since:  ptr(now),
	}
	err := s.modelState.SetFilesystemStatus(c.Context(), fsUUID, sts)
	c.Assert(err, tc.ErrorIsNil)

	sts = status.StatusInfo[status.StorageFilesystemStatusType]{
		Status: status.StorageFilesystemStatusTypePending,
		Since:  ptr(now),
	}
	err = s.modelState.SetFilesystemStatus(c.Context(), fsUUID, sts)
	c.Assert(err, tc.ErrorIs, statuserrors.FilesystemStatusTransitionNotValid)
}

func (s *storageSuite) TestGetFilesystemUUIDByID(c *tc.C) {
	filesystemUUID, fsID := s.newFilesystemWithStatus(
		c, status.StorageFilesystemStatusTypePending,
	)

	gotUUID, err := s.modelState.GetFilesystemUUIDByID(c.Context(), fsID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotUUID, tc.Equals, filesystemUUID)
}

func (s *storageSuite) TestGetFilesystemUUIDByIDNotFound(c *tc.C) {
	_, err := s.modelState.GetFilesystemUUIDByID(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, storageerrors.FilesystemNotFound)
}

func (s *storageSuite) TestImportFilesystemStatus(c *tc.C) {
	filesystemUUID, _ := s.newFilesystemWithStatus(
		c, status.StorageFilesystemStatusTypePending,
	)

	now := time.Now().UTC()
	expected := status.StatusInfo[status.StorageFilesystemStatusType]{
		Status:  status.StorageFilesystemStatusTypeAttached,
		Message: "message",
		Since:   ptr(now),
	}

	err := s.modelState.ImportFilesystemStatus(c.Context(), filesystemUUID, expected)
	c.Assert(err, tc.ErrorIsNil)
	s.assertFilesystemStatus(c, filesystemUUID, expected)
}

func (s *storageSuite) TestSetVolumeStatus(c *tc.C) {
	volumeUUID, _ := s.newVolumeWithStatus(c, status.StorageVolumeStatusTypePending)

	now := time.Now().UTC()
	expected := status.StatusInfo[status.StorageVolumeStatusType]{
		Status:  status.StorageVolumeStatusTypeAttached,
		Message: "message",
		Since:   ptr(now),
	}

	err := s.modelState.SetVolumeStatus(c.Context(), volumeUUID, expected)
	c.Assert(err, tc.ErrorIsNil)
	s.assertVolumeStatus(c, volumeUUID, expected)
}

func (s *storageSuite) TestSetVolumeStatusInitialMissing(c *tc.C) {
	volumeUUID, _ := s.newVolume(c)

	now := time.Now().UTC()
	expected := status.StatusInfo[status.StorageVolumeStatusType]{
		Status:  status.StorageVolumeStatusTypeAttached,
		Message: "message",
		Since:   ptr(now),
	}

	err := s.modelState.SetVolumeStatus(c.Context(), volumeUUID, expected)
	c.Assert(err, tc.ErrorIsNil)
	s.assertVolumeStatus(c, volumeUUID, expected)
}

func (s *storageSuite) TestSetVolumeStatusMultipleTimes(c *tc.C) {
	volumeUUID, _ := s.newVolumeWithStatus(c, status.StorageVolumeStatusTypePending)

	err := s.modelState.SetVolumeStatus(c.Context(), volumeUUID, status.StatusInfo[status.StorageVolumeStatusType]{
		Status:  status.StorageVolumeStatusTypeAttaching,
		Message: "waiting",
		Since:   ptr(time.Now().UTC()),
	})
	c.Assert(err, tc.ErrorIsNil)

	now := time.Now().UTC()
	expected := status.StatusInfo[status.StorageVolumeStatusType]{
		Status:  status.StorageVolumeStatusTypeAttached,
		Message: "attached with 60MB",
		Since:   ptr(now),
	}

	err = s.modelState.SetVolumeStatus(c.Context(), volumeUUID, expected)
	c.Assert(err, tc.ErrorIsNil)

	s.assertVolumeStatus(c, volumeUUID, expected)
}

func (s *storageSuite) TestSetVolumeStatusVolumeNotFound(c *tc.C) {
	now := time.Now().UTC()
	expected := status.StatusInfo[status.StorageVolumeStatusType]{
		Status:  status.StorageVolumeStatusTypeAttaching,
		Message: "waiting",
		Since:   ptr(now),
	}

	uuid := storageprovisioningtesting.GenVolumeUUID(c)
	err := s.modelState.SetVolumeStatus(c.Context(), uuid, expected)
	c.Assert(err, tc.ErrorIs, storageerrors.VolumeNotFound)
}

func (s *storageSuite) TestSetVolumeStatusInvalidStatus(c *tc.C) {
	volumeUUID, _ := s.newVolumeWithStatus(c, status.StorageVolumeStatusTypePending)

	expected := status.StatusInfo[status.StorageVolumeStatusType]{
		Status: status.StorageVolumeStatusType(99),
	}

	err := s.modelState.SetVolumeStatus(c.Context(), volumeUUID, expected)
	c.Assert(err, tc.ErrorMatches, `.*unknown status.*`)
}

// TestSetVolumeStatusPendingWhenProvisioned ensures that a volume
// status cannot be transitionted to pending when the volume is considered
// to have been provisioned within the model.
//
// TODO (tlm): This is testing logic that is broken. The code that this is
// testing assumes a volume is provisioned when it is associated with a
// storage instance. This is always the case and a volume
// is currnetly considered provisioned when the provisioning information has
// been set by the storage provisioning worker.
//
// What most likely needs to happen in a follow up fix is that the volume
// needs to carry a "provisioned" flag that is set or the state machine that is
// status should consider this a valid status.
func (s *storageSuite) TestSetVolumeStatusPendingWhenProvisioned(c *tc.C) {
	ch0 := s.newCharm(c)
	blkPoolUUID := s.newStoragePool(c, "blkpool", "blkpool", nil)
	s0, _ := s.newStorageInstance(c, ch0, "blk", blkPoolUUID, storage.StorageKindBlock)
	vUUID, _ := s.newVolumeWithStatus(c, status.StorageVolumeStatusTypePending)
	s.newStorageInstanceVolume(c, s0, vUUID)
	now := time.Now().UTC()

	sts := status.StatusInfo[status.StorageVolumeStatusType]{
		Status: status.StorageVolumeStatusTypeAttached,
		Since:  ptr(now),
	}
	err := s.modelState.SetVolumeStatus(c.Context(), vUUID, sts)
	c.Assert(err, tc.ErrorIsNil)

	sts = status.StatusInfo[status.StorageVolumeStatusType]{
		Status: status.StorageVolumeStatusTypePending,
		Since:  ptr(now),
	}
	err = s.modelState.SetVolumeStatus(c.Context(), vUUID, sts)
	c.Assert(err, tc.ErrorIs, statuserrors.VolumeStatusTransitionNotValid)
}

func (s *storageSuite) TestGetVolumeUUIDByID(c *tc.C) {
	volumeUUID, vsID := s.newVolumeWithStatus(c, status.StorageVolumeStatusTypePending)

	gotUUID, err := s.modelState.GetVolumeUUIDByID(c.Context(), vsID)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(gotUUID, tc.Equals, volumeUUID)
}

func (s *storageSuite) TestGetVolumeUUIDByIDNotFound(c *tc.C) {
	_, err := s.modelState.GetVolumeUUIDByID(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, storageerrors.VolumeNotFound)
}

func (s *storageSuite) TestImportVolumeStatus(c *tc.C) {
	volumeUUID, _ := s.newVolumeWithStatus(c, status.StorageVolumeStatusTypePending)

	now := time.Now().UTC()
	expected := status.StatusInfo[status.StorageVolumeStatusType]{
		Status:  status.StorageVolumeStatusTypeAttached,
		Message: "message",
		Since:   ptr(now),
	}

	err := s.modelState.ImportVolumeStatus(c.Context(), volumeUUID, expected)
	c.Assert(err, tc.ErrorIsNil)
	s.assertVolumeStatus(c, volumeUUID, expected)
}

func (s *storageStatusSuite) NewModelState(c *tc.C) *ModelState {
	return NewModelState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

func (s *storageStatusSuite) TestGetStorageInstancesEmpty(c *tc.C) {
	st := s.NewModelState(c)
	res, err := st.GetStorageInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.HasLen, 0)
}

func (s *storageStatusSuite) TestGetStorageInstances(c *tc.C) {
	ch0 := s.newCharm(c)
	s.newCharmStorage(c, ch0, "blk", storage.StorageKindBlock)
	s.newCharmStorage(c, ch0, "fs", storage.StorageKindFilesystem)

	blkPoolUUID := s.newStoragePool(c, "blkpool", "blkpool", nil)
	fsPoolUUID := s.newStoragePool(c, "fspool", "fspool", nil)

	// Block device storage instance with no owner that is dying.
	s0, _ := s.newStorageInstance(c, ch0, "blk", blkPoolUUID, storage.StorageKindBlock)
	s.changeStorageInstanceLife(c, s0.String(), life.Dying)

	// Filesystem storage instance with an owning unit that is alive.
	s1, _ := s.newStorageInstance(c, ch0, "fs", fsPoolUUID, storage.StorageKindFilesystem)
	a0 := s.newApplication(c, "foo", ch0)
	nn0 := s.newNetNode(c)
	u0, u0n := s.newUnitWithNetNode(c, a0, nn0)
	s.newStorageUnitOwner(c, s1, u0)

	st := s.NewModelState(c)
	res, err := st.GetStorageInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.UnorderedMatch[[]status.StorageInstance](tc.DeepEquals), []status.StorageInstance{
		{
			UUID: s0,
			ID:   "blk/0",
			Life: life.Dying,
			Kind: storage.StorageKindBlock,
		},
		{
			UUID:  s1,
			ID:    "fs/1",
			Life:  life.Alive,
			Owner: &u0n,
			Kind:  storage.StorageKindFilesystem,
		},
	})
}

func (s *storageStatusSuite) TestGetStorageInstanceAttachmentsEmpty(c *tc.C) {
	st := s.NewModelState(c)
	res, err := st.GetStorageInstanceAttachments(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.HasLen, 0)
}

func (s *storageStatusSuite) TestGetStorageInstanceAttachments(c *tc.C) {
	ch0 := s.newCharm(c)
	s.newCharmStorage(c, ch0, "blk", storage.StorageKindBlock)
	s.newCharmStorage(c, ch0, "fs", storage.StorageKindFilesystem)

	blkPoolUUID := s.newStoragePool(c, "blkpool", "blkpool", nil)
	fsPoolUUID := s.newStoragePool(c, "fspool", "fspool", nil)

	// Storage instance attachment of a block device storage instance with a
	// unit only attachment.
	a0 := s.newApplication(c, "foo", ch0)
	nn0 := s.newNetNode(c)
	u0, u0n := s.newUnitWithNetNode(c, a0, nn0)
	s0, _ := s.newStorageInstance(c, ch0, "blk", blkPoolUUID, storage.StorageKindBlock)
	s.newStorageAttachment(c, s0, u0)

	// Storage instance attachment of a filesystem storage instance with a unit
	// attachment on a machine.
	a1 := s.newApplication(c, "bar", ch0)
	nn1 := s.newNetNode(c)
	_, m1n := s.newMachineWithNetNode(c, nn1)
	u1, u1n := s.newUnitWithNetNode(c, a1, nn1)
	s1, _ := s.newStorageInstance(c, ch0, "fs", fsPoolUUID, storage.StorageKindFilesystem)
	s.newStorageAttachment(c, s1, u1)

	st := s.NewModelState(c)
	res, err := st.GetStorageInstanceAttachments(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.UnorderedMatch[[]status.StorageAttachment](tc.DeepEquals), []status.StorageAttachment{
		{
			StorageInstanceUUID: s0,
			Life:                life.Alive,
			Unit:                u0n,
		},
		{
			StorageInstanceUUID: s1,
			Life:                life.Alive,
			Unit:                u1n,
			Machine:             &m1n,
		},
	})
}

func (s *storageStatusSuite) TestGetFilesystemsEmpty(c *tc.C) {
	st := s.NewModelState(c)
	res, err := st.GetFilesystems(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.HasLen, 0)
}

func (s *storageStatusSuite) TestGetFilesystems(c *tc.C) {
	ch0 := s.newCharm(c)
	s.newCharmStorage(c, ch0, "fs", storage.StorageKindFilesystem)

	fsPoolUUID := s.newStoragePool(c, "fspool", "fspool", nil)

	// Filesystem with status, size and a provider id.
	a0 := s.newApplication(c, "foo", ch0)
	nn0 := s.newNetNode(c)
	u0, _ := s.newUnitWithNetNode(c, a0, nn0)
	s0, s0id := s.newStorageInstance(c, ch0, "fs", fsPoolUUID, storage.StorageKindFilesystem)
	s.newStorageAttachment(c, s0, u0)
	f0, f0id := s.newFilesystem(c)
	s.changeFilesystemInfo(c, f0, "my-provider-id-1", 123)
	s.newStorageInstanceFilesystem(c, s0, f0)

	// Filesystem backed by a volume with size and provider id.
	s1, s1id := s.newStorageInstance(c, ch0, "fs", fsPoolUUID, storage.StorageKindFilesystem)
	f1, f1id := s.newFilesystem(c)
	s.changeFilesystemInfo(c, f1, "my-provider-id-2", 456)
	s.newStorageInstanceFilesystem(c, s1, f1)
	v1, v1id := s.newVolume(c)
	s.newStorageInstanceVolume(c, s1, v1)

	st := s.NewModelState(c)
	err := st.SetFilesystemStatus(c.Context(), f0, status.StatusInfo[status.StorageFilesystemStatusType]{
		Status:  status.StorageFilesystemStatusTypeAttaching,
		Message: "attaching the filez",
	})
	c.Assert(err, tc.ErrorIsNil)

	res, err := st.GetFilesystems(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.UnorderedMatch[[]status.Filesystem](tc.DeepEquals), []status.Filesystem{
		{
			UUID: f0,
			ID:   f0id,
			Life: life.Alive,
			Status: status.StatusInfo[status.StorageFilesystemStatusType]{
				Status:  status.StorageFilesystemStatusTypeAttaching,
				Message: "attaching the filez",
			},
			StorageID:  s0id,
			ProviderID: "my-provider-id-1",
			SizeMiB:    123,
		},
		{
			UUID: f1,
			ID:   f1id,
			Life: life.Alive,
			Status: status.StatusInfo[status.StorageFilesystemStatusType]{
				Status: status.StorageFilesystemStatusTypePending,
			},
			StorageID:  s1id,
			ProviderID: "my-provider-id-2",
			SizeMiB:    456,
			VolumeID:   &v1id,
		},
	})
}

func (s *storageStatusSuite) TestGetFilesystemAttachmentsEmpty(c *tc.C) {
	st := s.NewModelState(c)
	res, err := st.GetFilesystemAttachments(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.HasLen, 0)
}

func (s *storageStatusSuite) TestGetFilesystemAttachments(c *tc.C) {
	ch0 := s.newCharm(c)
	s.newCharmStorage(c, ch0, "fs", storage.StorageKindFilesystem)

	fsPoolUUID := s.newStoragePool(c, "fspool", "fspool", nil)

	// Filesystem attachment to a unit with a read only mount.
	a0 := s.newApplication(c, "foo", ch0)
	nn0 := s.newNetNode(c)
	u0, u0n := s.newUnitWithNetNode(c, a0, nn0)
	s0, _ := s.newStorageInstance(c, ch0, "fs", fsPoolUUID, storage.StorageKindFilesystem)
	s.newStorageAttachment(c, s0, u0)
	f0, _ := s.newFilesystem(c)
	s.newStorageInstanceFilesystem(c, s0, f0)
	f0a := s.newFilesystemAttachment(c, f0, nn0)
	s.changeFilesystemAttachmentInfo(c, f0a, "/mnt/x", true)

	// Filesystem attachment to a unit on a machine with a writable mount.
	a1 := s.newApplication(c, "bar", ch0)
	nn1 := s.newNetNode(c)
	_, m1n := s.newMachineWithNetNode(c, nn1)
	u1, u1n := s.newUnitWithNetNode(c, a1, nn1)
	s1, _ := s.newStorageInstance(c, ch0, "fs", fsPoolUUID, storage.StorageKindFilesystem)
	s.newStorageAttachment(c, s1, u1)
	f1, _ := s.newFilesystem(c)
	s.newStorageInstanceFilesystem(c, s1, f1)
	f1a := s.newFilesystemAttachment(c, f1, nn1)
	s.changeFilesystemAttachmentInfo(c, f1a, "/mnt/y", false)

	st := s.NewModelState(c)
	res, err := st.GetFilesystemAttachments(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.UnorderedMatch[[]status.FilesystemAttachment](tc.DeepEquals), []status.FilesystemAttachment{
		{
			FilesystemUUID: f0,
			Life:           life.Alive,
			Unit:           &u0n,
			MountPoint:     "/mnt/x",
			ReadOnly:       true,
		},
		{
			FilesystemUUID: f1,
			Life:           life.Alive,
			Unit:           &u1n,
			Machine:        &m1n,
			MountPoint:     "/mnt/y",
			ReadOnly:       false,
		},
	})
}

func (s *storageStatusSuite) TestGetVolumesEmpty(c *tc.C) {
	st := s.NewModelState(c)
	res, err := st.GetVolumes(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.HasLen, 0)
}

func (s *storageStatusSuite) TestGetVolumes(c *tc.C) {
	ch0 := s.newCharm(c)
	s.newCharmStorage(c, ch0, "blk", storage.StorageKindBlock)

	blkPoolUUID := s.newStoragePool(c, "blkpool", "blkpool", nil)

	// Volume with a unit, status, size, provider id, hardware id, wwn and is
	// persistent.
	a0 := s.newApplication(c, "foo", ch0)
	nn0 := s.newNetNode(c)
	u0, _ := s.newUnitWithNetNode(c, a0, nn0)
	s0, s0id := s.newStorageInstance(c, ch0, "blk", blkPoolUUID, storage.StorageKindBlock)
	s.newStorageAttachment(c, s0, u0)
	v0, v0id := s.newVolume(c)
	s.changeVolumeInfo(c, v0, "my-provider-id-1", 123, "hw0", "wwn0", true)
	s.newStorageInstanceVolume(c, s0, v0)

	// Volume pending.
	a1 := s.newApplication(c, "bar", ch0)
	nn1 := s.newNetNode(c)
	u1, _ := s.newUnitWithNetNode(c, a1, nn1)
	s1, s1id := s.newStorageInstance(c, ch0, "blk", blkPoolUUID, storage.StorageKindBlock)
	s.newStorageAttachment(c, s1, u1)
	v1, v1id := s.newVolume(c)
	s.newStorageInstanceVolume(c, s1, v1)

	st := s.NewModelState(c)
	err := st.SetVolumeStatus(c.Context(), v0, status.StatusInfo[status.StorageVolumeStatusType]{
		Status:  status.StorageVolumeStatusTypeAttaching,
		Message: "attaching the volumez",
	})
	c.Assert(err, tc.ErrorIsNil)

	res, err := st.GetVolumes(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.UnorderedMatch[[]status.Volume](tc.DeepEquals), []status.Volume{
		{
			UUID: v0,
			ID:   v0id,
			Life: life.Alive,
			Status: status.StatusInfo[status.StorageVolumeStatusType]{
				Status:  status.StorageVolumeStatusTypeAttaching,
				Message: "attaching the volumez",
			},
			StorageID:  s0id,
			ProviderID: "my-provider-id-1",
			SizeMiB:    123,
			HardwareID: "hw0",
			WWN:        "wwn0",
			Persistent: true,
		},
		{
			UUID: v1,
			ID:   v1id,
			Life: life.Alive,
			Status: status.StatusInfo[status.StorageVolumeStatusType]{
				Status: status.StorageVolumeStatusTypePending,
			},
			StorageID: s1id,
		},
	})
}

func (s *storageStatusSuite) TestGetVolumeAttachmentsEmpty(c *tc.C) {
	st := s.NewModelState(c)
	res, err := st.GetVolumeAttachments(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.HasLen, 0)
}

func (s *storageStatusSuite) TestGetVolumeAttachments(c *tc.C) {
	ch0 := s.newCharm(c)
	s.newCharmStorage(c, ch0, "blk", storage.StorageKindBlock)

	blkPoolUUID := s.newStoragePool(c, "blkpool", "blkpool", nil)

	// Volume attachment to a unit with no block device.
	a0 := s.newApplication(c, "foo", ch0)
	nn0 := s.newNetNode(c)
	u0, u0n := s.newUnitWithNetNode(c, a0, nn0)
	s0, _ := s.newStorageInstance(c, ch0, "blk", blkPoolUUID, storage.StorageKindBlock)
	s.newStorageAttachment(c, s0, u0)
	v0, _ := s.newVolume(c)
	s.newStorageInstanceVolume(c, s0, v0)
	s.newVolumeAttachment(c, v0, nn0)

	// Volume attachment to a unit and machine with no block device.
	a1 := s.newApplication(c, "bar", ch0)
	nn1 := s.newNetNode(c)
	_, m1n := s.newMachineWithNetNode(c, nn1)
	u1, u1n := s.newUnitWithNetNode(c, a1, nn1)
	s1, _ := s.newStorageInstance(c, ch0, "blk", blkPoolUUID, storage.StorageKindBlock)
	s.newStorageAttachment(c, s1, u1)
	v1, _ := s.newVolume(c)
	s.newStorageInstanceVolume(c, s1, v1)
	s.newVolumeAttachment(c, v1, nn1)

	// Volume attachment to a unit and machine with block device.
	a2 := s.newApplication(c, "baz", ch0)
	nn2 := s.newNetNode(c)
	m2, m2n := s.newMachineWithNetNode(c, nn2)
	u2, u2n := s.newUnitWithNetNode(c, a2, nn2)
	s2, _ := s.newStorageInstance(c, ch0, "blk", blkPoolUUID, storage.StorageKindBlock)
	s.newStorageAttachment(c, s2, u2)
	v2, _ := s.newVolume(c)
	s.newStorageInstanceVolume(c, s2, v2)
	v2a := s.newVolumeAttachment(c, v2, nn2)
	bd0 := s.newBlockDevice(c, m2, "blocky", "blocky-hw-id", "blocky-bus-addr", []string{
		"/dev/blocky",
		"/dev/disk/by-id/blocky",
	})
	s.changeVolumeAttachmentInfo(c, v2a, bd0, true)

	// Volume attachment to a unit and machine with no block device but with an
	// attachment plan.
	a3 := s.newApplication(c, "zaz", ch0)
	nn3 := s.newNetNode(c)
	_, m3n := s.newMachineWithNetNode(c, nn3)
	u3, u3n := s.newUnitWithNetNode(c, a3, nn3)
	s3, _ := s.newStorageInstance(c, ch0, "blk", blkPoolUUID, storage.StorageKindBlock)
	s.newStorageAttachment(c, s3, u3)
	v3, _ := s.newVolume(c)
	s.newStorageInstanceVolume(c, s3, v3)
	s.newVolumeAttachment(c, v3, nn3)
	s.newStorageVolumeAttachmentPlan(c, v3, nn3,
		storageprovisioning.PlanDeviceTypeISCSI,
		map[string]string{"a": "b", "c": "d"},
	)

	st := s.NewModelState(c)
	res, err := st.GetVolumeAttachments(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.UnorderedMatch[[]status.VolumeAttachment](tc.DeepEquals), []status.VolumeAttachment{
		{
			VolumeUUID: v0,
			Life:       life.Alive,
			Unit:       &u0n,
		},
		{
			VolumeUUID: v1,
			Life:       life.Alive,
			Unit:       &u1n,
			Machine:    &m1n,
		},
		{
			VolumeUUID: v2,
			Life:       life.Alive,
			Unit:       &u2n,
			Machine:    &m2n,
			DeviceName: "blocky",
			BusAddress: "blocky-bus-addr",
			DeviceLink: "/dev/blocky",
			ReadOnly:   true,
		},
		{
			VolumeUUID: v3,
			Life:       life.Alive,
			Unit:       &u3n,
			Machine:    &m3n,
			VolumeAttachmentPlan: &status.VolumeAttachmentPlan{
				DeviceType: storageprovisioning.PlanDeviceTypeISCSI,
				DeviceAttributes: map[string]string{
					"a": "b",
					"c": "d",
				},
			},
		},
	})
}

func (s *storageStatusSuite) newStorageVolumeAttachmentPlan(
	c *tc.C,
	volumeUUID storageprovisioning.VolumeUUID,
	netNodeUUID domainnetwork.NetNodeUUID,
	deviceTypeID storageprovisioning.PlanDeviceType,
	attrs map[string]string,
) string {
	vapUUID := uuid.MustNewUUID().String()
	_, err := s.DB().Exec(
		`INSERT INTO storage_volume_attachment_plan(uuid, storage_volume_uuid, net_node_uuid, life_id, device_type_id, provision_scope_id) VALUES(?, ?, ?, 0, ?, 1)`,
		vapUUID, volumeUUID, netNodeUUID, deviceTypeID)
	c.Assert(err, tc.ErrorIsNil)
	for key, value := range attrs {
		_, err := s.DB().Exec(
			`INSERT INTO storage_volume_attachment_plan_attr(attachment_plan_uuid, key, value) VALUES(?, ?, ?)`,
			vapUUID, key, value)
		c.Assert(err, tc.ErrorIsNil)
	}
	return vapUUID
}

func (s *storageStatusSuite) newBlockDevice(
	c *tc.C,
	machineUUID machine.UUID,
	name string,
	hardwareID string,
	busAddress string,
	deviceLinks []string,
) string {
	uuid := uuid.MustNewUUID().String()
	_, err := s.DB().Exec(
		`INSERT INTO block_device(uuid, machine_uuid, name, hardware_id, bus_address) VALUES(?, ?, ?, ?, ?)`,
		uuid, machineUUID, name, hardwareID, busAddress)
	c.Assert(err, tc.ErrorIsNil)
	for _, deviceLink := range deviceLinks {
		_, err := s.DB().Exec(
			`INSERT INTO block_device_link_device(block_device_uuid, machine_uuid, name) VALUES(?, ?, ?)`,
			uuid, machineUUID, deviceLink)
		c.Assert(err, tc.ErrorIsNil)
	}
	return uuid
}

func (s *storageStatusSuite) changeVolumeAttachmentInfo(
	c *tc.C,
	uuid storageprovisioning.VolumeAttachmentUUID,
	blockDeviceUUID string,
	readOnly bool,
) {
	_, err := s.DB().Exec(
		`UPDATE storage_volume_attachment SET block_device_uuid=?, read_only=? WHERE uuid=?`,
		blockDeviceUUID, readOnly, uuid)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageStatusSuite) changeVolumeInfo(
	c *tc.C,
	uuid storageprovisioning.VolumeUUID,
	providerID string,
	sizeMiB uint64,
	hardwareID string,
	wwn string,
	persistent bool,
) {
	_, err := s.DB().Exec(
		`UPDATE storage_volume SET provider_id=?, size_mib=?, hardware_id=?, wwn=?, persistent=? WHERE uuid=?`,
		providerID, sizeMiB, hardwareID, wwn, persistent, uuid)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageStatusSuite) changeFilesystemInfo(
	c *tc.C,
	uuid storageprovisioning.FilesystemUUID,
	providerID string,
	sizeMiB uint64,
) {
	_, err := s.DB().Exec(
		`UPDATE storage_filesystem SET provider_id=?, size_mib=? WHERE uuid=?`,
		providerID, sizeMiB, uuid)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageStatusSuite) changeFilesystemAttachmentInfo(
	c *tc.C,
	uuid storageprovisioning.FilesystemAttachmentUUID,
	mountPoint string,
	readOnly bool,
) {
	_, err := s.DB().Exec(
		`UPDATE storage_filesystem_attachment SET mount_point=?, read_only=? WHERE uuid=?`,
		mountPoint, readOnly, uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// newFilesystem creates a new filesystem in the model with model
// provision scope. Return is the uuid and filesystem id of the entity.

// newFilesystemAttachment creates a new filesystem attachment that has
// model provision scope. The attachment is associated with the provided
// filesystem uuid and net node uuid.
func (s *storageStatusSuite) newFilesystemAttachment(
	c *tc.C,
	fsUUID storageprovisioning.FilesystemUUID,
	netNodeUUID domainnetwork.NetNodeUUID,
) storageprovisioning.FilesystemAttachmentUUID {
	attachmentUUID := storageprovisioningtesting.GenFilesystemAttachmentUUID(c)

	_, err := s.DB().Exec(`
INSERT INTO storage_filesystem_attachment (uuid,
                                           storage_filesystem_uuid,
                                           net_node_uuid,
                                           life_id,
                                           provision_scope_id)
VALUES (?, ?, ?, 0, 0)
`,
		attachmentUUID.String(), fsUUID, netNodeUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID
}

// changeStorageInstanceLife is a utility function for updating the life
// value of a storage instance.
func (s *storageStatusSuite) changeStorageInstanceLife(
	c *tc.C, uuid string, life life.Life,
) {
	_, err := s.DB().Exec(`
UPDATE storage_instance
SET    life_id=?
WHERE  uuid=?
`,
		int(life), uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// newApplication creates a new application in the model returning the uuid of
// the new application.
func (s *storageStatusSuite) newApplication(c *tc.C, name string, charmUUID corecharm.ID) string {
	appUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(`
INSERT INTO application (uuid, charm_uuid, name, life_id, space_uuid)
VALUES (?, ?, ?, "0", ?)`, appUUID.String(), charmUUID.String(), name, network.AlphaSpaceId)
	c.Assert(err, tc.ErrorIsNil)

	return appUUID.String()
}

// newMachineWithNetNode creates a new machine in the model attached to the
// supplied net node. The newly created machines uuid is returned along with the
// name.
func (s *storageStatusSuite) newMachineWithNetNode(
	c *tc.C, netNodeUUID domainnetwork.NetNodeUUID,
) (machine.UUID, machine.Name) {
	machineUUID := machinetesting.GenUUID(c)
	name := "mfoo-" + machineUUID.String()

	_, err := s.DB().Exec(
		"INSERT INTO machine (uuid, name, net_node_uuid, life_id) VALUES (?, ?, ?, 0)",
		machineUUID.String(),
		name,
		netNodeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return machineUUID, machine.Name(name)
}

// newVolumeAttachment creates a new volume attachment that has
// model provision scope. The attachment is associated with the provided
// volume uuid and net node uuid.
func (s *storageStatusSuite) newVolumeAttachment(
	c *tc.C,
	vsUUID storageprovisioning.VolumeUUID,
	netNodeUUID domainnetwork.NetNodeUUID,
) storageprovisioning.VolumeAttachmentUUID {
	attachmentUUID := storageprovisioningtesting.GenVolumeAttachmentUUID(c)

	_, err := s.DB().Exec(`
INSERT INTO storage_volume_attachment (uuid,
                                       storage_volume_uuid,
                                       net_node_uuid,
                                       life_id,
                                       provision_scope_id)
VALUES (?, ?, ?, 0, 0)
`,
		attachmentUUID.String(), vsUUID.String(), netNodeUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID
}

// newNetNode creates a new net node in the model for referencing to storage
// entity attachments. The net node is not associated with any machine or units.
func (s *storageStatusSuite) newNetNode(c *tc.C) domainnetwork.NetNodeUUID {
	nodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(
		"INSERT INTO net_node VALUES (?)",
		nodeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return nodeUUID
}

func (s *storageStatusSuite) newStorageUnitOwner(c *tc.C, storageInstanceUUID storage.StorageInstanceUUID, unitUUID unit.UUID) {
	_, err := s.DB().Exec(`
INSERT INTO storage_unit_owner(storage_instance_uuid, unit_uuid)
VALUES (?, ?)
`,
		storageInstanceUUID, unitUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageStatusSuite) newStorageAttachment(c *tc.C, storageInstanceUUID storage.StorageInstanceUUID, unitUUID unit.UUID) {
	saUUID := storageprovisioningtesting.GenStorageAttachmentUUID(c)
	_, err := s.DB().Exec(`
INSERT INTO storage_attachment(uuid, storage_instance_uuid, unit_uuid, life_id)
VALUES (?, ?, ?, 0)
`,
		saUUID.String(), storageInstanceUUID, unitUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
}

// newUnitWithNetNode creates a new unit in the model for the provided
// application uuid. The new unit will use the supplied net node. Returned is
// the new uuid of the unit and the name that was used.
func (s *storageStatusSuite) newUnitWithNetNode(
	c *tc.C, appUUID string, netNodeUUID domainnetwork.NetNodeUUID,
) (unit.UUID, unit.Name) {
	var charmUUID, appName string
	err := s.DB().QueryRow(
		"SELECT charm_uuid, name FROM application WHERE uuid=?",
		appUUID,
	).Scan(&charmUUID, &appName)
	c.Assert(err, tc.ErrorIsNil)

	unitUUID := unittesting.GenUnitUUID(c)
	unitID := fmt.Sprintf("%s/%d", appName, s.nextSequenceNumber(c, appName))

	_, err = s.DB().Exec(`
INSERT INTO unit (uuid, name, application_uuid, charm_uuid, net_node_uuid, life_id)
VALUES (?, ?, ?, ?, ?, 0)
`,
		unitUUID.String(), unitID, appUUID, charmUUID, netNodeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return unitUUID, unit.Name(unitID)
}
