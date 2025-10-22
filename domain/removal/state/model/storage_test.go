// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"database/sql"
	"testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/life"
	schematesting "github.com/juju/juju/domain/schema/testing"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type storageSuite struct {
	schematesting.ModelSuite
}

func TestStorageSuite(t *testing.T) {
	tc.Run(t, &storageSuite{})
}

func (s *storageSuite) TestStorageAttachmentExists(c *tc.C) {
	ctx := c.Context()

	_, attachment := s.addAppUnitStorage(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.StorageAttachmentExists(ctx, attachment)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, true)

	exists, err = st.StorageAttachmentExists(ctx, "not-today-henry")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *storageSuite) TestEnsureStorageAttachmentNotAliveSuccess(c *tc.C) {
	_, attachment := s.addAppUnitStorage(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	ctx := c.Context()

	err := st.EnsureStorageAttachmentNotAlive(ctx, attachment)
	c.Assert(err, tc.ErrorIsNil)

	// Attachment had life "alive" and should now be "dying".
	row := s.DB().QueryRowContext(ctx, "SELECT life_id FROM storage_attachment WHERE uuid = ?", attachment)
	var lifeID int
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)

	// Idempotent. A "dying" attachment is a no-op. Life is unchanged.
	err = st.EnsureStorageAttachmentNotAlive(ctx, attachment)
	c.Assert(err, tc.ErrorIsNil)

	row = s.DB().QueryRow("SELECT life_id FROM storage_attachment WHERE uuid = ?", attachment)
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)
}

func (s *storageSuite) TestScheduleStorageAttachmentRemovalSuccess(c *tc.C) {
	_, attachment := s.addAppUnitStorage(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.StorageAttachmentScheduleRemoval(
		c.Context(), "removal-uuid", attachment, false, when,
	)
	c.Assert(err, tc.ErrorIsNil)

	// We should have a removal job scheduled immediately.
	row := s.DB().QueryRow(
		"SELECT removal_type_id, entity_uuid, force, scheduled_for FROM removal where uuid = ?",
		"removal-uuid",
	)
	var (
		removalTypeID int
		rUUID         string
		force         bool
		scheduledFor  time.Time
	)
	err = row.Scan(&removalTypeID, &rUUID, &force, &scheduledFor)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(removalTypeID, tc.Equals, 6)
	c.Check(rUUID, tc.Equals, attachment)
	c.Check(force, tc.Equals, false)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *storageSuite) TestGetStorageAttachentLifeSuccess(c *tc.C) {
	_, saUUID := s.addAppUnitStorage(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	l, err := st.GetStorageAttachmentLife(c.Context(), saUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(l, tc.Equals, life.Alive)
}

func (s *storageSuite) TestGetStorageAttachmentLifeNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, err := st.GetStorageAttachmentLife(c.Context(), "some-sa-uuid")
	c.Assert(err, tc.ErrorIs, storageerrors.StorageAttachmentNotFound)
}

func (s *storageSuite) TestDeleteStorageAttachmentSuccess(c *tc.C) {
	siUUID, saUUID := s.addAppUnitStorage(c)

	ctx := c.Context()

	err := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c)).DeleteStorageAttachment(ctx, saUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Attachment is gone.
	var dummy string
	row := s.DB().QueryRowContext(ctx, "SELECT uuid FROM storage_attachment WHERE uuid = ?", saUUID)
	c.Check(row.Scan(&dummy), tc.ErrorIs, sql.ErrNoRows)

	// The attached unit was the owner, so the owner record is gone.
	row = s.DB().QueryRowContext(
		ctx, "SELECT unit_uuid FROM storage_unit_owner WHERE storage_instance_uuid = ?", siUUID)
	c.Check(row.Scan(&dummy), tc.ErrorIs, sql.ErrNoRows)
}

func (s *storageSuite) TestGetVolumeLife(c *tc.C) {
	volUUID := s.addVolume(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	l, err := st.GetVolumeLife(c.Context(), volUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(l, tc.Equals, life.Alive)
}

func (s *storageSuite) TestGetVolumeLifeNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, err := st.GetVolumeLife(c.Context(), "some-vol-uuid")
	c.Assert(err, tc.ErrorIs, storageprovisioningerrors.VolumeNotFound)
}

func (s *storageSuite) TestVolumeScheduleRemovalSuccess(c *tc.C) {
	volUUID := s.addVolume(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.VolumeScheduleRemoval(
		c.Context(), "removal-uuid", volUUID, false, when,
	)
	c.Assert(err, tc.ErrorIsNil)

	// We should have a removal job scheduled immediately.
	row := s.DB().QueryRow(
		"SELECT removal_type_id, entity_uuid, force, scheduled_for FROM removal where uuid = ?",
		"removal-uuid",
	)
	var (
		removalTypeID int
		rUUID         string
		force         bool
		scheduledFor  time.Time
	)
	err = row.Scan(&removalTypeID, &rUUID, &force, &scheduledFor)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(removalTypeID, tc.Equals, 7)
	c.Check(rUUID, tc.Equals, volUUID)
	c.Check(force, tc.Equals, false)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *storageSuite) TestDeleteVolume(c *tc.C) {
	volUUID := s.addVolume(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteVolume(c.Context(), volUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Volume is gone.
	var dummy string
	row := s.DB().QueryRow(
		"SELECT uuid FROM storage_volume WHERE uuid = ?", volUUID)
	c.Check(row.Scan(&dummy), tc.ErrorIs, sql.ErrNoRows)
}

func (s *storageSuite) TestDeleteVolumeWithInstance(c *tc.C) {
	siUUID, _ := s.addAppUnitStorage(c)
	volUUID := s.addVolume(c)
	s.bindVolumeToStorageInstance(c, siUUID, volUUID)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteVolume(c.Context(), volUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Volume is gone.
	var dummy string
	row := s.DB().QueryRow(
		"SELECT uuid FROM storage_volume WHERE uuid = ?", volUUID)
	c.Check(row.Scan(&dummy), tc.ErrorIs, sql.ErrNoRows)
	// Storage instance volume is gone.
	row = s.DB().QueryRow(
		"SELECT storage_volume_uuid FROM storage_instance_volume WHERE storage_volume_uuid = ?",
		volUUID)
	c.Check(row.Scan(&dummy), tc.ErrorIs, sql.ErrNoRows)
}

func (s *storageSuite) TestGetFilesystemLife(c *tc.C) {
	fsUUID := s.addFilesystem(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	l, err := st.GetFilesystemLife(c.Context(), fsUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(l, tc.Equals, life.Alive)
}

func (s *storageSuite) TestGetFilesystemLifeNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, err := st.GetFilesystemLife(c.Context(), "some-vol-uuid")
	c.Assert(err, tc.ErrorIs, storageprovisioningerrors.FilesystemNotFound)
}

func (s *storageSuite) TestFilesystemScheduleRemovalSuccess(c *tc.C) {
	fsUUID := s.addFilesystem(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.FilesystemScheduleRemoval(
		c.Context(), "removal-uuid", fsUUID, false, when,
	)
	c.Assert(err, tc.ErrorIsNil)

	// We should have a removal job scheduled immediately.
	row := s.DB().QueryRow(
		"SELECT removal_type_id, entity_uuid, force, scheduled_for FROM removal where uuid = ?",
		"removal-uuid",
	)
	var (
		removalTypeID int
		rUUID         string
		force         bool
		scheduledFor  time.Time
	)
	err = row.Scan(&removalTypeID, &rUUID, &force, &scheduledFor)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(removalTypeID, tc.Equals, 8)
	c.Check(rUUID, tc.Equals, fsUUID)
	c.Check(force, tc.Equals, false)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *storageSuite) TestDeleteFilesystem(c *tc.C) {
	fsUUID := s.addFilesystem(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteFilesystem(c.Context(), fsUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Filesystem is gone.
	var dummy string
	row := s.DB().QueryRow(
		"SELECT uuid FROM storage_filesystem WHERE uuid = ?", fsUUID)
	c.Check(row.Scan(&dummy), tc.ErrorIs, sql.ErrNoRows)
}

func (s *storageSuite) TestDeleteFilesystemWithInstance(c *tc.C) {
	siUUID, _ := s.addAppUnitStorage(c)
	fsUUID := s.addFilesystem(c)
	s.bindFilesystemToStorageInstance(c, siUUID, fsUUID)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteFilesystem(c.Context(), fsUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Filesystem is gone.
	var dummy string
	row := s.DB().QueryRow(
		"SELECT uuid FROM storage_filesystem WHERE uuid = ?", fsUUID)
	c.Check(row.Scan(&dummy), tc.ErrorIs, sql.ErrNoRows)
	// Storage instance filesystem is gone.
	row = s.DB().QueryRow(
		"SELECT storage_filesystem_uuid FROM storage_instance_filesystem WHERE storage_filesystem_uuid = ?",
		fsUUID)
	c.Check(row.Scan(&dummy), tc.ErrorIs, sql.ErrNoRows)
}

// addAppUnitStorage sets up a unit with a storage attachment.
// The storage instance and attachment UUIDs are returned.
func (s *storageSuite) addAppUnitStorage(c *tc.C) (string, string) {
	ctx := c.Context()

	charm := "some-charm-uuid"
	_, err := s.DB().ExecContext(ctx, "INSERT INTO charm (uuid, reference_name, architecture_id) VALUES (?, ?, ?)",
		charm, charm, 0)
	c.Assert(err, tc.ErrorIsNil)

	app := "some-app-uuid"
	_, err = s.DB().ExecContext(
		ctx, "INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, ?, ?, ?)",
		app, app, 0, charm, network.AlphaSpaceId,
	)
	c.Assert(err, tc.ErrorIsNil)

	node := "some-net-node-uuid"
	_, err = s.DB().ExecContext(ctx, "INSERT INTO net_node (uuid) VALUES (?)", node)
	c.Assert(err, tc.ErrorIsNil)

	unit := "some-unit-uuid"
	_, err = s.DB().ExecContext(
		ctx,
		"INSERT INTO unit (uuid, name, life_id, application_uuid, charm_uuid, net_node_uuid) VALUES (?, ?, ?, ?, ?, ?)",
		unit, unit, 0, app, charm, node)
	c.Assert(err, tc.ErrorIsNil)

	storagePool := "some-storage-pool-uuid"
	_, err = s.DB().ExecContext(ctx, "INSERT INTO storage_pool (uuid, name, type) VALUES (?, ?, ?)",
		storagePool, "loop", "loop")
	c.Assert(err, tc.ErrorIsNil)

	storageInstance := "some-storage-instance-uuid"
	_, err = s.DB().Exec(`
INSERT INTO storage_instance (
    uuid, storage_id, storage_kind_id, storage_pool_uuid, requested_size_mib,
    charm_name, storage_name, life_id
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		storageInstance, charm+"/0", 1, storagePool, 100, charm, "storage", 0)
	c.Assert(err, tc.ErrorIsNil)

	storageAttachment := "some-storage-attachment-uuid"
	_, err = s.DB().ExecContext(ctx,
		"INSERT INTO storage_attachment (uuid, storage_instance_uuid, unit_uuid, life_id) VALUES (?, ?, ?, ?)",
		storageAttachment, storageInstance, unit, 0)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(ctx,
		"INSERT INTO storage_unit_owner (storage_instance_uuid, unit_uuid) VALUES (?, ?)", storageInstance, unit)
	c.Assert(err, tc.ErrorIsNil)

	return storageInstance, storageAttachment
}

func (s *storageSuite) addVolume(c *tc.C) string {
	ctx := c.Context()

	volUUID := "some-vol-uuid"
	_, err := s.DB().ExecContext(ctx,
		"INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id) VALUES (?, ?, ?, ?)",
		volUUID, "some-vol", 0, 0,
	)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(ctx,
		"INSERT INTO storage_volume_status (volume_uuid, status_id) VALUES (?, ?)",
		volUUID, 0,
	)
	c.Assert(err, tc.ErrorIsNil)

	return volUUID
}

func (s *storageSuite) bindVolumeToStorageInstance(c *tc.C, siUUID, volUUID string) {
	ctx := c.Context()

	_, err := s.DB().ExecContext(ctx,
		"INSERT INTO storage_instance_volume (storage_instance_uuid, storage_volume_uuid) VALUES (?, ?)",
		siUUID, volUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) addFilesystem(c *tc.C) string {
	ctx := c.Context()

	fsUUID := "some-fs-uuid"
	_, err := s.DB().ExecContext(ctx,
		"INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provision_scope_id) VALUES (?, ?, ?, ?)",
		fsUUID, "some-fs", 0, 0,
	)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(ctx,
		"INSERT INTO storage_filesystem_status (filesystem_uuid, status_id) VALUES (?, ?)",
		fsUUID, 0,
	)
	c.Assert(err, tc.ErrorIsNil)

	return fsUUID
}

func (s *storageSuite) bindFilesystemToStorageInstance(c *tc.C, siUUID, fsUUID string) {
	ctx := c.Context()

	_, err := s.DB().ExecContext(ctx,
		"INSERT INTO storage_instance_filesystem (storage_instance_uuid, storage_filesystem_uuid) VALUES (?, ?)",
		siUUID, fsUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
}
