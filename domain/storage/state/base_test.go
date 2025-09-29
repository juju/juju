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
	"github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	domainnetwork "github.com/juju/juju/domain/network"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domainsequence "github.com/juju/juju/domain/sequence"
	sequencestate "github.com/juju/juju/domain/sequence/state"
	"github.com/juju/juju/domain/storage"
	storagetesting "github.com/juju/juju/domain/storage/testing"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningtesting "github.com/juju/juju/domain/storageprovisioning/testing"
	"github.com/juju/juju/internal/uuid"
)

// baseSuite defines a set of common test suite fixtures for common
// storage operations during testing.
type baseSuite struct {
	schematesting.ModelSuite
}

type preparer struct{}

// newCharm creates a new charm in the model and returns the uuid for it.
func (s *baseSuite) newCharm(c *tc.C) corecharm.ID {
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

func (s *baseSuite) newCharmStorage(
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

// func (s *baseSuite) newFilesystem(c *tc.C) (
// 	storageprovisioning.FilesystemUUID, string,
// ) {
// 	fsUUID := storageprovisioningtesting.GenFilesystemUUID(c)

// 	fsID := fmt.Sprintf("foo/%s", fsUUID.String())

// 	_, err := s.DB().ExecContext(
// 		c.Context(),
// 		`
// INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provision_scope_id)
// VALUES (?, ?, 0, 0)
// 	`,
// 		fsUUID.String(), fsID)
// 	c.Assert(err, tc.ErrorIsNil)

// 	return fsUUID, fsID
// }

// func (s *baseSuite) newFilesystemWithStatus(
// 	c *tc.C,
// 	sType status.StorageFilesystemStatusType,
// ) (storageprovisioning.FilesystemUUID, string) {
// 	fsUUID, fsID := s.newFilesystem(c)

// 	_, err := s.DB().ExecContext(
// 		c.Context(),
// 		`
// INSERT INTO storage_filesystem_status (filesystem_uuid, status_id)
// VALUES (?, ?)
// `,
// 		fsUUID.String(), int(sType),
// 	)
// 	c.Assert(err, tc.ErrorIsNil)

// 	return fsUUID, fsID
// }

// nextStorageSequenceNumber retrieves the next sequence number in the storage
// namespace.
func (s *baseSuite) nextSequenceNumber(
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

func (s *baseSuite) newStorageInstance(
	c *tc.C,
	charmUUID corecharm.ID,
	storageName string,
	poolUUID storage.StoragePoolUUID,
	storageKind storage.StorageKind,
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
VALUES (?, ?, ?, ?, ?, 0, 100, ?)
`,
		storageInstanceUUID.String(),
		charmName,
		storageName,
		storageID,
		storageKind,
		poolUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	return storageInstanceUUID, storageID
}

// func (s *baseSuite) newStorageInstanceFilesystem(
// 	c *tc.C,
// 	instanceUUID storage.StorageInstanceUUID,
// 	filesystemUUID storageprovisioning.FilesystemUUID,
// ) {
// 	_, err := s.DB().ExecContext(
// 		c.Context(),
// 		`
// INSERT INTO storage_instance_filesystem (storage_instance_uuid, storage_filesystem_uuid)
// VALUES (?, ?)
// `,
// 		instanceUUID.String(),
// 		filesystemUUID.String(),
// 	)
// 	c.Assert(err, tc.ErrorIsNil)
// }

func (s *baseSuite) newStorageInstanceVolume(
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
func (s *baseSuite) newStoragePool(c *tc.C,
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
func (s *baseSuite) newVolume(c *tc.C) (storageprovisioning.VolumeUUID, string) {
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

func (p preparer) Prepare(query string, typeSamples ...any) (*sqlair.Statement, error) {
	return sqlair.Prepare(query, typeSamples...)
}

// // newVolumeWithStatus creates a new volume in the model with model
// // provision scope and it's initial status set.
// func (s *baseSuite) newVolumeWithStatus(
// 	c *tc.C,
// 	sType status.StorageVolumeStatusType,
// ) (storageprovisioning.VolumeUUID, string) {
// 	vsUUID, vsID := s.newVolume(c)

// 	_, err := s.DB().ExecContext(
// 		c.Context(),
// 		`
// INSERT INTO storage_volume_status(volume_uuid, status_id)
// VALUES (?, ?)
// `,
// 		vsUUID.String(), int(sType),
// 	)
// 	c.Assert(err, tc.ErrorIsNil)

// 	return vsUUID, vsID
// }

func (s *baseSuite) newBlockDevice(
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

func (s *baseSuite) changeVolumeAttachmentInfo(
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

func (s *baseSuite) changeVolumeInfo(
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

// func (s *baseSuite) changeFilesystemInfo(
// 	c *tc.C,
// 	uuid storageprovisioning.FilesystemUUID,
// 	providerID string,
// 	sizeMiB uint64,
// ) {
// 	_, err := s.DB().Exec(
// 		`UPDATE storage_filesystem SET provider_id=?, size_mib=? WHERE uuid=?`,
// 		providerID, sizeMiB, uuid)
// 	c.Assert(err, tc.ErrorIsNil)
// }

func (s *baseSuite) changeFilesystemAttachmentInfo(
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

// // newFilesystem creates a new filesystem in the model with model
// // provision scope. Return is the uuid and filesystem id of the entity.

// // newFilesystemAttachment creates a new filesystem attachment that has
// // model provision scope. The attachment is associated with the provided
// // filesystem uuid and net node uuid.
// func (s *baseSuite) newFilesystemAttachment(
// 	c *tc.C,
// 	fsUUID storageprovisioning.FilesystemUUID,
// 	netNodeUUID domainnetwork.NetNodeUUID,
// ) storageprovisioning.FilesystemAttachmentUUID {
// 	attachmentUUID := storageprovisioningtesting.GenFilesystemAttachmentUUID(c)

// 	_, err := s.DB().Exec(`
// INSERT INTO storage_filesystem_attachment (uuid,
//                                            storage_filesystem_uuid,
//                                            net_node_uuid,
//                                            life_id,
//                                            provision_scope_id)
// VALUES (?, ?, ?, 0, 0)
// `,
// 		attachmentUUID.String(), fsUUID, netNodeUUID.String())
// 	c.Assert(err, tc.ErrorIsNil)

// 	return attachmentUUID
// }

// // changeStorageInstanceLife is a utility function for updating the life
// // value of a storage instance.
// func (s *baseSuite) changeStorageInstanceLife(
// 	c *tc.C, uuid string, life life.Life,
// ) {
// 	_, err := s.DB().Exec(`
// UPDATE storage_instance
// SET    life_id=?
// WHERE  uuid=?
// `,
// 		int(life), uuid)
// 	c.Assert(err, tc.ErrorIsNil)
// }

// newApplication creates a new application in the model returning the uuid of
// the new application.
func (s *baseSuite) newApplication(c *tc.C, name string, charmUUID corecharm.ID) string {
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
func (s *baseSuite) newMachineWithNetNode(
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
func (s *baseSuite) newVolumeAttachment(
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
func (s *baseSuite) newNetNode(c *tc.C) domainnetwork.NetNodeUUID {
	nodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(
		"INSERT INTO net_node VALUES (?)",
		nodeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return nodeUUID
}

func (s *baseSuite) newStorageUnitOwner(c *tc.C, storageInstanceUUID storage.StorageInstanceUUID, unitUUID unit.UUID) {
	_, err := s.DB().Exec(`
INSERT INTO storage_unit_owner(storage_instance_uuid, unit_uuid)
VALUES (?, ?)
`,
		storageInstanceUUID, unitUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseSuite) newStorageAttachment(c *tc.C, storageInstanceUUID storage.StorageInstanceUUID, unitUUID unit.UUID) {
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
func (s *baseSuite) newUnitWithNetNode(
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

func (s *baseSuite) getStorageID(
	c *tc.C, storageInstanceUUID storage.StorageInstanceUUID,
) string {
	var storageID string
	err := s.DB().QueryRowContext(
		c.Context(), `
SELECT storage_id FROM storage_instance WHERE uuid = ?`,
		storageInstanceUUID.String(),
	).Scan(&storageID)
	c.Assert(err, tc.ErrorIsNil)
	return storageID
}
