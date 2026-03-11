// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	coremachine "github.com/juju/juju/core/machine"
	corenetwork "github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	domainapplication "github.com/juju/juju/domain/application"
	domainblockdevice "github.com/juju/juju/domain/blockdevice"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/schema/testing"
	domainsequence "github.com/juju/juju/domain/sequence"
	domainsequencestate "github.com/juju/juju/domain/sequence/state"
	domainstatus "github.com/juju/juju/domain/status"
	domainstorage "github.com/juju/juju/domain/storage"
)

type baseSuite struct {
	testing.ModelSuite
}

// preparer implements a testing [github.com/juju/juju/domain.Preparer] that
// results in a proxied call to [sqlair.Prepare].
type preparer struct{}

// newBlockDevice creates a new block device in the model for the provided
// machine. Returns the block device UUID.
func (s *baseSuite) newBlockDevice(
	c *tc.C,
	machineUUID coremachine.UUID,
) domainblockdevice.BlockDeviceUUID {
	uuid := tc.Must(c, domainblockdevice.NewBlockDeviceUUID)
	_, err := s.DB().Exec(
		`INSERT INTO block_device (uuid, machine_uuid, name) VALUES (?, ?, ?)`,
		uuid, machineUUID.String(), uuid)
	c.Assert(err, tc.ErrorIsNil)

	return uuid
}

// newBlockStorageInstanceForCharmWithPool is responsible for establishing a new
// storage instance of kind block in the model using the supplied charm and
// storage pool. Returned is the new uuid for the storage instance and the
// storage id.
func (s *baseSuite) newBlockStorageInstanceForCharmWithPool(
	c *tc.C,
	charmUUID corecharm.ID,
	poolUUID domainstorage.StoragePoolUUID,
	storageName string,
) (domainstorage.StorageInstanceUUID, string) {
	var charmName string
	err := s.DB().QueryRowContext(
		c.Context(),
		"SELECT name FROM charm_metadata WHERE charm_uuid = ?",
		charmUUID.String(),
	).Scan(&charmName)
	c.Assert(err, tc.ErrorIsNil)

	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	storageID := fmt.Sprintf("%s/%d", storageName, s.nextStorageSequenceNumber(c))

	_, err = s.DB().Exec(`
INSERT INTO storage_instance(uuid, charm_name, storage_name, storage_id,
                             life_id, requested_size_mib, storage_pool_uuid,
                             storage_kind_id)
VALUES (?, ?, ?, ?, 0, 100, ?, 0)
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

// newCharm creates a new charm in the model returning the charm uuid.
func (s *baseSuite) newCharm(c *tc.C) corecharm.ID {
	charmUUID := tc.Must(c, corecharm.NewID)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`
INSERT INTO charm (uuid, source_id, reference_name, revision, architecture_id)
VALUES (?, 0, ?, 1, 0)
`,
			charmUUID.String(), charmUUID.String(),
		)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(
			ctx,
			`
INSERT INTO charm_metadata (charm_uuid, name)
VALUES (?, ?)
`,
			charmUUID.String(), charmUUID.String(),
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return charmUUID
}

// newApplication creates a new application in the model with the provided name.
// It also creates a new charm and associates it with the application.
// Returns the application UUID and the charm UUID.
func (s *baseSuite) newApplication(c *tc.C, name string) (coreapplication.UUID, corecharm.ID) {
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmUUID := s.newCharm(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO application (uuid, charm_uuid, name, life_id, space_uuid)
VALUES (?, ?, ?, "0", ?)`, appUUID.String(), charmUUID.String(), name, corenetwork.AlphaSpaceId)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return appUUID, charmUUID
}

// newFilesystemStorageInstanceForCharmWithPool is responsible for establishing
// a new storage instance in the model using the supplied storage pool. Returned
// is the new uuid for the storage instance and the storage id.
func (s *baseSuite) newFilesystemStorageInstanceForCharmWithPool(
	c *tc.C,
	charmUUID corecharm.ID,
	poolUUID domainstorage.StoragePoolUUID,
	storageName string,
) (domainstorage.StorageInstanceUUID, string) {
	var charmName string
	err := s.DB().QueryRowContext(
		c.Context(),
		"SELECT name FROM charm_metadata WHERE charm_uuid = ?",
		charmUUID.String(),
	).Scan(&charmName)
	c.Assert(err, tc.ErrorIsNil)

	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	storageID := fmt.Sprintf("%s/%d", storageName, s.nextStorageSequenceNumber(c))

	_, err = s.DB().Exec(`
INSERT INTO storage_instance(uuid, charm_name, storage_name, storage_id,
                             life_id, requested_size_mib, storage_pool_uuid,
                             storage_kind_id)
VALUES (?, ?, ?, ?, 0, 100, ?, 1)
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

// newMachine create a new machine in the model returning the uuid of the new
// Machine created and the Machine name.
func (s *baseSuite) newMachine(c *tc.C) (coremachine.UUID, string) {
	uuid := tc.Must(c, coremachine.NewUUID)
	name := uuid.String()
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)

	_, err := s.DB().Exec("INSERT INTO net_node VALUES (?)", netNodeUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(
		`INSERT INTO machine (uuid, name, net_node_uuid, life_id)
		VALUES (?, ?, ?, 0)`,
		uuid.String(), name, netNodeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return uuid, name
}

func (s *baseSuite) newModelFilesystem(
	c *tc.C, siUUID domainstorage.StorageInstanceUUID,
) domainstorage.FilesystemUUID {
	fsUUID := tc.Must(c, domainstorage.NewFilesystemUUID)
	fsID := fmt.Sprintf("foo/%s", fsUUID.String())

	_, err := s.DB().Exec(`
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 0)
`,
		fsUUID.String(), fsID)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(`
INSERT INTO storage_instance_filesystem (storage_instance_uuid, storage_filesystem_uuid)
VALUES (?, ?)
`,
		siUUID.String(), fsUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return fsUUID
}

// newModelFilesystemAttachment creates a new filesystem attachment that has
// model provision scope. The attachment is associated with the provided
// filesystem uuid and net node uuid.
func (s *baseSuite) newModelFilesystemAttachmentWithMountPoint(
	c *tc.C,
	filesystemUUID domainstorage.FilesystemUUID,
	netNodeUUID domainnetwork.NetNodeUUID,
	mountPoint string,
) domainstorage.FilesystemAttachmentUUID {
	attachmentUUID := tc.Must(c, domainstorage.NewFilesystemAttachmentUUID)

	_, err := s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_filesystem_attachment (uuid,
                                           storage_filesystem_uuid,
                                           net_node_uuid,
                                           life_id,
                                           mount_point,
                                           provision_scope_id)
VALUES (?, ?, ?, 0, ?, 0)
`,
		attachmentUUID.String(),
		filesystemUUID.String(),
		netNodeUUID.String(),
		mountPoint,
	)
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID
}

// newModelVolume creates a new volume in the model with model provision scope
// and associates it with the provided storage instance. Returned is the uuid
// of the volume.
func (s *baseSuite) newModelVolume(
	c *tc.C,
	storageInstanceUUID domainstorage.StorageInstanceUUID,
) domainstorage.VolumeUUID {
	volumeUUID := tc.Must(c, domainstorage.NewVolumeUUID)
	volumeID := strconv.FormatUint(s.nextVolumeSequenceNumber(c), 10)

	_, err := s.DB().Exec(`
INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 0)
	`,
		volumeUUID.String(), volumeID)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(`
INSERT INTO storage_instance_volume (storage_instance_uuid, storage_volume_uuid)
VALUES (?, ?)
	`,
		storageInstanceUUID.String(), volumeUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	return volumeUUID
}

// newModelVolumeAttachment creates a new volume attachment that has model
// provision scope. The attachment is associated with the provided volume uuid,
// net node uuid, and block device uuid.
func (s *baseSuite) newModelVolumeAttachment(
	c *tc.C,
	volumeUUID domainstorage.VolumeUUID,
	netNodeUUID domainnetwork.NetNodeUUID,
	blockDeviceUUID domainblockdevice.BlockDeviceUUID,
) domainstorage.VolumeAttachmentUUID {
	attachmentUUID := tc.Must(c, domainstorage.NewVolumeAttachmentUUID)

	_, err := s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_volume_attachment (uuid,
                                       storage_volume_uuid,
                                       net_node_uuid,
                                       life_id,
                                       provision_scope_id,
                                       block_device_uuid)
VALUES (?, ?, ?, 0, 0, ?)
`,
		attachmentUUID.String(),
		volumeUUID.String(),
		netNodeUUID.String(),
		blockDeviceUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID
}

// newPersistentModelVolume creates a new persistent volume in the model with
// model provision scope and associates it with the provided storage instance.
// The persistent flag is set to true. Returned is the uuid of the volume.
func (s *baseSuite) newPersistentModelVolume(
	c *tc.C,
	storageInstanceUUID domainstorage.StorageInstanceUUID,
) domainstorage.VolumeUUID {
	volumeUUID := tc.Must(c, domainstorage.NewVolumeUUID)
	volumeID := strconv.FormatUint(s.nextVolumeSequenceNumber(c), 10)

	_, err := s.DB().Exec(`
INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id, persistent)
VALUES (?, ?, 0, 0, 1)
	`,
		volumeUUID.String(), volumeID)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(`
INSERT INTO storage_instance_volume (storage_instance_uuid, storage_volume_uuid)
VALUES (?, ?)
	`,
		storageInstanceUUID.String(), volumeUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	return volumeUUID
}

// newStorageAttachment is responsible for establishing a new storage attachment
// in the model between the provided storage instance and unit.
func (s *baseSuite) newStorageAttachment(
	c *tc.C,
	storageInstanceUUID domainstorage.StorageInstanceUUID,
	unitUUID coreunit.UUID,
) domainstorage.StorageAttachmentUUID {
	saUUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)

	_, err := s.DB().Exec(`
INSERT INTO storage_attachment (uuid, storage_instance_uuid, unit_uuid, life_id)
VALUES (?, ?, ?, 0)
`,
		saUUID.String(), storageInstanceUUID.String(), unitUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)
	return saUUID
}

// newStorageInstanceUnitOwner sets the unit owner for the provided storage
// instance by inserting a row into the storage_unit_owner table.
func (s *baseSuite) newStorageInstanceUnitOwner(
	c *tc.C,
	storageInstanceUUID domainstorage.StorageInstanceUUID,
	unitUUID coreunit.UUID,
) {
	_, err := s.DB().Exec(`
INSERT INTO storage_unit_owner (unit_uuid, storage_instance_uuid)
VALUES (?, ?)`,
		unitUUID.String(), storageInstanceUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)
}

// newStoragePool creates a new storage pool with name, provider type and attrs.
// It returns the UUID of the new storage pool.
func (s *baseSuite) newStoragePool(
	c *tc.C, name string, providerType string, attrs map[string]string,
) domainstorage.StoragePoolUUID {
	spUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)

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

// nextStorageSequenceNumber retrieves the next sequence number in the storage
// namespace.
func (s *baseSuite) nextStorageSequenceNumber(c *tc.C) uint64 {
	var id uint64
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		id, err = domainsequencestate.NextValue(
			ctx, preparer{}, tx, domainsequence.StaticNamespace("storage"),
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return id
}

// nextVolumeSequenceNumber retrieves the next sequence number in the volume
// namespace.
func (s *baseSuite) nextVolumeSequenceNumber(c *tc.C) uint64 {
	var id uint64
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		id, err = domainsequencestate.NextValue(
			ctx, preparer{}, tx, domainsequence.StaticNamespace("volume"),
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return id
}

// newUnit is responsible for establishing a unit with in the model and
// returning the units uuid. This should only be used when the test just
// requires a unit in the model and no other parameters are required.
func (s *baseSuite) newUnit(
	c *tc.C,
) coreunit.UUID {
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmUUID := tc.Must(c, corecharm.NewID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	unitNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`
INSERT INTO charm (uuid, source_id, reference_name, revision, architecture_id)
VALUES (?, 0, ?, 1, 0)
`,
			charmUUID.String(), charmUUID.String(),
		)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(
			ctx,
			`
INSERT INTO application (uuid, charm_uuid, name, life_id, space_uuid)
VALUES (?, ?, ?, "0", ?)
`,
			appUUID.String(), charmUUID, appUUID.String(),
			corenetwork.AlphaSpaceId,
		)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(
			ctx,
			"INSERT INTO net_node VALUES (?)",
			unitNetNodeUUID.String(),
		)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(
			c.Context(), `
INSERT INTO unit (uuid, name, application_uuid, charm_uuid, net_node_uuid, life_id)
VALUES (?, ?, ?, ?, ?, 0)
`,
			unitUUID.String(),
			appUUID.String()+"/0",
			appUUID.String(),
			charmUUID.String(),
			unitNetNodeUUID.String(),
		)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	return unitUUID
}

// newUnitForApplication creates a new unit in the model for the supplied
// application uuid and returns the unit uuid and net node uuid.
func (s *baseSuite) newUnitForApplication(
	c *tc.C,
	appUUID coreapplication.UUID,
) (coreunit.UUID, string, domainnetwork.NetNodeUUID) {
	var charmUUID, appName string
	err := s.DB().QueryRowContext(
		c.Context(),
		"SELECT charm_uuid, name FROM application WHERE uuid = ?",
		appUUID.String(),
	).Scan(&charmUUID, &appName)
	c.Assert(err, tc.ErrorIsNil)

	unitUUID := tc.Must(c, coreunit.NewUUID)
	unitNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)

	var unitNum uint64
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		namespace := domainsequence.MakePrefixNamespace(
			domainapplication.ApplicationSequenceNamespace, appName,
		)
		var err error
		unitNum, err = domainsequencestate.NextValue(
			ctx, preparer{}, tx, namespace,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	unitName := appName + "/" + strconv.FormatUint(unitNum, 10)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			"INSERT INTO net_node VALUES (?)",
			unitNetNodeUUID.String(),
		)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(
			ctx, `
INSERT INTO unit (uuid, name, application_uuid, charm_uuid, net_node_uuid, life_id)
VALUES (?, ?, ?, ?, ?, 0)
`,
			unitUUID.String(),
			unitName,
			appUUID.String(),
			charmUUID,
			unitNetNodeUUID.String(),
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return unitUUID, unitName, unitNetNodeUUID
}

// newUnitForApplicationOnMachine creates a new unit in the model for the
// supplied application uuid and machine uuid. The unit is assigned to the
// machine by using the machine's net_node_uuid. Returns the unit uuid, unit
// name, and net node uuid.
func (s *baseSuite) newUnitForApplicationOnMachine(
	c *tc.C,
	appUUID coreapplication.UUID,
	machineUUID coremachine.UUID,
) (coreunit.UUID, string, domainnetwork.NetNodeUUID) {
	var charmUUID, appName string
	err := s.DB().QueryRowContext(
		c.Context(),
		"SELECT charm_uuid, name FROM application WHERE uuid = ?",
		appUUID.String(),
	).Scan(&charmUUID, &appName)
	c.Assert(err, tc.ErrorIsNil)

	var machineNetNodeUUID string
	err = s.DB().QueryRowContext(
		c.Context(),
		"SELECT net_node_uuid FROM machine WHERE uuid = ?",
		machineUUID.String(),
	).Scan(&machineNetNodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	unitUUID := tc.Must(c, coreunit.NewUUID)
	unitNetNodeUUID := domainnetwork.NetNodeUUID(machineNetNodeUUID)

	var unitNum uint64
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		namespace := domainsequence.MakePrefixNamespace(
			domainapplication.ApplicationSequenceNamespace, appName,
		)
		var err error
		unitNum, err = domainsequencestate.NextValue(
			ctx, preparer{}, tx, namespace,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	unitName := appName + "/" + strconv.FormatUint(unitNum, 10)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err = tx.ExecContext(
			ctx, `
INSERT INTO unit (uuid, name, application_uuid, charm_uuid, net_node_uuid, life_id)
VALUES (?, ?, ?, ?, ?, 0)
`,
			unitUUID.String(),
			unitName,
			appUUID.String(),
			charmUUID,
			unitNetNodeUUID.String(),
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return unitUUID, unitName, unitNetNodeUUID
}

// setBlockDeviceLinks populates the block_device_link_device table with the
// supplied device link names for the given block device. The machine uuid is
// discovered by querying the block_device table.
func (s *baseSuite) setBlockDeviceLinks(
	c *tc.C,
	blockDeviceUUID domainblockdevice.BlockDeviceUUID,
	deviceLinks []string,
) {
	var machineUUID string
	err := s.DB().QueryRowContext(
		c.Context(),
		"SELECT machine_uuid FROM block_device WHERE uuid = ?",
		blockDeviceUUID.String(),
	).Scan(&machineUUID)
	c.Assert(err, tc.ErrorIsNil)

	for _, deviceLink := range deviceLinks {
		_, err := s.DB().Exec(
			`INSERT INTO block_device_link_device (block_device_uuid, machine_uuid, name) VALUES (?, ?, ?)`,
			blockDeviceUUID, machineUUID, deviceLink)
		c.Assert(err, tc.ErrorIsNil)
	}
}

// setFilesystemStatus sets the status, message and updated_at time on the
// provided filesystem by upserting into the storage_filesystem_status table.
func (s *baseSuite) setFilesystemStatus(
	c *tc.C,
	filesystemUUID domainstorage.FilesystemUUID,
	status domainstatus.StorageFilesystemStatusType,
	message string,
	at time.Time,
) {
	statusID, err := domainstatus.EncodeStorageFilesystemStatus(status)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_filesystem_status (filesystem_uuid, status_id, message, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT (filesystem_uuid) DO UPDATE SET
    status_id = excluded.status_id,
    message = excluded.message,
    updated_at = excluded.updated_at
`,
		filesystemUUID.String(), statusID, message, at,
	)
	c.Assert(err, tc.ErrorIsNil)
}

// setVolumeStatus sets the status, message and updated_at time on the provided
// volume by upserting into the storage_volume_status table.
func (s *baseSuite) setVolumeStatus(
	c *tc.C,
	volumeUUID domainstorage.VolumeUUID,
	status domainstatus.StorageVolumeStatusType,
	message string,
	at time.Time,
) {
	statusID, err := domainstatus.EncodeStorageVolumeStatus(status)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_volume_status (volume_uuid, status_id, message, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT (volume_uuid) DO UPDATE SET
    status_id = excluded.status_id,
    message = excluded.message,
    updated_at = excluded.updated_at
`,
		volumeUUID.String(), statusID, message, at,
	)
	c.Assert(err, tc.ErrorIsNil)
}

// Prepare proxies the call to [sqlair.Prepare] implementing the
// [github.com/juju/juju/domain.Preparer] interface.
func (p preparer) Prepare(query string, typeSamples ...any) (*sqlair.Statement, error) {
	return sqlair.Prepare(query, typeSamples...)
}
