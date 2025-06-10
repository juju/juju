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

	"github.com/juju/clock"
	"github.com/juju/tc"

	corecharm "github.com/juju/juju/core/charm"
	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/semversion"
	corestorage "github.com/juju/juju/core/storage"
	storagetesting "github.com/juju/juju/core/storage/testing"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/life"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/status"
	statuserrors "github.com/juju/juju/domain/status/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type storageSuite struct {
	schematesting.ModelSuite

	state *State

	storageInstCount int
	filesystemCount  int
	volumeCount      int
}

func TestStorageSuite(t *testing.T) {
	tc.Run(t, &storageSuite{})
}

func (s *storageSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

type charmStorageArg struct {
	name     string
	kind     domainstorage.StorageKind
	min, max int
	readOnly bool
	location string
}

var (
	filesystemStorage = charmStorageArg{
		name:     "pgdata",
		kind:     domainstorage.StorageKindFilesystem,
		min:      1,
		max:      2,
		readOnly: true,
		location: "/tmp",
	}
	blockStorage = charmStorageArg{
		name:     "pgblock",
		kind:     domainstorage.StorageKindBlock,
		min:      1,
		max:      2,
		readOnly: true,
		location: "/dev/block",
	}
)

func (s *storageSuite) createFilesystem(c *tc.C) corestorage.FilesystemUUID {
	filesystemUUID := storagetesting.GenFilesystemUUID(c)

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

func (s *storageSuite) createFilesystemNoStatus(c *tc.C) corestorage.FilesystemUUID {
	filesystemUUID := storagetesting.GenFilesystemUUID(c)

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

func (s *storageSuite) createStorageInstance(c *tc.C, storageName, charmUUID corecharm.ID) corestorage.UUID {
	ctx := c.Context()
	storageUUID := storagetesting.GenStorageUUID(c)
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_instance (
    uuid, charm_uuid, storage_name, storage_id,
    life_id, requested_size_mib, storage_type
) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			storageUUID, charmUUID, storageName,
			fmt.Sprintf("%s/%d", storageName, s.storageInstCount),
			life.Alive, 100, "pool")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	s.storageInstCount++
	return storageUUID
}

func (s *storageSuite) createFilesystemInstance(c *tc.C, filesystemUUID corestorage.FilesystemUUID) {
	charmUUID := s.insertCharmWithStorage(c, filesystemStorage)
	storageUUID := s.createStorageInstance(c, "pgdata", charmUUID)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_instance_filesystem(storage_filesystem_uuid, storage_instance_uuid)
VALUES (?, ?)`, filesystemUUID, storageUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) assertFilesystemStatus(c *tc.C, filesystemUUID corestorage.FilesystemUUID, expected status.StatusInfo[status.StorageFilesystemStatusType]) {
	ctx := c.Context()

	var got status.StatusInfo[status.StorageFilesystemStatusType]
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
SELECT status_id, message, updated_at FROM storage_filesystem_status
WHERE filesystem_uuid = ?`, filesystemUUID).Scan(
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

func (s *storageSuite) createVolume(c *tc.C) corestorage.VolumeUUID {
	ctx := c.Context()

	volumeUUID := storagetesting.GenVolumeUUID(c)

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

func (s *storageSuite) createVolumeNoStatus(c *tc.C) corestorage.VolumeUUID {
	ctx := c.Context()

	volumeUUID := storagetesting.GenVolumeUUID(c)

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

func (s *storageSuite) createVolumeInstance(c *tc.C, volumeUUID corestorage.VolumeUUID) {
	charmUUID := s.insertCharmWithStorage(c, blockStorage)
	storageUUID := s.createStorageInstance(c, "pgblock", charmUUID)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_instance_volume(storage_volume_uuid, storage_instance_uuid)
VALUES (?, ?)`, volumeUUID, storageUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) assertVolumeStatus(c *tc.C, volumeUUID corestorage.VolumeUUID, expected status.StatusInfo[status.StorageVolumeStatusType]) {
	ctx := c.Context()

	var got status.StatusInfo[status.StorageVolumeStatusType]
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
SELECT status_id, message, updated_at FROM storage_volume_status
WHERE volume_uuid = ?`, volumeUUID).Scan(
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

	err := s.state.SetFilesystemStatus(c.Context(), filesystemUUID, expected)
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

	err := s.state.SetFilesystemStatus(c.Context(), filesystemUUID, expected)
	c.Assert(err, tc.ErrorIsNil)
	s.assertFilesystemStatus(c, filesystemUUID, expected)
}

func (s *storageSuite) TestSetFilesystemStatusMultipleTimes(c *tc.C) {
	filesystemUUID := s.createFilesystem(c)

	err := s.state.SetFilesystemStatus(c.Context(), filesystemUUID, status.StatusInfo[status.StorageFilesystemStatusType]{
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

	err = s.state.SetFilesystemStatus(c.Context(), filesystemUUID, expected)
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

	uuid := storagetesting.GenFilesystemUUID(c)
	err := s.state.SetFilesystemStatus(c.Context(), uuid, expected)
	c.Assert(err, tc.ErrorIs, storageerrors.FilesystemNotFound)
}

func (s *storageSuite) TestSetFilesystemStatusInvalidStatus(c *tc.C) {
	filesystemUUID := s.createFilesystem(c)

	expected := status.StatusInfo[status.StorageFilesystemStatusType]{
		Status: status.StorageFilesystemStatusType(99),
	}

	err := s.state.SetFilesystemStatus(c.Context(), filesystemUUID, expected)
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
	err := s.state.SetFilesystemStatus(c.Context(), filesystemUUID, sts)
	c.Assert(err, tc.ErrorIsNil)

	sts = status.StatusInfo[status.StorageFilesystemStatusType]{
		Status: status.StorageFilesystemStatusTypePending,
		Since:  ptr(now),
	}
	err = s.state.SetFilesystemStatus(c.Context(), filesystemUUID, sts)
	c.Assert(err, tc.ErrorIs, statuserrors.FilesystemStatusTransitionNotValid)
}

func (s *storageSuite) TestImportFilesystemStatus(c *tc.C) {
	filesystemUUID := s.createFilesystem(c)

	now := time.Now().UTC()
	expected := status.StatusInfo[status.StorageFilesystemStatusType]{
		Status:  status.StorageFilesystemStatusTypeAttached,
		Message: "message",
		Since:   ptr(now),
	}

	id := strconv.Itoa(s.filesystemCount - 1)
	err := s.state.ImportFilesystemStatus(c.Context(), id, expected)
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

	err := s.state.SetVolumeStatus(c.Context(), volumeUUID, expected)
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

	err := s.state.SetVolumeStatus(c.Context(), volumeUUID, expected)
	c.Assert(err, tc.ErrorIsNil)
	s.assertVolumeStatus(c, volumeUUID, expected)
}

func (s *storageSuite) TestSetVolumeStatusMultipleTimes(c *tc.C) {
	volumeUUID := s.createVolume(c)

	err := s.state.SetVolumeStatus(c.Context(), volumeUUID, status.StatusInfo[status.StorageVolumeStatusType]{
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

	err = s.state.SetVolumeStatus(c.Context(), volumeUUID, expected)
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

	uuid := storagetesting.GenVolumeUUID(c)
	err := s.state.SetVolumeStatus(c.Context(), uuid, expected)
	c.Assert(err, tc.ErrorIs, storageerrors.VolumeNotFound)
}

func (s *storageSuite) TestSetVolumeStatusInvalidStatus(c *tc.C) {
	volumeUUID := s.createVolume(c)

	expected := status.StatusInfo[status.StorageVolumeStatusType]{
		Status: status.StorageVolumeStatusType(99),
	}

	err := s.state.SetVolumeStatus(c.Context(), volumeUUID, expected)
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
	err := s.state.SetVolumeStatus(c.Context(), volumeUUID, sts)
	c.Assert(err, tc.ErrorIsNil)

	sts = status.StatusInfo[status.StorageVolumeStatusType]{
		Status: status.StorageVolumeStatusTypePending,
		Since:  ptr(now),
	}
	err = s.state.SetVolumeStatus(c.Context(), volumeUUID, sts)
	c.Assert(err, tc.ErrorIs, statuserrors.VolumeStatusTransitionNotValid)
}

func (s *storageSuite) TestImportVolumeStatus(c *tc.C) {
	volumeUUID := s.createVolume(c)

	now := time.Now().UTC()
	expected := status.StatusInfo[status.StorageVolumeStatusType]{
		Status:  status.StorageVolumeStatusTypeAttached,
		Message: "message",
		Since:   ptr(now),
	}

	id := strconv.Itoa(s.volumeCount - 1)
	err := s.state.ImportVolumeStatus(c.Context(), id, expected)
	c.Assert(err, tc.ErrorIsNil)
	s.assertVolumeStatus(c, volumeUUID, expected)
}
