// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	coremachine "github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	"github.com/juju/juju/core/network"
	storagetesting "github.com/juju/juju/core/storage/testing"
	coreunit "github.com/juju/juju/core/unit"
	domainlife "github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domainsequence "github.com/juju/juju/domain/sequence"
	sequencestate "github.com/juju/juju/domain/sequence/state"
	"github.com/juju/juju/internal/uuid"
)

// baseSuite defines a set of common helper methods for storage provisioning
// tests. Base suite does not seed a starting state and does not run any tests.
type baseSuite struct {
	schematesting.ModelSuite
}

func (s *baseSuite) nextSequenceNumber(c *tc.C, ctx context.Context, namespace domainsequence.StaticNamespace) uint64 {
	st := NewState(s.TxnRunnerFactory())
	db, err := st.DB()
	c.Assert(err, tc.ErrorIsNil)

	var id uint64
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		id, err = sequencestate.NextValue(ctx, st, tx, namespace)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return id
}

// newMachineWithNetNode creates a new machine in the model attached to the
// supplied net node. The newly created machines uuid is returned along with the
// name.
func (s *baseSuite) newMachineWithNetNode(c *tc.C, netNodeUUID string) (string, coremachine.Name) {
	machineUUID := machinetesting.GenUUID(c)
	name := "mfoo-" + machineUUID.String()

	_, err := s.DB().ExecContext(
		c.Context(),
		"INSERT INTO machine (uuid, name, net_node_uuid, life_id) VALUES (?, ?, ?, 0)",
		machineUUID.String(),
		name,
		netNodeUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	return machineUUID.String(), coremachine.Name(name)
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

// newNetNode creates a new net node in the model for referencing to storage
// entity attachments. The net node is not associated with any machine or units.
func (s *baseSuite) newNetNode(c *tc.C) string {
	nodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		"INSERT INTO net_node VALUES (?)",
		nodeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return nodeUUID.String()
}

// newApplication creates a new application in the model returning the uuid of
// the new application.
func (s *baseSuite) newApplication(c *tc.C, name string) string {
	appUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, source_id, reference_name, revision, architecture_id)
VALUES (?, 0, ?, 1, 0)`, appUUID.String(), name)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_metadata (charm_uuid, name)
VALUES (?, 'myapp')`, appUUID.String())
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO application (uuid, charm_uuid, name, life_id, space_uuid)
VALUES (?, ?, ?, "0", ?)`, appUUID.String(), appUUID.String(), name, network.AlphaSpaceId)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return appUUID.String()
}

// newUnitWithNetNode creates a new unit in the model for the provided
// application uuid. The new unit will use the supplied net node. Returned is
// the new uuid of the unit and the name that was used.
func (s *baseSuite) newUnitWithNetNode(c *tc.C, name, appUUID, netNodeUUID string) (string, coreunit.Name) {
	var charmUUID string
	err := s.DB().QueryRowContext(
		c.Context(),
		"SELECT charm_uuid FROM application WHERE uuid = ?",
		appUUID,
	).Scan(&charmUUID)
	c.Assert(err, tc.ErrorIsNil)

	unitUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(), `
INSERT INTO unit (uuid, name, application_uuid, charm_uuid, net_node_uuid, life_id)
VALUES (?, ?, ?, ?, ?, 0)
`,
		unitUUID.String(), name, appUUID, charmUUID, netNodeUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	return unitUUID.String(), coreunit.Name(name)
}

func (s *baseSuite) newStorageInstance(c *tc.C, storageName, charmUUID string) string {
	ctx := c.Context()

	storageUUID := storagetesting.GenStorageUUID(c)
	poolUUID := uuid.MustNewUUID().String()
	storageID := fmt.Sprintf("%s/%d", storageName, s.nextSequenceNumber(c, ctx, domainsequence.StaticNamespace("storage")))
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_pool(uuid, name, type)
VALUES (?, ?, ?)
ON CONFLICT DO NOTHING`, poolUUID, "pool", "rootfs")
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_storage (charm_uuid, name, storage_kind_id, count_min, count_max)
VALUES (?, ?, ?, ?, ?)
		`, charmUUID, storageName, 0, 0, 1)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO storage_instance(uuid, charm_uuid, storage_name, storage_id, life_id, requested_size_mib, storage_pool_uuid)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
			storageUUID, charmUUID, storageName, storageID, domainlife.Alive, 100, poolUUID,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return storageUUID.String()
}

func (s *baseSuite) newStorageInstanceVolume(c *tc.C, instanceUUID, volumeUUID string) {
	ctx := c.Context()
	_, err := s.DB().ExecContext(ctx, `
INSERT INTO storage_instance_volume (storage_instance_uuid, storage_volume_uuid)
VALUES (?, ?)`, instanceUUID, volumeUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseSuite) newStorageInstanceFilesystem(c *tc.C, instanceUUID, filesystemUUID string) {
	ctx := c.Context()
	_, err := s.DB().ExecContext(ctx, `
INSERT INTO storage_instance_filesystem (storage_instance_uuid, storage_filesystem_uuid)
VALUES (?, ?)`, instanceUUID, filesystemUUID)
	c.Assert(err, tc.ErrorIsNil)
}

// changeVolumeLife is a utility function for updating the life value of a
// volume.
func (s *baseSuite) changeVolumeLife(
	c *tc.C, uuid string, life domainlife.Life,
) {
	_, err := s.DB().Exec(`
UPDATE storage_volume
SET    life_id = ?
WHERE  uuid = ?
`,
		int(life), uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// changeVolumeAttachmentLife is a utility function for updating the life
// value of a volume attachment.
func (s *baseSuite) changeVolumeAttachmentLife(
	c *tc.C, uuid string, life domainlife.Life,
) {
	_, err := s.DB().Exec(`
UPDATE storage_volume_attachment
SET    life_id = ?
WHERE  uuid = ?
`,
		int(life), uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// changeVolumeAttachmentPlanLife is a utility function for updating the life
// value of a volume attachment plan.
func (s *baseSuite) changeVolumeAttachmentPlanLife(
	c *tc.C, uuid string, life domainlife.Life,
) {
	_, err := s.DB().Exec(`
UPDATE storage_volume_attachment_plan
SET    life_id = ?
WHERE  uuid = ?
`,
		int(life), uuid)
	c.Assert(err, tc.ErrorIsNil)

}

// newMachineVolume creates a new volume in the model with machine
// provision scope. Returned is the uuid and volume id of the entity.
func (s *baseSuite) newMachineVolume(c *tc.C) (string, string) {
	vsUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	vsID := fmt.Sprintf("foo/%s", vsUUID.String())

	_, err = s.DB().Exec(`
INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 1)
	`,
		vsUUID.String(), vsID)
	c.Assert(err, tc.ErrorIsNil)

	return vsUUID.String(), vsID
}

// newModelVolume creates a new volume in the model with model
// provision scope. Return is the uuid and volume id of the entity.
func (s *baseSuite) newModelVolume(c *tc.C) (string, string) {
	vsUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	vsID := fmt.Sprintf("foo/%s", vsUUID.String())

	_, err = s.DB().Exec(`
INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 0)
	`,
		vsUUID.String(), vsID)
	c.Assert(err, tc.ErrorIsNil)

	return vsUUID.String(), vsID
}

// newMachineVolumeAttachment creates a new volume attachment that has
// machine provision scope. The attachment is associated with the provided
// volume uuid and net node uuid.
func (s *baseSuite) newMachineVolumeAttachment(
	c *tc.C, vsUUID string, netNodeUUID string,
) string {
	attachmentUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_volume_attachment (uuid,
                                       storage_volume_uuid,
                                       net_node_uuid,
                                       life_id,
                                       provision_scope_id)
VALUES (?, ?, ?, 0, 1)
`,
		attachmentUUID.String(), vsUUID, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID.String()
}

// newModelVolumeAttachment creates a new volume attachment that has
// model provision scope. The attachment is associated with the provided
// volume uuid and net node uuid.
func (s *baseSuite) newModelVolumeAttachment(
	c *tc.C, vsUUID string, netNodeUUID string,
) string {
	attachmentUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_volume_attachment (uuid,
                                       storage_volume_uuid,
                                       net_node_uuid,
                                       life_id,
                                       provision_scope_id)
VALUES (?, ?, ?, 0, 0)
`,
		attachmentUUID.String(), vsUUID, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID.String()
}

// newVolumeAttachmentPlan creates a new volume attachment plan. The attachment
// plan is associated with the provided volume uuid and net node uuid.
func (s *baseSuite) newVolumeAttachmentPlan(
	c *tc.C, volumeUUID, netNodeUUID string,
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
		attachmentUUID.String(), volumeUUID, netNodeUUID)
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
