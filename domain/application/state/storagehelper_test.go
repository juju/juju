// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"

	"github.com/juju/tc"

	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageprov "github.com/juju/juju/domain/storageprovisioning"
)

type dbGetter interface {
	DB() *sql.DB
}

type storageHelper struct {
	dbGetter
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

// newStorageInstanceFilesysatemWithProviderID creates a new storage instance in
// the model backed by a filesystem with the given provider ID set.
func (s *storageHelper) newStorageInstanceFilesysatemWithProviderID(
	c *tc.C, storageName string, providerID string,
) (domainstorage.StorageInstanceUUID, domainstorageprov.FilesystemUUID) {
	storageInstUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	storageFilesystemUUID := tc.Must(c, domainstorageprov.NewFilesystemUUID)
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
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provider_id)
VALUES (?, ?, 0, ?)
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
	domainstorageprov.FilesystemUUID,
	domainstorageprov.VolumeUUID,
) {
	storageInstUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	storageFilesystemUUID := tc.Must(c, domainstorageprov.NewFilesystemUUID)
	storageVolumeUUID := tc.Must(c, domainstorageprov.NewVolumeUUID)
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
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provider_id)
VALUES (?, ?, 0, ?)
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
INSERT INTO storage_volume (uuid, volume_id, life_id, provider_id)
VALUES (?, ?, 0, ?)
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
) (domainstorage.StorageInstanceUUID, domainstorageprov.VolumeUUID) {
	storageInstUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	storageVolumeUUID := tc.Must(c, domainstorageprov.NewVolumeUUID)
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
INSERT INTO storage_volume (uuid, volume_id, life_id, provider_id)
VALUES (?, ?, 0, ?)
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
