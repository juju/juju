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

type preparer struct{}

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

func (s *baseSuite) newStorageInstance(c *tc.C) domainstorage.StorageInstanceUUID {
	return s.newStorageInstanceWithProviderType(c, "rootfs")
}

func (s *baseSuite) newStorageInstanceWithProviderType(
	c *tc.C, pType string,
) domainstorage.StorageInstanceUUID {
	charmUUID := s.newCharm(c)
	return s.newStorageInstanceForCharmWithProviderType(c, charmUUID, pType)
}

func (s *baseSuite) newStorageInstanceForCharmWithProviderType(
	c *tc.C, charmUUID string, pType string,
) domainstorage.StorageInstanceUUID {
	storageInstanceUUID := storagetesting.GenStorageInstanceUUID(c)
	storageName := "mystorage"
	storageID := fmt.Sprintf("%s/%d", storageName, s.nextStorageSequenceNumber(c))

	err := s.TxnRunner().StdTxn(
		c.Context(),
		func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `
INSERT INTO charm_storage (charm_uuid, name, storage_kind_id, count_min, count_max)
VALUES (?, ?, 0, 0, 1)
`,
				charmUUID, storageName,
			)
			if err != nil {
				return err
			}

			_, err = tx.ExecContext(ctx, `
INSERT INTO storage_instance(uuid, charm_uuid, storage_name, storage_id, life_id, requested_size_mib, storage_type)
VALUES (?, ?, ?, ?, 0, 100, ?)
`,
				storageInstanceUUID.String(),
				charmUUID,
				storageName,
				storageID,
				pType,
			)
			return err
		})
	c.Assert(err, tc.ErrorIsNil)

	return storageInstanceUUID
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
	life domainlife.Life,
) {
	err := s.TxnRunner().StdTxn(
		c.Context(),
		func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `
INSERT INTO storage_attachment (storage_instance_uuid, unit_uuid, life_id)
VALUES (?, ?, ?)
`, storageInstanceUUID.String(), unitUUID.String(), life)
			return err
		},
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseSuite) newStorageInstanceWithPool(
	c *tc.C, poolUUID string,
) domainstorage.StorageInstanceUUID {
	charmUUID := s.newCharm(c)
	return s.newStorageInstanceForCharmWithPool(c, charmUUID, poolUUID)
}

func (s *baseSuite) newStorageInstanceForCharmWithPool(
	c *tc.C, charmUUID, poolUUID string,
) domainstorage.StorageInstanceUUID {
	storageInstanceUUID := storagetesting.GenStorageInstanceUUID(c)
	storageName := "mystorage"
	storageID := fmt.Sprintf("%s/%d", storageName, s.nextStorageSequenceNumber(c))

	err := s.TxnRunner().StdTxn(
		c.Context(),
		func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `
INSERT INTO charm_storage (charm_uuid, name, storage_kind_id, count_min, count_max)
VALUES (?, ?, 0, 0, 1)
`,
				charmUUID, storageName,
			)
			if err != nil {
				return err
			}

			_, err = tx.ExecContext(ctx, `
INSERT INTO storage_instance(uuid, charm_uuid, storage_name, storage_id, life_id, requested_size_mib, storage_pool_uuid)
VALUES (?, ?, ?, ?, 0, 100, ?)
`,
				storageInstanceUUID.String(),
				charmUUID,
				storageName,
				storageID,
				poolUUID,
			)
			return err
		})
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
	c *tc.C, name, appUUID string, netNodeUUID domainnetwork.NetNodeUUID,
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
		unitUUID.String(), name, appUUID, charmUUID, netNodeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return unitUUID, coreunit.Name(name)
}

// newVolumeAttachmentPlan creates a new volume attachment plan. The attachment
// plan is associated with the provided volume uuid and net node uuid.
func (s *baseSuite) newVolumeAttachmentPlan(
	c *tc.C,
	volumeUUID storageprovisioning.VolumeUUID,
	netNodeUUID domainnetwork.NetNodeUUID,
) string {
	attachmentUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_volume_attachment_plan (uuid,
                                            storage_volume_uuid,
                                            net_node_uuid,
                                            life_id,
                                            provision_scope_id)
VALUES (?, ?, ?, 0, 1)
`,
		attachmentUUID.String(), volumeUUID.String(), netNodeUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID.String()
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
// Only one of storagePoolUUID or storageType can be specified.
func (s *baseSuite) newApplicationStorageDirective(c *tc.C,
	appUUID string, charmUUID string, storageName string, storagePoolUUID string,
	storageType string, sizeMiB int64, count int,
) {
	var storagePoolUUIDArg sql.NullString
	if storagePoolUUID != "" {
		storagePoolUUIDArg.String = storagePoolUUID
		storagePoolUUIDArg.Valid = true
	}
	var storageTypeArg sql.NullString
	if storageType != "" {
		storageTypeArg.String = storageType
		storageTypeArg.Valid = true
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO application_storage_directive (application_uuid, charm_uuid, storage_name, storage_pool_uuid, storage_type, size_mib, count)
VALUES (?, ?, ?, ?, ?, ?, ?)`, appUUID, charmUUID, storageName, storagePoolUUIDArg, storageTypeArg, sizeMiB, count)
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

func (p preparer) Prepare(query string, typeSamples ...any) (*sqlair.Statement, error) {
	return sqlair.Prepare(query, typeSamples...)
}
