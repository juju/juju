// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corecharm "github.com/juju/juju/core/charm"
	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/model/testing"
	corestorage "github.com/juju/juju/core/storage"
	storagetesting "github.com/juju/juju/core/storage/testing"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

// TestCreateApplicationWithResources tests creation of an application with
// specified resources.
// It verifies that the charm_resource table is populated, alongside the
// resource and application_resource table with datas from charm and arguments.
func (s *applicationStateSuite) TestCreateApplicationWithStorage(c *gc.C) {
	ctx := context.Background()
	uuid := uuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_pool (uuid, name, type) VALUES (?, ?, ?)`,
			uuid, "fast", "ebs")
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	chStorage := []charm.Storage{{
		Name: "database",
		Type: "block",
	}, {
		Name: "logs",
		Type: "filesystem",
	}, {
		Name: "cache",
		Type: "block",
	}}
	addStorageArgs := []application.ApplicationStorageArg{
		{
			Name:           "database",
			PoolNameOrType: "ebs",
			Size:           10,
			Count:          2,
		},
		{
			Name:           "logs",
			PoolNameOrType: "rootfs",
			Size:           20,
			Count:          1,
		},
		{
			Name:           "cache",
			PoolNameOrType: "fast",
			Size:           30,
			Count:          1,
		},
	}
	c.Assert(err, jc.ErrorIsNil)

	appUUID, err := s.state.CreateApplication(ctx, "666", s.addApplicationArgForStorage(c, "666",
		c.MkDir(), chStorage, addStorageArgs), nil)
	c.Assert(err, jc.ErrorIsNil)

	var charmUUID string
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
SELECT charm_uuid
FROM application
WHERE name=?`, "666").Scan(&charmUUID)
	})
	c.Assert(err, jc.ErrorIsNil)
	var (
		foundCharmStorage []charm.Storage
		foundAppStorage   []application.ApplicationStorageArg
		poolUUID          string
	)

	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT cs.name, csk.kind
FROM charm_storage cs
JOIN charm_storage_kind csk ON csk.id=cs.storage_kind_id
WHERE charm_uuid=?`, charmUUID)
		if err != nil {
			return errors.Capture(err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var stor charm.Storage
			if err := rows.Scan(&stor.Name, &stor.Type); err != nil {
				return errors.Capture(err)
			}
			foundCharmStorage = append(foundCharmStorage, stor)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT storage_name, storage_pool, size_mib, count
FROM v_application_storage_directive
WHERE application_uuid = ? AND charm_uuid = ?`, appUUID, charmUUID)
		if err != nil {
			return errors.Capture(err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var stor application.ApplicationStorageArg
			if err := rows.Scan(&stor.Name, &stor.PoolNameOrType, &stor.Size, &stor.Count); err != nil {
				return errors.Capture(err)
			}
			foundAppStorage = append(foundAppStorage, stor)
		}
		rows, err = tx.QueryContext(ctx, `
SELECT storage_pool_uuid
FROM application_storage_directive
WHERE storage_type IS NULL AND application_uuid = ? AND charm_uuid = ?`, appUUID, charmUUID)
		if err != nil {
			return errors.Capture(err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			if err := rows.Scan(&poolUUID); err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(foundCharmStorage, jc.SameContents, chStorage)
	c.Check(foundAppStorage, jc.SameContents, addStorageArgs)
	c.Assert(poolUUID, gc.Equals, uuid)
}

func (s *applicationStateSuite) TestCreateApplicationWithUnrecognisedStorage(c *gc.C) {
	chStorage := []charm.Storage{{
		Name: "database",
		Type: "block",
	}}
	addStorageArgs := []application.ApplicationStorageArg{{
		Name:           "foo",
		PoolNameOrType: "rootfs",
		Size:           20,
		Count:          1,
	}}
	ctx := context.Background()

	_, err := s.state.CreateApplication(ctx, "666", s.addApplicationArgForStorage(c, "666",
		c.MkDir(), chStorage, addStorageArgs), nil)
	c.Assert(err, gc.ErrorMatches, `.*storage \["foo"\] is not supported`)
}

func (s *applicationStateSuite) TestCreateApplicationWithStorageButCharmHasNone(c *gc.C) {
	addStorageArgs := []application.ApplicationStorageArg{{
		Name:           "foo",
		PoolNameOrType: "rootfs",
		Size:           20,
		Count:          1,
	}}
	ctx := context.Background()

	_, err := s.state.CreateApplication(ctx, "666", s.addApplicationArgForStorage(c, "666",
		c.MkDir(), []charm.Storage{}, addStorageArgs), nil)
	c.Assert(err, gc.ErrorMatches, `.*storage \["foo"\] is not supported`)
}

func (s *applicationStateSuite) TestCreateApplicationWithUnitsAndStorageInvalidCount(c *gc.C) {
	chStorage := []charm.Storage{{
		Name:     "database",
		Type:     "block",
		CountMin: 1,
		CountMax: 2,
	}}
	addStorageArgs := []application.ApplicationStorageArg{
		{
			Name:           "database",
			PoolNameOrType: "ebs",
			Size:           10,
			Count:          200,
		},
	}
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	ctx := context.Background()

	_, err := s.state.CreateApplication(ctx, "foo", s.addApplicationArgForStorage(c, "foo",
		c.MkDir(), chStorage, addStorageArgs), []application.AddUnitArg{u1})
	c.Assert(err, jc.ErrorIs, applicationerrors.InvalidStorageCount)

}

type baseStorageSuite struct {
	baseSuite

	state *State

	storageParentDir string

	storageInstCount int
	filesystemCount  int
}

func (s *baseStorageSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	s.storageParentDir = c.MkDir()
}

type charmStorageArg struct {
	name     string
	kind     domainstorage.StorageKind
	min, max int
	readOnly bool
	location string
}

func (s *baseStorageSuite) insertCharmWithStorage(c *gc.C, stor ...charmStorageArg) string {
	uuid := charmtesting.GenCharmID(c).String()

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var err error
		if _, err = insertCharmMetadata(ctx, c, tx, uuid); err != nil {
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
	c.Assert(err, jc.ErrorIsNil)
	return uuid
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

func (s *baseStorageSuite) TestGetStorageUUIDByID(c *gc.C) {
	ctx := context.Background()

	charmUUID := s.insertCharmWithStorage(c, filesystemStorage)
	uuid := storagetesting.GenStorageUUID(c)

	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_instance(uuid, charm_uuid, storage_name, storage_id, life_id, storage_type, requested_size_mib)
VALUES (?, ?, ?, ?, ?, ?, ?)`, uuid, charmUUID, "pgdata", "pgdata/0", 0, "rootfs", 666)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.state.GetStorageUUIDByID(ctx, "pgdata/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, uuid)
}

func (s *baseStorageSuite) TestGetStorageUUIDByIDNotFound(c *gc.C) {
	ctx := context.Background()

	_, err := s.state.GetStorageUUIDByID(ctx, "pgdata/0")
	c.Assert(err, jc.ErrorIs, storageerrors.StorageNotFound)
}

func (s *baseStorageSuite) createUnitWithCharm(c *gc.C, stor ...charmStorageArg) (coreunit.UUID, string) {
	ctx := context.Background()

	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u1)
	unitUUID, err := s.state.GetUnitUUIDByName(context.Background(), u1.UnitName)
	c.Assert(err, jc.ErrorIsNil)

	charmUUID := s.insertCharmWithStorage(c, stor...)

	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err = tx.ExecContext(ctx, `
UPDATE unit SET charm_uuid = ? WHERE unit.name = ?`, charmUUID, "foo/666")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	return unitUUID, charmUUID
}

func (s *baseStorageSuite) createStorageInstance(c *gc.C, storageName, charmUUID string, ownerUUID *coreunit.UUID) corestorage.UUID {
	ctx := context.Background()

	poolUUID := uuid.MustNewUUID().String()
	storageUUID := storagetesting.GenStorageUUID(c)
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_pool(uuid, name, type)
VALUES (?, ?, ?)
ON CONFLICT DO NOTHING`, poolUUID, "pool", "rootfs")
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO storage_instance(uuid, charm_uuid, storage_name, storage_id, life_id, requested_size_mib, storage_pool_uuid)
SELECT ?, ?, ?, ?, ?, ?, uuid FROM storage_pool WHERE name = ?`, storageUUID, charmUUID, storageName, fmt.Sprintf("%s/%d", storageName, s.storageInstCount), life.Alive, 100, "pool")
		if err != nil || ownerUUID == nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO storage_unit_owner(unit_uuid, storage_instance_uuid)
VALUES (?, ?)`, *ownerUUID, storageUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	s.storageInstCount = s.storageInstCount + 1
	return storageUUID
}

func (s *baseStorageSuite) assertStorageAttached(c *gc.C, unitUUID coreunit.UUID, storageUUID corestorage.UUID) {
	var (
		attachmentLife life.Life
	)
	// Check that the storage attachment row exists and that the charm
	// of the associated storage instance matches that of the unit.
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
SELECT sa.life_id FROM storage_attachment sa
-- JOIN storage_instance si ON si.uuid = sa.storage_instance_uuid
-- JOIN charm ON charm.uuid = si.charm_uuid
-- JOIN unit ON unit.uuid  = sa.unit_uuid AND unit.charm_uuid = si.charm_uuid
WHERE sa.unit_uuid = ? AND sa.storage_instance_uuid = ?`,
			unitUUID, storageUUID).Scan(&attachmentLife)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachmentLife, gc.Equals, life.Alive)

}

func (s *baseStorageSuite) assertFilesystemAttachment(c *gc.C, unitUUID coreunit.UUID, storageUUID corestorage.UUID, expected filesystemAttachment) {
	var (
		mountPoint string
		readOnly   bool
	)
	// Check that the filesystem attachment row exists.
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
SELECT sfa.mount_point, sfa.read_only FROM storage_filesystem_attachment sfa
JOIN unit ON unit.uuid = ?
JOIN net_node ON net_node.uuid = sfa.net_node_uuid
JOIN storage_instance_filesystem sif ON sif.storage_filesystem_uuid = sfa.storage_filesystem_uuid
WHERE sif.storage_instance_uuid = ?`,
			unitUUID, storageUUID).Scan(&mountPoint, &readOnly)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mountPoint, gc.Equals, expected.MountPoint)
	c.Assert(readOnly, gc.Equals, expected.ReadOnly)

}

func (s *baseStorageSuite) assertVolumeAttachment(c *gc.C, unitUUID coreunit.UUID, storageUUID corestorage.UUID, expected volumeAttachment) {
	var readOnly bool
	// Check that the volume attachment row exists.
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
SELECT sva.read_only FROM storage_volume_attachment sva
JOIN unit ON unit.uuid = ?
JOIN net_node ON net_node.uuid = sva.net_node_uuid
JOIN storage_instance_volume siv ON siv.storage_volume_uuid = sva.storage_volume_uuid
WHERE siv.storage_instance_uuid = ?`,
			unitUUID, storageUUID).Scan(&readOnly)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(readOnly, gc.Equals, expected.ReadOnly)

}

type filesystemAttachmentArg struct {
	unitUUID   coreunit.UUID
	readOnly   bool
	mountPoint string
}

func (s *baseStorageSuite) createFilesystem(c *gc.C, storageUUID corestorage.UUID, attachments ...filesystemAttachmentArg) {
	ctx := context.Background()

	filesystemUUID := storagetesting.GenFilesystemUUID(c)
	attachmentUUID := storagetesting.GenFilesystemAttachmentUUID(c)

	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_filesystem(uuid, life_id, filesystem_id, provisioning_status_id)
VALUES (?, ?, ?, ?)`, filesystemUUID, life.Alive, s.filesystemCount, domainstorage.ProvisioningStatusPending)
		if err != nil {
			return err
		}
		for _, a := range attachments {
			_, err = tx.ExecContext(ctx, `
INSERT INTO storage_filesystem_attachment(uuid, storage_filesystem_uuid, net_node_uuid, life_id, provisioning_status_id, mount_point, read_only)
VALUES (?, ?, (SELECT net_node_uuid FROM unit WHERE uuid = ?), ?, ?, ?, ?)`, attachmentUUID, filesystemUUID, a.unitUUID, life.Alive, domainstorage.ProvisioningStatusPending, a.mountPoint, a.readOnly)
			if err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO storage_instance_filesystem(storage_instance_uuid, storage_filesystem_uuid)
VALUES (?, ?)`, storageUUID, filesystemUUID)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	s.filesystemCount = s.filesystemCount + 1
}

type volumeAttachmentArg struct {
	unitUUID coreunit.UUID
	readOnly bool
}

func (s *baseStorageSuite) createVolume(c *gc.C, storageUUID corestorage.UUID, attachments ...volumeAttachmentArg) {
	ctx := context.Background()

	volumeUUID := storagetesting.GenVolumeUUID(c)
	attachmentUUID := storagetesting.GenVolumeAttachmentUUID(c)

	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_volume(uuid, life_id, volume_id, provisioning_status_id)
VALUES (?, ?, ?, ?)`, volumeUUID, life.Alive, 667, domainstorage.ProvisioningStatusPending)
		if err != nil {
			return err
		}
		for _, a := range attachments {
			_, err = tx.ExecContext(ctx, `
INSERT INTO storage_volume_attachment(uuid, storage_volume_uuid, net_node_uuid, life_id, provisioning_status_id, read_only)
VALUES (?, ?, (SELECT net_node_uuid FROM unit WHERE uuid = ?), ?, ?, ?, ?)`, attachmentUUID, volumeUUID, a.unitUUID, life.Alive, domainstorage.ProvisioningStatusPending, a.readOnly)
			if err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO storage_instance_volume(storage_instance_uuid, storage_volume_uuid)
VALUES (?, ?)`, storageUUID, volumeUUID)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}

type storageInstanceFilesystemArg struct {
	// Instance.
	StorageID         corestorage.ID
	StorageName       corestorage.Name
	LifeID            life.Life
	StoragePoolOrType string
	SizeMIB           uint64
	// Filesystem.
	FilesystemLifeID     life.Life
	FilesystemID         string
	ProvisioningStatusID domainstorage.ProvisioningStatus
}

func (s *baseStorageSuite) assertFilesystems(c *gc.C, charmUUID corecharm.ID, expected []storageInstanceFilesystemArg) {
	var results []storageInstanceFilesystemArg
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var row storageInstanceFilesystemArg
		rows, err := tx.QueryContext(ctx, `
SELECT
    sf.life_id AS filesystem_life_id, sf.filesystem_id,
    si.storage_id, si.storage_name, si.storage_pool, si.requested_size_mib
FROM storage_filesystem sf
JOIN storage_instance_filesystem sif ON sif.storage_filesystem_uuid = sf.uuid
JOIN v_storage_instance si ON si.uuid = sif.storage_instance_uuid
WHERE si.charm_uuid = ?`,
			charmUUID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			err = rows.Scan(&row.FilesystemLifeID, &row.FilesystemID, &row.StorageID,
				&row.StorageName, &row.StoragePoolOrType, &row.SizeMIB)
			if err != nil {
				return err
			}
			results = append(results, row)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.SameContents, expected)

}

type storageInstanceVolumeArg struct {
	// Instance.
	StorageID   corestorage.ID
	StorageName corestorage.Name
	LifeID      life.Life
	StoragePool string
	SizeMIB     uint64
	// Volume.
	VolumeLifeID         life.Life
	VolumeID             string
	ProvisioningStatusID domainstorage.ProvisioningStatus
}

func (s *baseStorageSuite) assertVolumes(c *gc.C, charmUUID corecharm.ID, expected []storageInstanceVolumeArg) {
	var results []storageInstanceVolumeArg
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var row storageInstanceVolumeArg
		rows, err := tx.QueryContext(ctx, `
SELECT
    sv.life_id AS volume_life_id, sv.volume_id,
    si.storage_id, si.storage_name, si.storage_pool, si.requested_size_mib
FROM storage_volume sv
JOIN storage_instance_volume siv ON siv.storage_volume_uuid = sv.uuid
JOIN v_storage_instance si ON si.uuid = siv.storage_instance_uuid
WHERE si.charm_uuid = ?`,
			charmUUID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			err = rows.Scan(&row.VolumeLifeID, &row.VolumeID, &row.StorageID,
				&row.StorageName, &row.StoragePool, &row.SizeMIB)
			if err != nil {
				return err
			}
			results = append(results, row)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.SameContents, expected)

}

// TODO(storage) - we need unit machine assignment to be done then this can be on the baseStorageSuite
func (s *caasStorageSuite) TestAttachStorageBadMountPoint(c *gc.C) {
	fsCopy := filesystemStorage
	fsCopy.location = "/var/lib/juju/storage/here"
	unitUUID, charmUUID := s.createUnitWithCharm(c, fsCopy)
	storageUUID := s.createStorageInstance(c, "pgdata", charmUUID, nil)
	s.createFilesystem(c, storageUUID)

	ctx := context.Background()
	err := s.state.AttachStorage(ctx, "/var/lib/juju/storage", storageUUID, unitUUID)
	c.Assert(err, jc.ErrorIs, applicationerrors.InvalidStorageMountPoint)
}

// TODO(storage) - we need unit machine assignment to be done then this can be on the baseStorageSuite
func (s *caasStorageSuite) TestAttachStorageFilesystemAlreadyAttached(c *gc.C) {
	unitUUID, charmUUID := s.createUnitWithCharm(c, filesystemStorage)
	storageUUID := s.createStorageInstance(c, "pgdata", charmUUID, nil)
	s.createFilesystem(c, storageUUID, filesystemAttachmentArg{unitUUID: unitUUID})

	ctx := context.Background()
	err := s.state.AttachStorage(ctx, s.storageParentDir, storageUUID, unitUUID)
	c.Assert(err, jc.ErrorIs, applicationerrors.FilesystemAlreadyAttached)
}

func (s *baseStorageSuite) TestAttachStorageUnitNotFound(c *gc.C) {
	_, charmUUID := s.createUnitWithCharm(c, filesystemStorage)
	storageUUID := s.createStorageInstance(c, "pgdata", charmUUID, nil)

	ctx := context.Background()
	err := s.state.AttachStorage(ctx, s.storageParentDir, storageUUID, unittesting.GenUnitUUID(c))
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *baseStorageSuite) TestAttachStorageUnitNotAlive(c *gc.C) {
	unitUUID, charmUUID := s.createUnitWithCharm(c, filesystemStorage)
	ctx := context.Background()
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE unit SET life_id = ? WHERE unit.name = ?`, 1, "foo/666")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	storageUUID := s.createStorageInstance(c, "pgdata", charmUUID, nil)

	err = s.state.AttachStorage(ctx, s.storageParentDir, storageUUID, unitUUID)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotAlive)
}

func (s *baseStorageSuite) TestAttachStorageNotFound(c *gc.C) {
	unitUUID, charmUUID := s.createUnitWithCharm(c, filesystemStorage)
	s.createStorageInstance(c, "pgdata", charmUUID, nil)

	ctx := context.Background()
	err := s.state.AttachStorage(ctx, s.storageParentDir, storagetesting.GenStorageUUID(c), unitUUID)
	c.Assert(err, jc.ErrorIs, storageerrors.StorageNotFound)
}

func (s *baseStorageSuite) TestAttachStorageNotAlive(c *gc.C) {
	unitUUID, charmUUID := s.createUnitWithCharm(c, filesystemStorage)
	storageUUID := s.createStorageInstance(c, "pgdata", charmUUID, nil)
	ctx := context.Background()
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE storage_instance SET life_id = ? WHERE uuid = ?`, 1, storageUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.AttachStorage(ctx, s.storageParentDir, storageUUID, unitUUID)
	c.Assert(err, jc.ErrorIs, applicationerrors.StorageNotAlive)
}

func (s *baseStorageSuite) TestAttachStorageTwice(c *gc.C) {
	unitUUID, charmUUID := s.createUnitWithCharm(c, filesystemStorage)
	storageUUID := s.createStorageInstance(c, "pgdata", charmUUID, nil)
	s.createFilesystem(c, storageUUID)

	ctx := context.Background()
	err := s.state.AttachStorage(ctx, s.storageParentDir, storageUUID, unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.AttachStorage(ctx, s.storageParentDir, storageUUID, unitUUID)
	c.Assert(err, jc.ErrorIs, applicationerrors.StorageAlreadyAttached)
}

func (s *baseStorageSuite) TestAttachStorageExceedsMaxCount(c *gc.C) {
	unitUUID, charmUUID := s.createUnitWithCharm(c, filesystemStorage)
	storageUUID := s.createStorageInstance(c, "pgdata", charmUUID, &unitUUID)
	s.createFilesystem(c, storageUUID)
	storageUUID2 := s.createStorageInstance(c, "pgdata", charmUUID, &unitUUID)
	s.createFilesystem(c, storageUUID2)
	storageUUID3 := s.createStorageInstance(c, "pgdata", charmUUID, nil)
	s.createFilesystem(c, storageUUID3)

	ctx := context.Background()
	err := s.state.AttachStorage(ctx, s.storageParentDir, storageUUID, unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.AttachStorage(ctx, s.storageParentDir, storageUUID2, unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.AttachStorage(ctx, s.storageParentDir, storageUUID3, unitUUID)
	c.Assert(err, jc.ErrorIs, applicationerrors.InvalidStorageCount)
}

func (s *baseStorageSuite) TestAttachStorageUnsupportedStorageName(c *gc.C) {
	ctx := context.Background()

	unitUUID, _ := s.createUnitWithCharm(c, filesystemStorage)

	charmUUID2 := charmtesting.GenCharmID(c).String()
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		err := insertCharmStateWithRevision(ctx, c, tx, charmUUID2, 666)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_storage (
    charm_uuid,
    name,
    storage_kind_id,
    count_min,
    count_max
) VALUES (?, ?, ?, ?, ?)`,
			charmUUID2, "other", domainstorage.StorageKindFilesystem, 1, 1)
		if err != nil {
			return errors.Capture(err)
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	storageUUID := s.createStorageInstance(c, "other", charmUUID2, nil)

	err = s.state.AttachStorage(ctx, s.storageParentDir, storageUUID, unitUUID)
	c.Assert(err, jc.ErrorIs, applicationerrors.StorageNameNotSupported)
}

type caasStorageSuite struct {
	baseStorageSuite
}

var _ = gc.Suite(&caasStorageSuite{})

func (s *caasStorageSuite) SetUpTest(c *gc.C) {
	s.baseStorageSuite.SetUpTest(c)

	modelUUID := testing.GenModelUUID(c)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, name, type, cloud, cloud_type)
			VALUES (?, ?, "test", "caas", "test-model", "microk8s")
		`, modelUUID.String(), coretesting.ControllerTag.Id())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

// TestCreateApplicationWithUnitsAndStorage tests creation of an application with
// units having storage.
// It verifies that the required volumes, filesystems, and attachment records are crated.
func (s *caasStorageSuite) TestCreateApplicationWithUnitsAndStorage(c *gc.C) {
	chStorage := []charm.Storage{{
		Name:     "database",
		Type:     "block",
		CountMin: 1,
		CountMax: 3,
	}, {
		Name:     "logs",
		Type:     "filesystem",
		CountMin: 1,
		CountMax: 1,
	}, {
		Name:     "cache",
		Type:     "filesystem",
		CountMin: 1,
		CountMax: 1,
	}}
	addStorageArgs := []application.ApplicationStorageArg{
		{
			Name:           "database",
			PoolNameOrType: "ebs",
			Size:           10,
			Count:          2,
		}, {
			Name:           "logs",
			PoolNameOrType: "rootfs",
			Size:           20,
			Count:          1,
		}, {
			Name:           "cache",
			PoolNameOrType: "loop",
			Size:           30,
			Count:          1,
		},
	}
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	ctx := context.Background()

	_, err := s.state.CreateApplication(ctx, "foo", s.addApplicationArgForStorage(c, "foo",
		s.storageParentDir, chStorage, addStorageArgs), []application.AddUnitArg{u1})
	c.Assert(err, jc.ErrorIsNil)

	var (
		charmUUID corecharm.ID
		unitUUID  coreunit.UUID
	)
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
SELECT charm_uuid
FROM application
WHERE name=?`, "foo").Scan(&charmUUID)
		if err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, `
SELECT uuid
FROM unit
WHERE name=?`, "foo/666").Scan(&unitUUID)
	})
	c.Assert(err, jc.ErrorIsNil)

	expectedStorageInstances := []storageInstance{{
		CharmUUID:        charmUUID,
		StorageID:        "database/0",
		StorageName:      "database",
		LifeID:           life.Alive,
		StorageType:      ptr("ebs"),
		RequestedSizeMIB: 10,
	}, {
		CharmUUID:        charmUUID,
		StorageID:        "database/1",
		StorageName:      "database",
		LifeID:           life.Alive,
		StorageType:      ptr("ebs"),
		RequestedSizeMIB: 10,
	}, {
		CharmUUID:        charmUUID,
		StorageID:        "logs/2",
		StorageName:      "logs",
		LifeID:           life.Alive,
		StorageType:      ptr("rootfs"),
		RequestedSizeMIB: 20,
	}, {
		CharmUUID:        charmUUID,
		StorageID:        "cache/3",
		StorageName:      "cache",
		LifeID:           life.Alive,
		StorageType:      ptr("loop"),
		RequestedSizeMIB: 30,
	}}

	var (
		foundStorageInstances []storageInstance
		storageUUIDByID       = make(map[corestorage.ID]corestorage.UUID)
	)

	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT uuid, storage_name, storage_pool_uuid, storage_type, requested_size_mib, storage_id, life_id
FROM storage_instance
WHERE charm_uuid = ?`, charmUUID)
		if err != nil {
			return errors.Capture(err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var inst storageInstance
			if err := rows.Scan(&inst.StorageUUID, &inst.StorageName, &inst.StoragePoolUUID, &inst.StorageType,
				&inst.RequestedSizeMIB, &inst.StorageID, &inst.LifeID); err != nil {
				return errors.Capture(err)
			}
			inst.CharmUUID = charmUUID
			storageUUIDByID[inst.StorageID] = inst.StorageUUID
			inst.StorageUUID = ""
			foundStorageInstances = append(foundStorageInstances, inst)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foundStorageInstances, jc.SameContents, expectedStorageInstances)

	s.assertFilesystems(c, charmUUID, []storageInstanceFilesystemArg{{
		StorageID:            "logs/2",
		StorageName:          "logs",
		LifeID:               life.Alive,
		StoragePoolOrType:    "rootfs",
		SizeMIB:              20,
		FilesystemLifeID:     life.Alive,
		FilesystemID:         "0",
		ProvisioningStatusID: domainstorage.ProvisioningStatusPending,
	}, {
		StorageID:            "cache/3",
		StorageName:          "cache",
		LifeID:               life.Alive,
		StoragePoolOrType:    "loop",
		SizeMIB:              30,
		FilesystemLifeID:     life.Alive,
		FilesystemID:         "1",
		ProvisioningStatusID: domainstorage.ProvisioningStatusPending,
	}})
	storageUUID, ok := storageUUIDByID["logs/2"]
	c.Assert(ok, jc.IsTrue)
	s.assertStorageAttached(c, unitUUID, storageUUID)
	s.assertFilesystemAttachment(c, unitUUID, storageUUID, filesystemAttachment{
		MountPoint:           filepath.Join(s.storageParentDir, "logs/2"),
		ReadOnly:             false,
		LifeID:               life.Alive,
		ProvisioningStatusID: domainstorage.ProvisioningStatusPending,
	})
	storageUUID, ok = storageUUIDByID["cache/3"]
	c.Assert(ok, jc.IsTrue)
	s.assertStorageAttached(c, unitUUID, storageUUID)
	s.assertFilesystemAttachment(c, unitUUID, storageUUID, filesystemAttachment{
		MountPoint:           filepath.Join(s.storageParentDir, "cache/3"),
		ReadOnly:             false,
		LifeID:               life.Alive,
		ProvisioningStatusID: domainstorage.ProvisioningStatusPending,
	})

	s.assertVolumes(c, charmUUID, []storageInstanceVolumeArg{{
		StorageID:            "database/0",
		StorageName:          "database",
		LifeID:               life.Alive,
		StoragePool:          "ebs",
		SizeMIB:              10,
		VolumeLifeID:         life.Alive,
		VolumeID:             "0",
		ProvisioningStatusID: domainstorage.ProvisioningStatusPending,
	}, {
		StorageID:            "database/1",
		StorageName:          "database",
		LifeID:               life.Alive,
		StoragePool:          "ebs",
		SizeMIB:              10,
		VolumeLifeID:         life.Alive,
		VolumeID:             "1",
		ProvisioningStatusID: domainstorage.ProvisioningStatusPending,
	}, {
		StorageID:            "cache/3",
		StorageName:          "cache",
		LifeID:               life.Alive,
		StoragePool:          "loop",
		SizeMIB:              30,
		VolumeLifeID:         life.Alive,
		VolumeID:             "2",
		ProvisioningStatusID: domainstorage.ProvisioningStatusPending,
	}})
	storageUUID, ok = storageUUIDByID["database/0"]
	c.Assert(ok, jc.IsTrue)
	s.assertStorageAttached(c, unitUUID, storageUUID)
	s.assertVolumeAttachment(c, unitUUID, storageUUID, volumeAttachment{
		ReadOnly:             false,
		LifeID:               life.Alive,
		ProvisioningStatusID: domainstorage.ProvisioningStatusPending,
	})
	storageUUID, ok = storageUUIDByID["database/1"]
	c.Assert(ok, jc.IsTrue)
	s.assertStorageAttached(c, unitUUID, storageUUID)
	s.assertVolumeAttachment(c, unitUUID, storageUUID, volumeAttachment{
		ReadOnly:             false,
		LifeID:               life.Alive,
		ProvisioningStatusID: domainstorage.ProvisioningStatusPending,
	})
	storageUUID, ok = storageUUIDByID["cache/3"]
	c.Assert(ok, jc.IsTrue)
	s.assertStorageAttached(c, unitUUID, storageUUID)
	s.assertVolumeAttachment(c, unitUUID, storageUUID, volumeAttachment{
		ReadOnly:             false,
		LifeID:               life.Alive,
		ProvisioningStatusID: domainstorage.ProvisioningStatusPending,
	})
}

type iaasStorageSuite struct {
	baseStorageSuite
}

var _ = gc.Suite(&iaasStorageSuite{})

func (s *iaasStorageSuite) SetUpTest(c *gc.C) {
	s.baseStorageSuite.SetUpTest(c)

	modelUUID := testing.GenModelUUID(c)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, name, type, cloud, cloud_type)
			VALUES (?, ?, "test", "iaas", "test-model", "ec2")
		`, modelUUID.String(), coretesting.ControllerTag.Id())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *iaasStorageSuite) TestAttachStorageFilesystem(c *gc.C) {
	unitUUID, charmUUID := s.createUnitWithCharm(c, filesystemStorage)
	storageUUID := s.createStorageInstance(c, "pgdata", charmUUID, nil)
	s.createFilesystem(c, storageUUID)

	ctx := context.Background()
	err := s.state.AttachStorage(ctx, s.storageParentDir, storageUUID, unitUUID)
	c.Assert(err, jc.ErrorIsNil)

	s.assertStorageAttached(c, unitUUID, storageUUID)
	// TODO(storage) - we need unit machine assignment to be done
	//s.assertFilesystemAttachment(c, unitUUID, storageUUID, filesystemAttachment{
	//	MountPoint: "/tmp/pgdata/0",
	//	ReadOnly:   true,
	//})
}

func (s *iaasStorageSuite) TestAttachStorageVolume(c *gc.C) {
	unitUUID, charmUUID := s.createUnitWithCharm(c, blockStorage)
	storageUUID := s.createStorageInstance(c, "pgblock", charmUUID, nil)
	s.createVolume(c, storageUUID)

	ctx := context.Background()
	err := s.state.AttachStorage(ctx, s.storageParentDir, storageUUID, unitUUID)
	c.Assert(err, jc.ErrorIsNil)

	s.assertStorageAttached(c, unitUUID, storageUUID)
	// TODO(storage) - we need unit machine assignment to be done
	//s.assertVolumeAttachment(c, unitUUID, storageUUID, volumeAttachment{
	//	ReadOnly: true,
	//})
}

func (s *iaasStorageSuite) TestAttachStorageVolumeBackedFilesystem(c *gc.C) {
	unitUUID, charmUUID := s.createUnitWithCharm(c, filesystemStorage)
	storageUUID := s.createStorageInstance(c, "pgdata", charmUUID, nil)
	s.createFilesystem(c, storageUUID)
	s.createVolume(c, storageUUID)

	ctx := context.Background()
	err := s.state.AttachStorage(ctx, s.storageParentDir, storageUUID, unitUUID)
	c.Assert(err, jc.ErrorIsNil)

	s.assertStorageAttached(c, unitUUID, storageUUID)
	// TODO(storage) - we need unit machine assignment to be done
	//s.assertFilesystemAttachment(c, unitUUID, storageUUID, filesystemAttachment{
	//	MountPoint: "/tmp/pgdata/0",
	//	ReadOnly:   true,
	//})
	//s.assertVolumeAttachment(c, unitUUID, storageUUID, volumeAttachment{
	//	ReadOnly: true,
	//})
}
