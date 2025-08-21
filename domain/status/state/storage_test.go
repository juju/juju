// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/tc"

	corecharm "github.com/juju/juju/core/charm"
	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domainsequence "github.com/juju/juju/domain/sequence"
	sequencestate "github.com/juju/juju/domain/sequence/state"
	"github.com/juju/juju/domain/status"
	statuserrors "github.com/juju/juju/domain/status/errors"
	"github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	storagetesting "github.com/juju/juju/domain/storage/testing"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningtesting "github.com/juju/juju/domain/storageprovisioning/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type storageSuite struct {
	schematesting.ModelSuite

	modelState *ModelState

	storageInstCount int
	filesystemCount  int
	volumeCount      int
}

func TestStorageSuite(t *testing.T) {
	tc.Run(t, &storageSuite{})
}

func (s *storageSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	s.modelState = NewModelState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

type charmStorageArg struct {
	name     string
	kind     storage.StorageKind
	min, max int
	readOnly bool
	location string
}

var (
	filesystemStorage = charmStorageArg{
		name:     "pgdata",
		kind:     storage.StorageKindFilesystem,
		min:      1,
		max:      2,
		readOnly: true,
		location: "/tmp",
	}
	blockStorage = charmStorageArg{
		name:     "pgblock",
		kind:     storage.StorageKindBlock,
		min:      1,
		max:      2,
		readOnly: true,
		location: "/dev/block",
	}
)

func (s *storageSuite) createFilesystem(c *tc.C) storageprovisioning.FilesystemUUID {
	filesystemUUID := storageprovisioningtesting.GenFilesystemUUID(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_filesystem(uuid, life_id, filesystem_id)
VALUES (?, ?, ?)`, filesystemUUID, life.Alive, s.filesystemCount)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO storage_filesystem_status(filesystem_uuid, status_id)
VALUES (?, ?)`, filesystemUUID, 0)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	s.filesystemCount++
	return filesystemUUID
}

func (s *storageSuite) createFilesystemNoStatus(c *tc.C) storageprovisioning.FilesystemUUID {
	filesystemUUID := storageprovisioningtesting.GenFilesystemUUID(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_filesystem(uuid, life_id, filesystem_id)
VALUES (?, ?, ?)`, filesystemUUID, life.Alive, s.filesystemCount)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	s.filesystemCount++
	return filesystemUUID
}

func insertCharmState(c *tc.C, tx *sql.Tx, uuid corecharm.ID) error {
	return insertCharmStateWithRevision(c, tx, uuid, 42)
}

func insertCharmStateWithRevision(c *tc.C, tx *sql.Tx, uuid corecharm.ID, revision int) error {
	_, err := tx.ExecContext(c.Context(), `
INSERT INTO charm (uuid, archive_path, available, reference_name, revision, version, architecture_id)
VALUES (?, 'archive', false, 'ubuntu', ?, 'deadbeef', 0)
`, uuid, revision)
	if err != nil {
		return errors.Capture(err)
	}

	_, err = tx.ExecContext(c.Context(), `
INSERT INTO charm_metadata (charm_uuid, name, description, summary, subordinate, min_juju_version, run_as_id, assumes)
VALUES (?, 'ubuntu', 'description', 'summary', true, '4.0.0', 1, 'null')`, uuid)
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

func insertCharmMetadata(c *tc.C, tx *sql.Tx, uuid corecharm.ID) (charm.Metadata, error) {
	if err := insertCharmState(c, tx, uuid); err != nil {
		return charm.Metadata{}, errors.Capture(err)
	}

	return charm.Metadata{
		Name:           "ubuntu",
		Summary:        "summary",
		Description:    "description",
		Subordinate:    true,
		RunAs:          charm.RunAsRoot,
		MinJujuVersion: semversion.MustParse("4.0.0"),
		Assumes:        []byte("null"),
	}, nil
}

func (s *storageSuite) insertCharmWithStorage(c *tc.C, stor ...charmStorageArg) corecharm.ID {
	uuid := charmtesting.GenCharmID(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		var err error
		if _, err = insertCharmMetadata(c, tx, uuid); err != nil {
			return errors.Capture(err)
		}

		for _, arg := range stor {
			_, err = tx.ExecContext(ctx, `
INSERT INTO charm_storage (
    charm_uuid,
    name,
    storage_kind_id,
    read_only,
    count_min,
    count_max,
    location
) VALUES
    (?, ?, ?, ?, ?, ?, ?);`,
				uuid, arg.name, arg.kind, arg.readOnly, arg.min, arg.max, arg.location)
			if err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	return uuid
}

func (s *storageSuite) createStorageInstance(c *tc.C, storageName, charmUUID corecharm.ID, poolUUID storage.StoragePoolUUID) storage.StorageInstanceUUID {
	storageUUID := storagetesting.GenStorageInstanceUUID(c)

	_, err := s.DB().Exec(`
INSERT INTO storage_instance (
    uuid, charm_uuid, storage_name, storage_id,
    life_id, requested_size_mib, storage_pool_uuid
) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		storageUUID, charmUUID, storageName,
		fmt.Sprintf("%s/%d", storageName, s.storageInstCount),
		life.Alive, 100, poolUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	s.storageInstCount++
	return storageUUID
}

// newStoragePool creates a new storage pool with name, provider type and attrs.
// It returns the UUID of the new storage pool.
func (s *storageSuite) newStoragePool(c *tc.C, name string, providerType string, attrs map[string]string) storage.StoragePoolUUID {
	spUUID := storagetesting.GenStoragePoolUUID(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_pool (uuid, name, type)
VALUES (?, ?, ?)`, spUUID.String(), name, providerType)
		if err != nil {
			return err
		}

		for k, v := range attrs {
			_, err = tx.ExecContext(ctx, `
INSERT INTO storage_pool_attribute (storage_pool_uuid, key, value)
VALUES (?, ?, ?)`, spUUID.String(), k, v)
			if err != nil {
				return err
			}
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	return spUUID
}

func (s *storageSuite) createFilesystemInstance(
	c *tc.C,
	filesystemUUID storageprovisioning.FilesystemUUID,
) {
	charmUUID := s.insertCharmWithStorage(c, filesystemStorage)
	poolUUID := s.newStoragePool(c, "pool", "pool", nil)
	storageUUID := s.createStorageInstance(c, "pgdata", charmUUID, poolUUID)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_instance_filesystem(storage_filesystem_uuid, storage_instance_uuid)
VALUES (?, ?)`, filesystemUUID, storageUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
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

func (s *storageSuite) createVolume(c *tc.C) storageprovisioning.VolumeUUID {
	ctx := c.Context()

	volumeUUID := storageprovisioningtesting.GenVolumeUUID(c)

	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_volume(uuid, life_id, volume_id)
VALUES (?, ?, ?)`, volumeUUID, life.Alive, s.volumeCount)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO storage_volume_status(volume_uuid, status_id)
VALUES (?, ?)`, volumeUUID, 0)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	s.volumeCount++
	return volumeUUID
}

func (s *storageSuite) createVolumeNoStatus(c *tc.C) storageprovisioning.VolumeUUID {
	ctx := c.Context()

	volumeUUID := storageprovisioningtesting.GenVolumeUUID(c)

	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_volume(uuid, life_id, volume_id)
VALUES (?, ?, ?)`, volumeUUID, life.Alive, s.volumeCount)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	s.volumeCount++
	return volumeUUID
}

func (s *storageSuite) createVolumeInstance(c *tc.C, volumeUUID storageprovisioning.VolumeUUID) {
	charmUUID := s.insertCharmWithStorage(c, blockStorage)
	poolUUID := s.newStoragePool(c, "blockpool", "blockpool", nil)
	storageUUID := s.createStorageInstance(c, "pgblock", charmUUID, poolUUID)

	_, err := s.DB().Exec(`
INSERT INTO storage_instance_volume(storage_volume_uuid, storage_instance_uuid)
VALUES (?, ?)`, volumeUUID, storageUUID)
	c.Assert(err, tc.ErrorIsNil)
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
	filesystemUUID := s.createFilesystem(c)

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
	filesystemUUID := s.createFilesystemNoStatus(c)

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
	filesystemUUID := s.createFilesystem(c)

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
	filesystemUUID := s.createFilesystem(c)

	expected := status.StatusInfo[status.StorageFilesystemStatusType]{
		Status: status.StorageFilesystemStatusType(99),
	}

	err := s.modelState.SetFilesystemStatus(c.Context(), filesystemUUID, expected)
	c.Assert(err, tc.ErrorMatches, `.*unknown status.*`)
}

func (s *storageSuite) TestSetFilesystemStatusInvalidTransition(c *tc.C) {
	filesystemUUID := s.createFilesystem(c)
	now := time.Now().UTC()
	s.createFilesystemInstance(c, filesystemUUID)

	sts := status.StatusInfo[status.StorageFilesystemStatusType]{
		Status: status.StorageFilesystemStatusTypeAttached,
		Since:  ptr(now),
	}
	err := s.modelState.SetFilesystemStatus(c.Context(), filesystemUUID, sts)
	c.Assert(err, tc.ErrorIsNil)

	sts = status.StatusInfo[status.StorageFilesystemStatusType]{
		Status: status.StorageFilesystemStatusTypePending,
		Since:  ptr(now),
	}
	err = s.modelState.SetFilesystemStatus(c.Context(), filesystemUUID, sts)
	c.Assert(err, tc.ErrorIs, statuserrors.FilesystemStatusTransitionNotValid)
}

func (s *storageSuite) TestGetFilesystemUUIDByID(c *tc.C) {
	filesystemUUID := s.createFilesystem(c)

	id := strconv.Itoa(s.filesystemCount - 1)
	gotUUID, err := s.modelState.GetFilesystemUUIDByID(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(gotUUID, tc.Equals, filesystemUUID)
}

func (s *storageSuite) TestGetFilesystemUUIDByIDNotFound(c *tc.C) {
	_, err := s.modelState.GetFilesystemUUIDByID(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, storageerrors.FilesystemNotFound)
}

func (s *storageSuite) TestImportFilesystemStatus(c *tc.C) {
	filesystemUUID := s.createFilesystem(c)

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
	volumeUUID := s.createVolume(c)

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
	volumeUUID := s.createVolumeNoStatus(c)

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
	volumeUUID := s.createVolume(c)

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
	volumeUUID := s.createVolume(c)

	expected := status.StatusInfo[status.StorageVolumeStatusType]{
		Status: status.StorageVolumeStatusType(99),
	}

	err := s.modelState.SetVolumeStatus(c.Context(), volumeUUID, expected)
	c.Assert(err, tc.ErrorMatches, `.*unknown status.*`)
}

func (s *storageSuite) TestSetVolumeStatusInvalidTransition(c *tc.C) {
	volumeUUID := s.createVolume(c)
	now := time.Now().UTC()
	s.createVolumeInstance(c, volumeUUID)

	sts := status.StatusInfo[status.StorageVolumeStatusType]{
		Status: status.StorageVolumeStatusTypeAttached,
		Since:  ptr(now),
	}
	err := s.modelState.SetVolumeStatus(c.Context(), volumeUUID, sts)
	c.Assert(err, tc.ErrorIsNil)

	sts = status.StatusInfo[status.StorageVolumeStatusType]{
		Status: status.StorageVolumeStatusTypePending,
		Since:  ptr(now),
	}
	err = s.modelState.SetVolumeStatus(c.Context(), volumeUUID, sts)
	c.Assert(err, tc.ErrorIs, statuserrors.VolumeStatusTransitionNotValid)
}

func (s *storageSuite) TestGetVolumeUUIDByID(c *tc.C) {
	volumeUUID := s.createVolume(c)

	id := strconv.Itoa(s.volumeCount - 1)
	gotUUID, err := s.modelState.GetVolumeUUIDByID(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(gotUUID, tc.Equals, volumeUUID)
}

func (s *storageSuite) TestGetVolumeUUIDByIDNotFound(c *tc.C) {
	_, err := s.modelState.GetVolumeUUIDByID(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, storageerrors.VolumeNotFound)
}

func (s *storageSuite) TestImportVolumeStatus(c *tc.C) {
	volumeUUID := s.createVolume(c)

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

type storageStatusSuite struct {
	schematesting.ModelSuite
}

func TestStorageStatusSuite(t *testing.T) {
	tc.Run(t, &storageStatusSuite{})
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
	s0, _ := s.newStorageInstance(c, ch0, "blk", blkPoolUUID)
	s.changeStorageInstanceLife(c, s0.String(), life.Dying)

	// Filesystem storage instance with an owning unit that is alive.
	s1, _ := s.newStorageInstance(c, ch0, "fs", fsPoolUUID)
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
			Kind: storage.StorageKindBlock,
			Life: life.Dying,
		},
		{
			UUID:  s1,
			ID:    "fs/1",
			Kind:  storage.StorageKindFilesystem,
			Life:  life.Alive,
			Owner: &u0n,
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
	s0, _ := s.newStorageInstance(c, ch0, "blk", blkPoolUUID)
	s.newStorageAttachment(c, s0, u0)

	// Storage instance attachment of a filesystem storage instance with a unit
	// attachment on a machine.
	a1 := s.newApplication(c, "bar", ch0)
	nn1 := s.newNetNode(c)
	_, m1n := s.newMachineWithNetNode(c, nn1)
	u1, u1n := s.newUnitWithNetNode(c, a1, nn1)
	s1, _ := s.newStorageInstance(c, ch0, "fs", fsPoolUUID)
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
	s0, s0id := s.newStorageInstance(c, ch0, "fs", fsPoolUUID)
	s.newStorageAttachment(c, s0, u0)
	f0, f0id := s.newFilesystem(c)
	s.changeFilesystemInfo(c, f0, "my-provider-id-1", 123)
	s.newStorageInstanceFilesystem(c, s0, f0)

	// Filesystem backed by a volume with size and provider id.
	s1, s1id := s.newStorageInstance(c, ch0, "fs", fsPoolUUID)
	f1, f1id := s.newFilesystem(c)
	s.changeFilesystemInfo(c, f1, "my-provider-id-2", 456)
	s.newStorageInstanceFilesystem(c, s1, f1)
	v1, v1id := s.newVolume(c)
	s.newStorageInstanceVolume(c, s1, v1)

	st := s.NewModelState(c)
	st.SetFilesystemStatus(c.Context(), f0, status.StatusInfo[status.StorageFilesystemStatusType]{
		Status:  status.StorageFilesystemStatusTypeAttaching,
		Message: "attaching the filez",
	})

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
	s0, _ := s.newStorageInstance(c, ch0, "fs", fsPoolUUID)
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
	s1, _ := s.newStorageInstance(c, ch0, "fs", fsPoolUUID)
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
	s0, s0id := s.newStorageInstance(c, ch0, "blk", blkPoolUUID)
	s.newStorageAttachment(c, s0, u0)
	v0, v0id := s.newVolume(c)
	s.changeVolumeInfo(c, v0, "my-provider-id-1", 123, "hw0", "wwn0", true)
	s.newStorageInstanceVolume(c, s0, v0)

	// Volume pending.
	a1 := s.newApplication(c, "bar", ch0)
	nn1 := s.newNetNode(c)
	u1, _ := s.newUnitWithNetNode(c, a1, nn1)
	s1, s1id := s.newStorageInstance(c, ch0, "blk", blkPoolUUID)
	s.newStorageAttachment(c, s1, u1)
	v1, v1id := s.newVolume(c)
	s.newStorageInstanceVolume(c, s1, v1)

	st := s.NewModelState(c)
	st.SetVolumeStatus(c.Context(), v0, status.StatusInfo[status.StorageVolumeStatusType]{
		Status:  status.StorageVolumeStatusTypeAttaching,
		Message: "attaching the volumez",
	})

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
	s0, _ := s.newStorageInstance(c, ch0, "blk", blkPoolUUID)
	s.newStorageAttachment(c, s0, u0)
	v0, _ := s.newVolume(c)
	s.newStorageInstanceVolume(c, s0, v0)
	s.newVolumeAttachment(c, v0, nn0)

	// Volume attachment to a unit and machine with no block device.
	a1 := s.newApplication(c, "bar", ch0)
	nn1 := s.newNetNode(c)
	_, m1n := s.newMachineWithNetNode(c, nn1)
	u1, u1n := s.newUnitWithNetNode(c, a1, nn1)
	s1, _ := s.newStorageInstance(c, ch0, "blk", blkPoolUUID)
	s.newStorageAttachment(c, s1, u1)
	v1, _ := s.newVolume(c)
	s.newStorageInstanceVolume(c, s1, v1)
	s.newVolumeAttachment(c, v1, nn1)

	// Volume attachment to a unit and machine with block device.
	a2 := s.newApplication(c, "baz", ch0)
	nn2 := s.newNetNode(c)
	m2, m2n := s.newMachineWithNetNode(c, nn2)
	u2, u2n := s.newUnitWithNetNode(c, a2, nn2)
	s2, _ := s.newStorageInstance(c, ch0, "blk", blkPoolUUID)
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
	s3, _ := s.newStorageInstance(c, ch0, "blk", blkPoolUUID)
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
		`INSERT INTO storage_volume_attachment_plan(uuid, storage_volume_uuid, net_node_uuid, life_id, device_type_id) VALUES(?, ?, ?, 0, ?)`,
		vapUUID, volumeUUID, netNodeUUID, deviceTypeID)
	c.Assert(err, tc.ErrorIsNil)
	for key, value := range attrs {
		_, err := s.DB().Exec(
			`INSERT INTO storage_volume_attachment_plan_attr(uuid, attachment_plan_uuid, key, value) VALUES(?, ?, ?, ?)`,
			uuid.MustNewUUID().String(), vapUUID, key, value)
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
			`INSERT INTO block_device_link_device(block_device_uuid, name) VALUES(?, ?)`,
			uuid, deviceLink)
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
func (s *storageStatusSuite) newFilesystem(c *tc.C) (
	storageprovisioning.FilesystemUUID, string,
) {
	fsUUID := storageprovisioningtesting.GenFilesystemUUID(c)

	fsID := fmt.Sprintf("foo/%s", fsUUID.String())

	_, err := s.DB().Exec(`
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 0)
	`,
		fsUUID.String(), fsID)
	c.Assert(err, tc.ErrorIsNil)

	return fsUUID, fsID
}

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
func (s *storageStatusSuite) newApplication(c *tc.C, name string, charmUUID string) string {
	appUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(`
INSERT INTO application (uuid, charm_uuid, name, life_id, space_uuid)
VALUES (?, ?, ?, "0", ?)`, appUUID.String(), charmUUID, name, network.AlphaSpaceId)
	c.Assert(err, tc.ErrorIsNil)

	return appUUID.String()
}

// newCharm creates a new charm in the model and returns the uuid for it.
func (s *storageStatusSuite) newCharm(c *tc.C) string {
	charmUUID := charmtesting.GenCharmID(c)
	_, err := s.DB().Exec(`
INSERT INTO charm (uuid, source_id, reference_name, revision, architecture_id)
VALUES (?, 0, ?, 1, 0)
`,
		charmUUID.String(), "foo-"+charmUUID[:4],
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(`
INSERT INTO charm_metadata (charm_uuid, name)
VALUES (?, 'myapp')
`,
		charmUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)
	return charmUUID.String()
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

// newVolume creates a new volume in the model with model
// provision scope. Return is the uuid and volume id of the entity.
func (s *storageStatusSuite) newVolume(c *tc.C) (storageprovisioning.VolumeUUID, string) {
	vsUUID := storageprovisioningtesting.GenVolumeUUID(c)

	vsID := fmt.Sprintf("foo/%s", vsUUID.String())

	_, err := s.DB().Exec(`
INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 0)
	`,
		vsUUID.String(), vsID)
	c.Assert(err, tc.ErrorIsNil)

	return vsUUID, vsID
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

func (s *storageStatusSuite) newCharmStorage(c *tc.C, charmUUID string, storageName string, kind storage.StorageKind) {
	_, err := s.DB().Exec(`
INSERT INTO charm_storage (charm_uuid, name, storage_kind_id, count_min, count_max)
VALUES (?, ?, ?, 0, 1)
`,
		charmUUID, storageName, kind,
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageStatusSuite) newStorageInstance(c *tc.C, charmUUID string, storageName string, poolUUID storage.StoragePoolUUID) (storage.StorageInstanceUUID, string) {
	storageInstanceUUID := storagetesting.GenStorageInstanceUUID(c)
	storageID := fmt.Sprintf("%s/%d", storageName, s.nextSequenceNumber(c, "storage"))

	_, err := s.DB().Exec(`
INSERT INTO storage_instance(uuid, charm_uuid, storage_name, storage_id, life_id, requested_size_mib, storage_pool_uuid)
VALUES (?, ?, ?, ?, 0, 100, ?)
`,
		storageInstanceUUID.String(),
		charmUUID,
		storageName,
		storageID,
		poolUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	return storageInstanceUUID, storageID
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
	_, err := s.DB().Exec(`
INSERT INTO storage_attachment(storage_instance_uuid, unit_uuid, life_id)
VALUES (?, ?, 0)
`,
		storageInstanceUUID, unitUUID,
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

// nextStorageSequenceNumber retrieves the next sequence number in the storage
// namespace.
func (s *storageStatusSuite) nextSequenceNumber(
	c *tc.C, namespace string,
) uint64 {
	var id uint64
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		id, err = sequencestate.NextValue(
			ctx, preparer{}, tx, domainsequence.StaticNamespace(namespace),
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return id
}

func (s *storageStatusSuite) newStorageInstanceVolume(
	c *tc.C, instanceUUID storage.StorageInstanceUUID,
	volumeUUID storageprovisioning.VolumeUUID,
) {
	_, err := s.DB().Exec(`
INSERT INTO storage_instance_volume (storage_instance_uuid, storage_volume_uuid)
VALUES (?, ?)`, instanceUUID.String(), volumeUUID.String())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageStatusSuite) newStorageInstanceFilesystem(
	c *tc.C, instanceUUID storage.StorageInstanceUUID,
	filesystemUUID storageprovisioning.FilesystemUUID,
) {
	_, err := s.DB().Exec(`
INSERT INTO storage_instance_filesystem (storage_instance_uuid, storage_filesystem_uuid)
VALUES (?, ?)`, instanceUUID.String(), filesystemUUID.String())
	c.Assert(err, tc.ErrorIsNil)
}

// newStoragePool creates a new storage pool with name, provider type and attrs.
// It returns the UUID of the new storage pool.
func (s *storageStatusSuite) newStoragePool(c *tc.C,
	name string, providerType string,
	attrs map[string]string,
) storage.StoragePoolUUID {
	spUUID := storagetesting.GenStoragePoolUUID(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_pool (uuid, name, type)
VALUES (?, ?, ?)`, spUUID.String(), name, providerType)
		if err != nil {
			return err
		}

		for k, v := range attrs {
			_, err = tx.ExecContext(ctx, `
INSERT INTO storage_pool_attribute (storage_pool_uuid, key, value)
VALUES (?, ?, ?)`, spUUID.String(), k, v)
			if err != nil {
				return err
			}
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	return spUUID
}

type preparer struct{}

func (p preparer) Prepare(query string, typeSamples ...any) (*sqlair.Statement, error) {
	return sqlair.Prepare(query, typeSamples...)
}
