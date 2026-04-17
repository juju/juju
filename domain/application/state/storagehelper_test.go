// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/tc"

	corecharm "github.com/juju/juju/core/charm"
	coredatabase "github.com/juju/juju/core/database"
	coreunit "github.com/juju/juju/core/unit"
	domainlife "github.com/juju/juju/domain/life"
	domainstorage "github.com/juju/juju/domain/storage"
)

type dbGetter interface {
	DB() *sql.DB
	TxnRunnerFactory() func(context.Context) (coredatabase.TxnRunner, error)
}

type storageHelper struct {
	dbGetter
}

// assertFilesystemAttachmentExists ensures a filesystem attachment exists for
// the supplied attachment UUID.
func (s *storageHelper) assertFilesystemAttachmentExists(
	c *tc.C, attachmentUUID domainstorage.FilesystemAttachmentUUID,
) {
	var gotUUID string
	err := s.DB().QueryRowContext(
		c.Context(),
		"SELECT uuid FROM storage_filesystem_attachment WHERE uuid = ?",
		attachmentUUID.String(),
	).Scan(&gotUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotUUID, tc.Equals, attachmentUUID.String())
}

// assertStorageInstanceAttachmentExists ensures a storage attachment exists
// for the supplied attachment, storage instance, and unit UUIDs.
func (s *storageHelper) assertStorageInstanceAttachmentExists(
	c *tc.C,
	attachmentUUID domainstorage.StorageAttachmentUUID,
	storageInstanceUUID domainstorage.StorageInstanceUUID,
	unitUUID coreunit.UUID,
) {
	var gotUUID string
	err := s.DB().QueryRowContext(
		c.Context(),
		`SELECT uuid FROM storage_attachment
WHERE uuid = ? AND storage_instance_uuid = ? AND unit_uuid = ?`,
		attachmentUUID.String(),
		storageInstanceUUID.String(),
		unitUUID.String(),
	).Scan(&gotUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotUUID, tc.Equals, attachmentUUID.String())
}

// getStorageInstanceCharmName returns the charm name for the supplied Storage
// Instance UUID.
func (s *storageHelper) getStorageInstanceCharmName(
	c *tc.C, storageInstanceUUID domainstorage.StorageInstanceUUID,
) string {
	var charmName string
	err := s.DB().QueryRowContext(
		c.Context(),
		"SELECT charm_name FROM storage_instance WHERE uuid = ?",
		storageInstanceUUID.String(),
	).Scan(&charmName)
	c.Assert(err, tc.ErrorIsNil)
	return charmName
}

func (s *storageHelper) newStoragePool(c *tc.C,
	name, providerType string,
) domainstorage.StoragePoolUUID {
	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	_, err := s.DB().Exec(
		"INSERT INTO storage_pool (uuid, name, type) VALUES (?, ?, ?)",
		poolUUID, name, providerType,
	)
	c.Assert(err, tc.ErrorIsNil)
	return poolUUID
}

// newStorageInstanceWithName creates a new storage instance with the supplied
// storage name and no backing filesystem or volume.
func (s *storageHelper) newStorageInstanceWithName(
	c *tc.C, storageName string,
) domainstorage.StorageInstanceUUID {
	return s.newStorageInstanceWithValues(
		c,
		storageName,
		domainstorage.StorageKindFilesystem,
		domainlife.Alive,
		"bar",
		1024,
	)
}

// newStorageInstanceWithValues creates a new storage instance with the supplied
// values and no backing filesystem or volume.
func (s *storageHelper) newStorageInstanceWithValues(
	c *tc.C,
	storageName string,
	kind domainstorage.StorageKind,
	life domainlife.Life,
	charmName string,
	requestedSizeMIB int,
) domainstorage.StorageInstanceUUID {
	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	storagePoolUUID := s.newStoragePool(c, storageInstanceUUID.String(), "test-provider")

	_, err := s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_instance (uuid, storage_name, storage_kind_id, storage_id,
                              life_id, storage_pool_uuid, charm_name, requested_size_mib)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`,
		storageInstanceUUID.String(),
		storageName,
		kind,
		storageInstanceUUID.String(),
		life,
		storagePoolUUID.String(),
		charmName,
		requestedSizeMIB,
	)
	c.Assert(err, tc.ErrorIsNil)

	return storageInstanceUUID
}

// newModelFilesystemForStorageInstance creates a model filesystem and links it
// to the supplied storage instance using default values.
//
// The filesystem created has life set to alive, model provision scope and
// size 1024 MiB.
func (s *storageHelper) newModelFilesystemForStorageInstance(
	c *tc.C,
	storageInstanceUUID domainstorage.StorageInstanceUUID,
) domainstorage.FilesystemUUID {
	filesystemUUID := tc.Must(c, domainstorage.NewFilesystemUUID)

	_, err := s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provision_scope_id, size_mib)
VALUES (?, ?, ?, ?, ?)
`,
		filesystemUUID.String(),
		filesystemUUID.String(),
		domainlife.Alive,
		0,
		1024,
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_instance_filesystem (storage_instance_uuid,
                                         storage_filesystem_uuid)
VALUES (?, ?)
`,
		storageInstanceUUID.String(),
		filesystemUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return filesystemUUID
}

// newModelVolumeForStorageInstance creates a model volume and links it to the
// supplied storage instance using default values.
//
// The volume created has life set to alive, model provision scope and
// size 2048 MiB.
func (s *storageHelper) newModelVolumeForStorageInstance(
	c *tc.C,
	storageInstanceUUID domainstorage.StorageInstanceUUID,
) domainstorage.VolumeUUID {
	volumeUUID := tc.Must(c, domainstorage.NewVolumeUUID)

	_, err := s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id, size_mib)
VALUES (?, ?, ?, ?, ?)
`,
		volumeUUID.String(),
		volumeUUID.String(),
		domainlife.Alive,
		0,
		2048,
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_instance_volume (storage_instance_uuid,
                                     storage_volume_uuid)
VALUES (?, ?)
`,
		storageInstanceUUID.String(),
		volumeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return volumeUUID
}

// newModelFilesystemStorageInstance creates a new storage instance backed by a
// model provisioned filesystem, using the charm name from the supplied charm
// UUID.
func (s *storageHelper) newModelFilesystemStorageInstance(
	c *tc.C, storageName string, charmUUID corecharm.ID,
) (domainstorage.StorageInstanceUUID, domainstorage.FilesystemUUID) {
	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	filesystemUUID := tc.Must(c, domainstorage.NewFilesystemUUID)
	storagePoolUUID := s.newStoragePool(c, storageInstanceUUID.String(), "test-provider")

	var charmName string
	err := s.DB().QueryRowContext(
		c.Context(),
		"SELECT name FROM charm_metadata WHERE charm_uuid = ?",
		charmUUID.String(),
	).Scan(&charmName)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_instance (uuid, storage_name, storage_kind_id, storage_id,
                              life_id, storage_pool_uuid, charm_name, requested_size_mib)
VALUES (?, ?, 1, ?, ?, ?, ?, 1024)
`,
		storageInstanceUUID.String(),
		storageName,
		storageInstanceUUID.String(),
		domainlife.Alive,
		storagePoolUUID.String(),
		charmName,
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provision_scope_id, size_mib)
VALUES (?, ?, ?, 0, 1024)
	`,
		filesystemUUID.String(),
		filesystemUUID.String(),
		domainlife.Alive,
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_instance_filesystem (storage_instance_uuid,
                                         storage_filesystem_uuid)
VALUES (?, ?)
	`,
		storageInstanceUUID.String(),
		filesystemUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return storageInstanceUUID, filesystemUUID
}

// newModelVolumeStorageInstance creates a new storage instance backed by a
// model provisioned volume, using the charm name from the supplied charm UUID.
func (s *storageHelper) newModelVolumeStorageInstance(
	c *tc.C, storageName string, charmUUID corecharm.ID,
) (domainstorage.StorageInstanceUUID, domainstorage.VolumeUUID) {
	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	volumeUUID := tc.Must(c, domainstorage.NewVolumeUUID)
	storagePoolUUID := s.newStoragePool(c, storageInstanceUUID.String(), "test-provider")

	var charmName string
	err := s.DB().QueryRowContext(
		c.Context(),
		"SELECT name FROM charm_metadata WHERE charm_uuid = ?",
		charmUUID.String(),
	).Scan(&charmName)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_instance (uuid, storage_name, storage_kind_id, storage_id,
                              life_id, storage_pool_uuid, charm_name, requested_size_mib)
VALUES (?, ?, 0, ?, ?, ?, ?, 2048)
`,
		storageInstanceUUID.String(),
		storageName,
		storageInstanceUUID.String(),
		domainlife.Alive,
		storagePoolUUID.String(),
		charmName,
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id, size_mib)
VALUES (?, ?, ?, 0, 2048)
	`,
		volumeUUID.String(),
		volumeUUID.String(),
		domainlife.Alive,
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_instance_volume (storage_instance_uuid, storage_volume_uuid)
VALUES (?, ?)
	`,
		storageInstanceUUID.String(),
		volumeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return storageInstanceUUID, volumeUUID
}

// newStorageInstanceFilesysatemWithProviderID creates a new storage instance in
// the model backed by a filesystem with the given provider ID set.
func (s *storageHelper) newStorageInstanceFilesysatemWithProviderID(
	c *tc.C, storageName string, providerID string,
) (domainstorage.StorageInstanceUUID, domainstorage.FilesystemUUID) {
	storageInstUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	storageFilesystemUUID := tc.Must(c, domainstorage.NewFilesystemUUID)
	storagePoolUUID := s.newStoragePool(c, storageInstUUID.String(), "lxd")

	_, err := s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_instance (uuid, charm_name, storage_name,
                              storage_kind_id, storage_id, life_id,
                              storage_pool_uuid, requested_size_mib)
VALUES (?, ?, ?, 1, ?, 0, ?, 1024)
`,
		storageInstUUID.String(),
		"charm",
		storageName,
		storageInstUUID.String(),
		storagePoolUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provider_id,
                                provision_scope_id)
VALUES (?, ?, 0, ?, 0)
		`,
		storageFilesystemUUID.String(),
		storageFilesystemUUID.String(),
		providerID,
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		"INSERT INTO storage_instance_filesystem VALUES (?, ?)",
		storageInstUUID.String(), storageFilesystemUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return storageInstUUID, storageFilesystemUUID
}

// newStorageInstanceFilesystemBackedVolumeWithProviderID creates a new storage
// instance in the model backed by a filesystem and volume with their provider
// ids respectively set to the supplied values.
//
// This is useful for simulating a volume backed filesystem in the model.
func (s *storageHelper) newStorageInstanceFilesystemBackedVolumeWithProviderID(
	c *tc.C, storageName string, fsProviderID, vProviderID string,
) (
	domainstorage.StorageInstanceUUID,
	domainstorage.FilesystemUUID,
	domainstorage.VolumeUUID,
) {
	storageInstUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	storageFilesystemUUID := tc.Must(c, domainstorage.NewFilesystemUUID)
	storageVolumeUUID := tc.Must(c, domainstorage.NewVolumeUUID)
	storagePoolUUID := s.newStoragePool(c, storageInstUUID.String(), "lxd")

	_, err := s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_instance (uuid, charm_name, storage_name,
                              storage_kind_id, storage_id, life_id,
                              storage_pool_uuid, requested_size_mib)
VALUES (?, ?, ?, 1, ?, 0, ?, 1024)
`,
		storageInstUUID.String(),
		"charm",
		storageName,
		storageInstUUID.String(),
		storagePoolUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provider_id,
                                provision_scope_id)
VALUES (?, ?, 0, ?, 0)
		`,
		storageFilesystemUUID.String(),
		storageFilesystemUUID.String(),
		fsProviderID,
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		"INSERT INTO storage_instance_filesystem VALUES (?, ?)",
		storageInstUUID.String(), storageFilesystemUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_volume (uuid, volume_id, life_id, provider_id,
                            provision_scope_id)
VALUES (?, ?, 0, ?, 0)
		`,
		storageVolumeUUID.String(),
		storageVolumeUUID.String(),
		vProviderID,
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		"INSERT INTO storage_instance_volume VALUES (?, ?)",
		storageInstUUID.String(), storageVolumeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return storageInstUUID, storageFilesystemUUID, storageVolumeUUID
}

// newStorageInstanceVolumeWithProviderID creates a new storage instance in
// the model backed by a volume with the given provider ID set.
func (s *storageHelper) newStorageInstanceVolumeWithProviderID(
	c *tc.C, storageName string, providerID string,
) (domainstorage.StorageInstanceUUID, domainstorage.VolumeUUID) {
	storageInstUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	storageVolumeUUID := tc.Must(c, domainstorage.NewVolumeUUID)
	storagePoolUUID := s.newStoragePool(c, storageInstUUID.String(), "lxd")

	_, err := s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_instance (uuid, charm_name, storage_name,
                              storage_kind_id, storage_id, life_id,
                              storage_pool_uuid, requested_size_mib)
VALUES (?, ?, ?, 1, ?, 0, ?, 1024)
`,
		storageInstUUID.String(),
		"charm",
		storageName,
		storageInstUUID.String(),
		storagePoolUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_volume (uuid, volume_id, life_id, provider_id,
                            provision_scope_id)
VALUES (?, ?, 0, ?, 0)
		`,
		storageVolumeUUID.String(),
		storageVolumeUUID.String(),
		providerID,
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		"INSERT INTO storage_instance_volume VALUES (?, ?)",
		storageInstUUID.String(), storageVolumeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return storageInstUUID, storageVolumeUUID
}

// newStorageUnitOwner is a helper function to create a new storage unit owner
// for the supplied instance and unit.
func (s *storageHelper) newStorageUnitOwner(
	c *tc.C, instUUID domainstorage.StorageInstanceUUID, unitUUID coreunit.UUID,
) {
	_, err := s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_unit_owner (storage_instance_uuid, unit_uuid) VALUES (?, ?)
`,
		instUUID.String(),
		unitUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)
}

// newStorageInstanceAttachment creates a storage attachment for the supplied
// storage instance and unit.
func (s *storageHelper) newStorageInstanceAttachment(
	c *tc.C, instUUID domainstorage.StorageInstanceUUID, unitUUID coreunit.UUID,
) domainstorage.StorageAttachmentUUID {
	attachmentUUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	_, err := s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_attachment (uuid, storage_instance_uuid, unit_uuid, life_id)
VALUES (?, ?, ?, ?)
`,
		attachmentUUID.String(),
		instUUID.String(),
		unitUUID.String(),
		domainlife.Alive,
	)
	c.Assert(err, tc.ErrorIsNil)
	return attachmentUUID
}

// setStorageInstanceLife updates the life for the supplied storage instance.
func (s *storageHelper) setStorageInstanceLife(
	c *tc.C, storageInstanceUUID domainstorage.StorageInstanceUUID,
	life domainlife.Life,
) {
	_, err := s.DB().ExecContext(
		c.Context(),
		"UPDATE storage_instance SET life_id = ? WHERE uuid = ?",
		life,
		storageInstanceUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)
}
