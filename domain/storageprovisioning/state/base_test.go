// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	charmtesting "github.com/juju/juju/core/charm/testing"
	coremachine "github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/blockdevice"
	domainlife "github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domainsequence "github.com/juju/juju/domain/sequence"
	sequencestate "github.com/juju/juju/domain/sequence/state"
	domainstorage "github.com/juju/juju/domain/storage"
	storagetesting "github.com/juju/juju/domain/storage/testing"
	"github.com/juju/juju/domain/storageprovisioning"
	domaintesting "github.com/juju/juju/domain/storageprovisioning/testing"
	"github.com/juju/juju/internal/uuid"
)

// baseSuite defines a set of common helper methods for storage provisioning
// tests. Base suite does not seed a starting state and does not run any tests.
type baseSuite struct {
	schematesting.ModelSuite
}

// changeMachineLife is a utility function for updating the life value of a
// machine.
func (s *baseSuite) changeMachineLife(c *tc.C, machineUUID string, lifeID domainlife.Life) {
	_, err := s.DB().ExecContext(
		c.Context(),
		"UPDATE machine SET life_id = ? WHERE uuid = ?",
		int(lifeID),
		machineUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
}

// newApplication creates a new application in the model returning the uuid of
// the new application.
func (s *baseSuite) newApplication(c *tc.C, name string) (string, string) {
	appUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	charmUUID := s.newCharm(c)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err = tx.ExecContext(ctx, `
INSERT INTO application (uuid, charm_uuid, name, life_id, space_uuid)
VALUES (?, ?, ?, "0", ?)`, appUUID.String(), charmUUID, name, network.AlphaSpaceId)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return appUUID.String(), charmUUID
}

// newCharm creates a new charm in the model and returns the uuid for it.
func (s *baseSuite) newCharm(c *tc.C) string {
	charmUUID := charmtesting.GenCharmID(c)

	err := s.TxnRunner().StdTxn(
		c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, source_id, reference_name, revision, architecture_id)
VALUES (?, 0, ?, 1, 0)
`,
				charmUUID.String(), "foo",
			)
			if err != nil {
				return err
			}

			_, err = tx.ExecContext(ctx, `
INSERT INTO charm_metadata (charm_uuid, name)
VALUES (?, 'myapp')
`,
				charmUUID.String(),
			)
			return err
		})
	c.Assert(err, tc.ErrorIsNil)
	return charmUUID.String()
}

// newMachineWithNetNode creates a new machine in the model attached to the
// supplied net node. The newly created machines uuid is returned along with the
// name.
func (s *baseSuite) newMachineWithNetNode(
	c *tc.C, netNodeUUID domainnetwork.NetNodeUUID,
) (string, coremachine.Name) {
	machineUUID := machinetesting.GenUUID(c)
	name := "mfoo-" + machineUUID.String()

	_, err := s.DB().ExecContext(
		c.Context(),
		"INSERT INTO machine (uuid, name, net_node_uuid, life_id) VALUES (?, ?, ?, 0)",
		machineUUID.String(),
		name,
		netNodeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return machineUUID.String(), coremachine.Name(name)
}

func (s *baseSuite) newMachineCloudInstanceWithID(
	c *tc.C, machineUUID, id string,
) {
	_, err := s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO machine_cloud_instance (machine_uuid, life_id, instance_id)
VALUES (?, 0, ?)
`,
		machineUUID,
		id,
	)
	c.Assert(err, tc.ErrorIsNil)
}

// newMachineVolume creates a new volume in the model with machine
// provision scope. Returned is the uuid and volume id of the entity.
func (s *baseSuite) newMachineVolume(c *tc.C) (storageprovisioning.VolumeUUID, string) {
	vsUUID := domaintesting.GenVolumeUUID(c)

	vsID := fmt.Sprintf("foo/%s", vsUUID.String())

	_, err := s.DB().Exec(`
INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 1)
	`,
		vsUUID.String(), vsID)
	c.Assert(err, tc.ErrorIsNil)

	return vsUUID, vsID
}

// newMachineVolumeAttachment creates a new volume attachment that has
// machine provision scope. The attachment is associated with the provided
// volume uuid and net node uuid.
func (s *baseSuite) newMachineVolumeAttachment(
	c *tc.C,
	vsUUID storageprovisioning.VolumeUUID,
	netNodeUUID domainnetwork.NetNodeUUID,
) storageprovisioning.VolumeAttachmentUUID {
	attachmentUUID := domaintesting.GenVolumeAttachmentUUID(c)

	_, err := s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_volume_attachment (uuid,
                                       storage_volume_uuid,
                                       net_node_uuid,
                                       life_id,
                                       provision_scope_id)
VALUES (?, ?, ?, 0, 1)
`,
		attachmentUUID.String(), vsUUID.String(), netNodeUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID
}

func (s *baseSuite) newModelFilesystemAttachmentWithMount(
	c *tc.C,
	fsUUID storageprovisioning.FilesystemUUID,
	netNodeUUID domainnetwork.NetNodeUUID,
	mountPoint string,
	readOnly bool,
) storageprovisioning.FilesystemAttachmentUUID {
	attachmentUUID := domaintesting.GenFilesystemAttachmentUUID(c)

	_, err := s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_filesystem_attachment (uuid,
                                           storage_filesystem_uuid,
                                           net_node_uuid,
                                           life_id,
                                           mount_point,
                                           read_only,
                                           provision_scope_id)
VALUES (?, ?, ?, 0, ?, ?, 0)
`,
		attachmentUUID.String(),
		fsUUID,
		netNodeUUID.String(),
		mountPoint,
		readOnly,
	)
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID
}

// newModelFilesystem creates a new filesystem in the model with model
// provision scope. Return is the uuid and filesystem id of the entity.
func (s *baseSuite) newModelFilesystem(c *tc.C) (
	storageprovisioning.FilesystemUUID, string,
) {
	fsUUID := domaintesting.GenFilesystemUUID(c)

	fsID := fmt.Sprintf("foo/%s", fsUUID.String())

	_, err := s.DB().Exec(`
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 0)
	`,
		fsUUID.String(), fsID)
	c.Assert(err, tc.ErrorIsNil)

	return fsUUID, fsID
}

// newModelFilesystemAttachmentWithMount creates a new filesystem attachment
// that has model provision scope. The attachment is associated with the
// provided filesystem uuid and net node uuid. This will also set the mount
// point and readonly attributes of the filesystem attachment.
func (s *baseSuite) newModelFilesystemAttachmentWithMount(
	c *tc.C,
	fsUUID storageprovisioning.FilesystemUUID,
	netNodeUUID domainnetwork.NetNodeUUID,
	mountPoint string,
	readOnly bool,
) storageprovisioning.FilesystemAttachmentUUID {
	attachmentUUID := domaintesting.GenFilesystemAttachmentUUID(c)
	_, err := s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_filesystem_attachment (uuid,
                                           storage_filesystem_uuid,
                                           net_node_uuid,
                                           life_id,
                                           mount_point,
                                           read_only,
                                           provision_scope_id)
VALUES (?, ?, ?, 0, ?, ?, 0)
`,
		attachmentUUID.String(),
		fsUUID,
		netNodeUUID.String(),
		mountPoint,
		readOnly,
	)
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID
}

// newModelVolume creates a new volume in the model with model
// provision scope. Return is the uuid and volume id of the entity.
func (s *baseSuite) newModelVolume(c *tc.C) (storageprovisioning.VolumeUUID, string) {
	vsUUID := domaintesting.GenVolumeUUID(c)

	vsID := fmt.Sprintf("foo/%s", vsUUID.String())

	_, err := s.DB().Exec(`
INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 0)
	`,
		vsUUID.String(), vsID)
	c.Assert(err, tc.ErrorIsNil)

	return vsUUID, vsID
}

// newModelVolumeAttachment creates a new volume attachment that has
// model provision scope. The attachment is associated with the provided
// volume uuid and net node uuid.
func (s *baseSuite) newModelVolumeAttachment(
	c *tc.C,
	vsUUID storageprovisioning.VolumeUUID,
	netNodeUUID domainnetwork.NetNodeUUID,
) storageprovisioning.VolumeAttachmentUUID {
	attachmentUUID := domaintesting.GenVolumeAttachmentUUID(c)

	_, err := s.DB().ExecContext(
		c.Context(),
		`
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

	_, err = s.DB().ExecContext(
		c.Context(),
		"INSERT INTO net_node VALUES (?)",
		nodeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return nodeUUID
}

func (s *baseSuite) getStorageID(
	c *tc.C, storageInstanceUUID domainstorage.StorageInstanceUUID,
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

func (s *baseSuite) newStorageAttachment(
	c *tc.C,
	storageInstanceUUID domainstorage.StorageInstanceUUID,
	unitUUID coreunit.UUID,
) storageprovisioning.StorageAttachmentUUID {
	saUUID := domaintesting.GenStorageAttachmentUUID(c)
	_, err := s.DB().Exec(`
INSERT INTO storage_attachment (uuid, storage_instance_uuid, unit_uuid, life_id)
VALUES (?, ?, ?, ?)
`, saUUID.String(), storageInstanceUUID.String(), unitUUID.String(), domainlife.Alive)
	c.Assert(err, tc.ErrorIsNil)
	return saUUID
}

func (s *baseSuite) newStorageInstanceForCharmWithPool(
	c *tc.C, charmUUID, poolUUID, storageName string,
) domainstorage.StorageInstanceUUID {
	storageInstanceUUID := storagetesting.GenStorageInstanceUUID(c)
	storageID := fmt.Sprintf("%s/%d", storageName, s.nextStorageSequenceNumber(c))

	var charmName string
	err := s.DB().QueryRowContext(
		c.Context(),
		"SELECT name FROM charm_metadata WHERE charm_uuid = ?",
		charmUUID,
	).Scan(&charmName)
	c.Assert(err, tc.ErrorIsNil)

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

	return storageInstanceUUID
}

func (s *baseSuite) newStorageInstanceBlockKindForCharmWithPool(
	c *tc.C, charmUUID, poolUUID, storageName string,
) domainstorage.StorageInstanceUUID {
	storageInstanceUUID := storagetesting.GenStorageInstanceUUID(c)
	storageID := fmt.Sprintf("%s/%d", storageName, s.nextStorageSequenceNumber(c))

	var charmName string
	err := s.DB().QueryRowContext(
		c.Context(),
		"SELECT name FROM charm_metadata WHERE charm_uuid = ?",
		charmUUID,
	).Scan(&charmName)
	c.Assert(err, tc.ErrorIsNil)

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

	return storageInstanceUUID
}

// nextStorageSequenceNumber retrieves the next sequence number in the storage
// namespace.
func (s *baseSuite) nextStorageSequenceNumber(
	c *tc.C,
) uint64 {
	var id uint64
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		id, err = sequencestate.NextValue(
			ctx, preparer{}, tx, domainsequence.StaticNamespace("storage"),
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return id
}

func (s *baseSuite) newStorageInstanceVolume(
	c *tc.C, instanceUUID domainstorage.StorageInstanceUUID,
	volumeUUID storageprovisioning.VolumeUUID,
) {
	ctx := c.Context()
	_, err := s.DB().ExecContext(ctx, `
INSERT INTO storage_instance_volume (storage_instance_uuid, storage_volume_uuid)
VALUES (?, ?)`, instanceUUID.String(), volumeUUID.String())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseSuite) newStorageInstanceFilesystem(
	c *tc.C, instanceUUID domainstorage.StorageInstanceUUID,
	filesystemUUID storageprovisioning.FilesystemUUID,
) {
	ctx := c.Context()
	_, err := s.DB().ExecContext(ctx, `
INSERT INTO storage_instance_filesystem (storage_instance_uuid, storage_filesystem_uuid)
VALUES (?, ?)`, instanceUUID.String(), filesystemUUID.String())
	c.Assert(err, tc.ErrorIsNil)
}

// newUnitWithNetNode creates a new unit in the model for the provided
// application uuid. The new unit will use the supplied net node. Returned is
// the new uuid of the unit and the name that was used.
func (s *baseSuite) newUnitWithNetNode(
	c *tc.C, unitName, appUUID string, netNodeUUID domainnetwork.NetNodeUUID,
) (coreunit.UUID, coreunit.Name) {
	var charmUUID string
	err := s.DB().QueryRowContext(
		c.Context(),
		"SELECT charm_uuid FROM application WHERE uuid = ?",
		appUUID,
	).Scan(&charmUUID)
	c.Assert(err, tc.ErrorIsNil)

	unitUUID := unittesting.GenUnitUUID(c)

	_, err = s.DB().ExecContext(
		c.Context(), `
INSERT INTO unit (uuid, name, application_uuid, charm_uuid, net_node_uuid, life_id)
VALUES (?, ?, ?, ?, ?, 0)
`,
		unitUUID.String(), unitName, appUUID, charmUUID, netNodeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return unitUUID, coreunit.Name(unitName)
}

func (s *baseSuite) newStorageOwner(
	c *tc.C, storageInstanceUUID domainstorage.StorageInstanceUUID, ownerUUID coreunit.UUID,
) {
	_, err := s.DB().Exec(`
INSERT INTO storage_unit_owner(unit_uuid, storage_instance_uuid)
VALUES (?, ?)`, ownerUUID.String(), storageInstanceUUID.String())
	c.Assert(err, tc.ErrorIsNil)
}

// newVolumeAttachmentPlan creates a new volume attachment plan. The attachment
// plan is associated with the provided volume uuid and net node uuid.
func (s *baseSuite) newVolumeAttachmentPlan(
	c *tc.C,
	volumeUUID storageprovisioning.VolumeUUID,
	netNodeUUID domainnetwork.NetNodeUUID,
) storageprovisioning.VolumeAttachmentPlanUUID {
	attachmentUUID := domaintesting.GenVolumeAttachmentPlanUUID(c)

	_, err := s.DB().Exec(`
INSERT INTO storage_volume_attachment_plan (uuid,
                                            storage_volume_uuid,
                                            net_node_uuid,
                                            life_id,
                                            provision_scope_id)
VALUES (?, ?, ?, 0, 1)
`,
		attachmentUUID.String(), volumeUUID.String(), netNodeUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID
}

// newStoragePool creates a new storage pool with name, provider type and attrs.
// It returns the UUID of the new storage pool.
func (s *baseSuite) newStoragePool(c *tc.C, name string, providerType string, attrs map[string]string) string {
	spUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
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

	return spUUID.String()
}

// newApplicationStorageDirective creates a new application storage directive.
func (s *baseSuite) newApplicationStorageDirective(c *tc.C,
	appUUID string, charmUUID string, storageName string, storagePoolUUID string,
	sizeMiB int64, count int,
) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO application_storage_directive (application_uuid, charm_uuid, storage_name, storage_pool_uuid, size_mib, count)
VALUES (?, ?, ?, ?, ?, ?)`, appUUID, charmUUID, storageName, storagePoolUUID, sizeMiB, count)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

// newCharmStorage creates a new charm storage for the given charm with fixed
// values for min/max count of 0 -> 10.
func (s *baseSuite) newCharmStorage(c *tc.C,
	charmUUID string, name string, kind string, readOnly bool, location string,
) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm_storage (charm_uuid, name, storage_kind_id, read_only, count_min, count_max, location)
VALUES (?, ?, (SELECT id FROM charm_storage_kind WHERE kind = ?), ?, 0, 10, ?)`, charmUUID, name, kind, readOnly, location)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

// newFilesystemCharmStorageWithLocationAndCount is a testing utility for
// creating new charm filesystem storage instance with location and count
// attributes.
func (s *baseSuite) newFilesystemCharmStorageWithLocationAndCount(
	c *tc.C, charmUUID, name, location string, countMin, countMax int,
) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`
INSERT INTO charm_storage (charm_uuid, name, storage_kind_id, read_only, count_min, count_max, location)
VALUES (?, ?, 1, false, ?, ?, ?)
`,
			charmUUID, name, countMin, countMax, location,
		)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

// newBlockDevice creates a new block device for the given machine.
func (s *baseSuite) newBlockDevice(
	c *tc.C,
	machineUUID string,
	name string,
	hardwareID string,
	busAddress string,
	deviceLinks []string,
) blockdevice.BlockDeviceUUID {
	uuid := tc.Must(c, blockdevice.NewBlockDeviceUUID)
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
	blockDeviceUUID blockdevice.BlockDeviceUUID,
	readOnly bool,
) {
	_, err := s.DB().Exec(
		`UPDATE storage_volume_attachment SET block_device_uuid=?, read_only=? WHERE uuid=?`,
		blockDeviceUUID, readOnly, uuid)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseSuite) changeVolumeAttachmentPlanInfo(
	c *tc.C,
	uuid storageprovisioning.VolumeAttachmentPlanUUID,
	deviceType storageprovisioning.PlanDeviceType,
	deviceAttrs map[string]string,
) {
	_, err := s.DB().Exec(
		`UPDATE storage_volume_attachment_plan SET device_type_id=? WHERE uuid=?`,
		deviceType, uuid)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().Exec(
		`DELETE FROM storage_volume_attachment_plan_attr WHERE attachment_plan_uuid=?`,
		uuid)
	c.Assert(err, tc.ErrorIsNil)
	for k, v := range deviceAttrs {
		_, err := s.DB().Exec(
			`INSERT INTO storage_volume_attachment_plan_attr(attachment_plan_uuid, "key", value) VALUES(?, ?, ?)`,
			uuid, k, v)
		c.Assert(err, tc.ErrorIsNil)
	}
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

func (s *baseSuite) changeVolumeProviderID(
	c *tc.C,
	uuid storageprovisioning.VolumeUUID,
	providerID string,
) {
	_, err := s.DB().Exec(
		`UPDATE storage_volume SET provider_id=? WHERE uuid=?`,
		providerID, uuid)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseSuite) removeVolumeWithObliterateValue(
	c *tc.C,
	uuid storageprovisioning.VolumeUUID,
	obliterateValue bool,
) {
	_, err := s.DB().Exec(
		`UPDATE storage_volume SET life_id=?, obliterate_on_cleanup=? WHERE uuid=?`,
		domainlife.Dying, obliterateValue, uuid)
	c.Assert(err, tc.ErrorIsNil)
}

type preparer struct{}

func (p preparer) Prepare(query string, typeSamples ...any) (*sqlair.Statement, error) {
	return sqlair.Prepare(query, typeSamples...)
}
