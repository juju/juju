// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	corecharm "github.com/juju/juju/core/charm"
	charmtesting "github.com/juju/juju/core/charm/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domainsequence "github.com/juju/juju/domain/sequence"
	sequencestate "github.com/juju/juju/domain/sequence/state"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/domain/storage"
	storagetesting "github.com/juju/juju/domain/storage/testing"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningtesting "github.com/juju/juju/domain/storageprovisioning/testing"
)

// baseStorageSuite defines a set of common test suite fixtures for common
// storage operations during testing.
type baseStorageSuite struct {
	schematesting.ModelSuite
}

type preparer struct{}

// newCharm creates a new charm in the model and returns the uuid for it.
func (s *baseStorageSuite) newCharm(c *tc.C) corecharm.ID {
	charmUUID := charmtesting.GenCharmID(c)
	_, err := s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO charm (uuid, source_id, reference_name, revision, architecture_id)
VALUES (?, 0, ?, 1, 0)
`,
		charmUUID.String(), "foo-"+charmUUID[:4],
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO charm_metadata (charm_uuid, name)
VALUES (?, 'myapp')
`,
		charmUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)
	return charmUUID
}

func (s *baseStorageSuite) newCharmStorage(
	c *tc.C,
	charmUUID corecharm.ID,
	storageName string,
	kind storage.StorageKind,
) {
	_, err := s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO charm_storage (charm_uuid, name, storage_kind_id, count_min, count_max)
VALUES (?, ?, ?, 0, 1)
`,
		charmUUID, storageName, kind,
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseStorageSuite) newFilesystem(c *tc.C) (
	storageprovisioning.FilesystemUUID, string,
) {
	fsUUID := storageprovisioningtesting.GenFilesystemUUID(c)

	fsID := fmt.Sprintf("foo/%s", fsUUID.String())

	_, err := s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 0)
	`,
		fsUUID.String(), fsID)
	c.Assert(err, tc.ErrorIsNil)

	return fsUUID, fsID
}

func (s *baseStorageSuite) newFilesystemWithStatus(
	c *tc.C,
	sType status.StorageFilesystemStatusType,
) (storageprovisioning.FilesystemUUID, string) {
	fsUUID, fsID := s.newFilesystem(c)

	_, err := s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_filesystem_status (filesystem_uuid, status_id)
VALUES (?, ?)
`,
		fsUUID.String(), int(sType),
	)
	c.Assert(err, tc.ErrorIsNil)

	return fsUUID, fsID
}

// nextStorageSequenceNumber retrieves the next sequence number in the storage
// namespace.
func (s *baseStorageSuite) nextSequenceNumber(
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

func (s *baseStorageSuite) newStorageInstance(
	c *tc.C,
	charmUUID corecharm.ID,
	storageName string,
	poolUUID storage.StoragePoolUUID,
) (storage.StorageInstanceUUID, string) {
	storageInstanceUUID := storagetesting.GenStorageInstanceUUID(c)
	storageID := fmt.Sprintf("%s/%d", storageName, s.nextSequenceNumber(c, "storage"))

	var charmName string
	err := s.DB().QueryRowContext(
		c.Context(),
		"SELECT name FROM charm_metadata WHERE charm_uuid = ?",
		charmUUID,
	).Scan(&charmName)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_instance(uuid, charm_name, storage_name, storage_id,
                             storage_kind_id, life_id, requested_size_mib,
                             storage_pool_uuid)
VALUES (?, ?, ?, ?, 1, 0, 100, ?)
`,
		storageInstanceUUID.String(),
		charmName,
		storageName,
		storageID,
		poolUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	return storageInstanceUUID, storageID
}

func (s *baseStorageSuite) newStorageInstanceFilesystem(
	c *tc.C,
	instanceUUID storage.StorageInstanceUUID,
	filesystemUUID storageprovisioning.FilesystemUUID,
) {
	_, err := s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_instance_filesystem (storage_instance_uuid, storage_filesystem_uuid)
VALUES (?, ?)
`,
		instanceUUID.String(),
		filesystemUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseStorageSuite) newStorageInstanceVolume(
	c *tc.C, instanceUUID storage.StorageInstanceUUID,
	volumeUUID storageprovisioning.VolumeUUID,
) {
	_, err := s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_instance_volume (storage_instance_uuid, storage_volume_uuid)
VALUES (?, ?)
`,
		instanceUUID.String(),
		volumeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)
}

// newStoragePool creates a new storage pool with name, provider type and attrs.
// It returns the UUID of the new storage pool.
func (s *baseStorageSuite) newStoragePool(c *tc.C,
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

// newVolume creates a new volume in the model with model
// provision scope. Return is the uuid and volume id of the entity.
func (s *baseStorageSuite) newVolume(c *tc.C) (storageprovisioning.VolumeUUID, string) {
	vsUUID := storageprovisioningtesting.GenVolumeUUID(c)

	vsID := fmt.Sprintf("foo/%s", vsUUID.String())

	_, err := s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 0)
	`,
		vsUUID.String(), vsID)
	c.Assert(err, tc.ErrorIsNil)

	return vsUUID, vsID
}

// newVolumeWithStatus creates a new volume in the model with model
// provision scope and it's initial status set.
func (s *baseStorageSuite) newVolumeWithStatus(
	c *tc.C,
	sType status.StorageVolumeStatusType,
) (storageprovisioning.VolumeUUID, string) {
	vsUUID, vsID := s.newVolume(c)

	_, err := s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_volume_status(volume_uuid, status_id)
VALUES (?, ?)
`,
		vsUUID.String(), int(sType),
	)
	c.Assert(err, tc.ErrorIsNil)

	return vsUUID, vsID
}

func (p preparer) Prepare(query string, typeSamples ...any) (*sqlair.Statement, error) {
	return sqlair.Prepare(query, typeSamples...)
}
