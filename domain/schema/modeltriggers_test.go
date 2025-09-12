// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	domainlife "github.com/juju/juju/domain/life"
	"github.com/juju/juju/internal/uuid"
)

// modelStorageSuite is a set of tests for asserting the behaviour of the
// storage provisioning triggers that exist in the model schema.
type modelStorageSuite struct {
	schemaBaseSuite
}

// TestModelStorageSuite registers the tests for the [modelStorageSuite].
func TestModelStorageSuite(t *testing.T) {
	tc.Run(t, &modelStorageSuite{})
}

// SetUpTest is responsible for setting up the model DDL so the storage triggers
// can be tested.
func (s *modelStorageSuite) SetUpTest(c *tc.C) {
	s.schemaBaseSuite.SetUpTest(c)
	s.applyDDL(c, ModelDDL())
}

func (s *modelStorageSuite) changeStorageAttachmentLife(
	c *tc.C, uuid string, life domainlife.Life,
) {
	_, err := s.DB().Exec(`
UPDATE storage_attachment
SET    life_id = ?
WHERE  uuid = ?
`,
		int(life), uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// changeFilesystemLife is a utility function for updating the life value of a
// filesystem.
func (s *modelStorageSuite) changeFilesystemLife(
	c *tc.C, uuid string, life domainlife.Life,
) {
	_, err := s.DB().Exec(`
UPDATE storage_filesystem
SET    life_id = ?
WHERE  uuid = ?
`,
		int(life), uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// changeVolumeLife is a utility function for updating the life value of a
// volume.
func (s *modelStorageSuite) changeVolumeLife(
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

// changeFilesystemAttachmentLife is a utility function for updating the life
// value of a filesystem attachment. This is used to trigger an update trigger
// for a filesystem attachment.
func (s *modelStorageSuite) changeFilesystemAttachmentLife(
	c *tc.C, uuid string, life domainlife.Life,
) {
	_, err := s.DB().Exec(`
UPDATE storage_filesystem_attachment
SET    life_id = ?
WHERE  uuid = ?
`,
		int(life), uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// changeVolumeAttachmentLife is a utility function for updating the life
// value of a volume attachment. This is used to trigger an update trigger
// for a volume attachment.
func (s *modelStorageSuite) changeVolumeAttachmentLife(
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
// value of a volume attachment plan. This is used to trigger an update trigger
// for a volume attachment plan.
func (s *modelStorageSuite) changeVolumeAttachmentPlanLife(
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

// deleteFilesystem is a utility function for deleting a filesystem in the
// model.
func (s *modelStorageSuite) deleteFilesystem(c *tc.C, uuid string) {
	_, err := s.DB().Exec(`
DELETE FROM storage_filesystem
WHERE  uuid = ?
`,
		uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// deleteFilesystemAttachment is a utility function for deleting a filesystem
// attachment in the model.
func (s *modelStorageSuite) deleteFilesystemAttachment(c *tc.C, uuid string) {
	_, err := s.DB().Exec(`
DELETE FROM storage_filesystem_attachment
WHERE  uuid = ?
`,
		uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// deleteVolume is a utility function for deleting a volume in the in the model.
func (s *modelStorageSuite) deleteVolume(c *tc.C, uuid string) {
	_, err := s.DB().Exec(`
DELETE FROM storage_volume
WHERE  uuid = ?
`,
		uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// deleteVolumeAttachment is a utility function for deleting a volume
// attachment in the model.
func (s *modelStorageSuite) deleteVolumeAttachment(c *tc.C, uuid string) {
	_, err := s.DB().Exec(`
DELETE FROM storage_volume_attachment
WHERE  uuid = ?
`,
		uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// deleteVolumeAttachmentPlan is a utility function for deleting a volume
// attachment plan in the model.
func (s *modelStorageSuite) deleteVolumeAttachmentPlan(c *tc.C, uuid string) {
	_, err := s.DB().Exec(`
DELETE FROM storage_volume_attachment_plan
WHERE  uuid = ?
`,
		uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// newMachineFilesystem creates a new filesystem in the model with machine
// provision scope. Returned is the uuid and filesystem id of the entity.
func (s *modelStorageSuite) newMachineFilesystem(c *tc.C) (string, string) {
	fsUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(`
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 1)
	`,
		fsUUID.String(), fsUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	return fsUUID.String(), fsUUID.String()
}

// newMachineVolume creates a new volume in the model with machine provision
// scope. Returned is the uuid and volume id of the entity.
func (s *modelStorageSuite) newMachineVolume(c *tc.C) (string, string) {
	vUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(`
INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 1)
	`,
		vUUID.String(), vUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	return vUUID.String(), vUUID.String()
}

// newModelFilesystem creates a new filesystem in the model with model
// provision scope. Return is the uuid and filesystem id of the entity.
func (s *modelStorageSuite) newModelFilesystem(c *tc.C) (string, string) {
	fsUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(`
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 0)
	`,
		fsUUID.String(), fsUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	return fsUUID.String(), fsUUID.String()
}

// newModelVolume creates a new volume in the model with model provision
// scope. Returned is the uuid and volume id of the entity.
func (s *modelStorageSuite) newModelVolume(c *tc.C) (string, string) {
	vUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(`
INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 0)
	`,
		vUUID.String(), vUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	return vUUID.String(), vUUID.String()
}

// newMachineFilesystemAttachment creates a new filesystem attachment that has
// machine provision scope. The attachment is associated with the provided
// filesystem uuid and net node uuid.
func (s *modelStorageSuite) newMachineFilesystemAttachment(
	c *tc.C, fsUUID string, netNodeUUID string,
) string {
	attachmentUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_filesystem_attachment (uuid,
										   storage_filesystem_uuid,
										   net_node_uuid,
										   life_id,
										   provision_scope_id)
VALUES (?, ?, ?, 0, 1)
`,
		attachmentUUID.String(), fsUUID, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID.String()
}

// newMachineVolumeAttachment creates a new volume attachment that has
// machine provision scope. The attachment is associated with the provided
// volume uuid and net node uuid.
func (s *modelStorageSuite) newMachineVolumeAttachment(
	c *tc.C, vUUID string, netNodeUUID string,
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
		attachmentUUID.String(), vUUID, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID.String()
}

func (s *modelStorageSuite) changeVolumeAttachmentBlockDevice(
	c *tc.C, attachmentUUID string, blockDeviceUUID string,
) {
	_, err := s.DB().Exec(`
UPDATE storage_volume_attachment
SET    block_device_uuid = ?
WHERE  uuid = ?`, blockDeviceUUID, attachmentUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelStorageSuite) changeBlockDeviceMountPoint(
	c *tc.C, blockDeviceUUID string, mountPoint string,
) {
	_, err := s.DB().Exec(`
UPDATE block_device
SET    mount_point = ?
WHERE  uuid = ?`, mountPoint, blockDeviceUUID)
	c.Assert(err, tc.ErrorIsNil)
}

// newMachineVolumeAttachmentPlan creates a new volume attachment plan that has
// machine provision scope. The attachment is associated with the provided
// volume uuid and net node uuid.
func (s *modelStorageSuite) newMachineVolumeAttachmentPlan(
	c *tc.C, vUUID string, netNodeUUID string,
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
		attachmentUUID.String(), vUUID, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID.String()
}

// newModelFilesystemAttachment creates a new filesystem attachment that has
// model provision scope. The attachment is associated with the provided
// filesystem uuid and net node uuid.
func (s *modelStorageSuite) newModelFilesystemAttachment(
	c *tc.C, fsUUID string, netNodeUUID string,
) string {
	attachmentUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_filesystem_attachment (uuid,
                                           storage_filesystem_uuid,
                                           net_node_uuid,
                                           life_id,
                                           provision_scope_id)
VALUES (?, ?, ?, 0, 0)
`,
		attachmentUUID.String(), fsUUID, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID.String()
}

// newModelVolumeAttachment creates a new volume attachment that has model
// provision scope. The attachment is associated with the provided volume uuid
// and net node uuid.
func (s *modelStorageSuite) newModelVolumeAttachment(
	c *tc.C, vUUID string, netNodeUUID string,
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
		attachmentUUID.String(), vUUID, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID.String()
}

// newNetNode creates a new net node in the model for referencing to storage
// entity attachments. The net node is not associated with any machine or units.
func (s *modelStorageSuite) newNetNode(c *tc.C) string {
	nodeUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		"INSERT INTO net_node VALUES (?)",
		nodeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return nodeUUID.String()
}

func (s *modelStorageSuite) newApplication(c *tc.C, name string) (string, string) {
	appUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	charmUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`
INSERT INTO charm (uuid, source_id, reference_name, revision, architecture_id)
VALUES (?, 0, ?, 1, 0)`, charmUUID.String(), "foo")
		if err != nil {
			return err
		}

		_, err = tx.Exec(`
INSERT INTO charm_metadata (charm_uuid, name)
VALUES (?, 'myapp')`, charmUUID.String())
		if err != nil {
			return err
		}

		_, err = tx.Exec(`
INSERT INTO application (uuid, charm_uuid, name, life_id, space_uuid)
VALUES (?, ?, ?, "0", ?)`, appUUID.String(), charmUUID.String(), name, network.AlphaSpaceId)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return appUUID.String(), charmUUID.String()
}

func (s *modelStorageSuite) newUnitWithNetNode(
	c *tc.C, name, appUUID, charmUUID string,
) (string, string) {
	unitUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	nodeUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err = s.DB().Exec(`
INSERT INTO net_node (uuid) VALUES (?)`, nodeUUID.String())
		if err != nil {
			return err
		}
		_, err = tx.Exec(`
INSERT INTO unit (uuid, name, application_uuid, charm_uuid, net_node_uuid, life_id)
VALUES (?, ?, ?, ?, ?, 0)`,
			unitUUID.String(), name, appUUID, charmUUID, nodeUUID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return unitUUID.String(), nodeUUID.String()
}

func (s *modelStorageSuite) newStoragePool(c *tc.C, name string, providerType string, attrs map[string]string) string {
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

func (s *modelStorageSuite) newStorageInstanceVolume(
	c *tc.C, instanceUUID string, volumeUUID string,
) {
	_, err := s.DB().Exec(`
INSERT INTO storage_instance_volume (storage_instance_uuid, storage_volume_uuid)
VALUES (?, ?)`, instanceUUID, volumeUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelStorageSuite) newStorageInstanceWithCharmUUID(
	c *tc.C, charmUUID, poolUUID string,
) (string, string) {
	storageInstanceUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	seq := s.nextSeq()
	storageName := fmt.Sprintf("mystorage-%d", seq)
	storageID := fmt.Sprintf("mystorage/%d", seq)

	_, err = s.DB().Exec(`
INSERT INTO charm_storage (charm_uuid, name, storage_kind_id, count_min, count_max)
VALUES (?, ?, 0, 0, 1)`, charmUUID, storageName)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(`
INSERT INTO storage_instance(uuid, charm_uuid, storage_name, storage_id, life_id, requested_size_mib, storage_pool_uuid)
VALUES (?, ?, ?, ?, 0, 100, ?)`,
		storageInstanceUUID.String(),
		charmUUID,
		storageName,
		storageID,
		poolUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	return storageInstanceUUID.String(), storageID
}

func (s *modelStorageSuite) newStorageAttachment(
	c *tc.C,
	storageInstanceUUID string,
	unitUUID string,
) string {
	saUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().Exec(`
INSERT INTO storage_attachment (uuid, storage_instance_uuid, unit_uuid, life_id)
VALUES (?, ?, ?, 0)`,
		saUUID.String(), storageInstanceUUID, unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	return saUUID.String()
}

func (s *modelStorageSuite) deleteStorageAttachment(
	c *tc.C,
	storageAttachmentUUID string,
) {
	_, err := s.DB().Exec(`
DELETE FROM storage_attachment
WHERE  uuid = ?`,
		storageAttachmentUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelStorageSuite) newStorageInstanceFilesystem(
	c *tc.C,
	storageInstanceUUID string,
	storageFilesystemUUID string,
) {
	_, err := s.DB().Exec(`
INSERT INTO storage_instance_filesystem (storage_instance_uuid, storage_filesystem_uuid)
VALUES (?, ?)`,
		storageInstanceUUID, storageFilesystemUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelStorageSuite) newBlockDevice(
	c *tc.C,
	machineUUID string,
) string {
	blockDeviceUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(`
INSERT INTO block_device (uuid, name, machine_uuid)
VALUES (?, ?, ?)`, blockDeviceUUID.String(), blockDeviceUUID.String(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	return blockDeviceUUID.String()
}

func (s *modelStorageSuite) newBlockDeviceLinkDevice(
	c *tc.C,
	blockDeviceUUID string,
	machineUUID string,
) {
	_, err := s.DB().Exec(`
INSERT INTO block_device_link_device (block_device_uuid, machine_uuid, name)
VALUES (?, ?, ?)`, blockDeviceUUID, machineUUID, blockDeviceUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelStorageSuite) renameBlockDeviceLinkDevice(
	c *tc.C,
	blockDeviceUUID string,
	newName string,
) {
	_, err := s.DB().Exec(`
UPDATE block_device_link_device
SET name = ?
WHERE block_device_uuid = ?`, newName, blockDeviceUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelStorageSuite) deleteBlockDeviceLinkDevice(
	c *tc.C,
	blockDeviceUUID string,
) {
	_, err := s.DB().Exec(`
DELETE FROM block_device_link_device
WHERE block_device_uuid = ?`, blockDeviceUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelStorageSuite) newMachine(
	c *tc.C,
	nodeUUID string,
) string {
	machineUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	name := "mfoo-" + machineUUID.String()

	_, err = s.DB().Exec(`
INSERT INTO machine (uuid, name, net_node_uuid, life_id) VALUES (?, ?, ?, 0)`,
		machineUUID.String(), name, nodeUUID)
	c.Assert(err, tc.ErrorIsNil)
	return machineUUID.String()
}

// TestStorageProvisionScopeIDMachine tests that the assumed value of 1
// correctly corresponds to machine provision scope. If this test fails it means
// that all of the storage triggers need to be updated with the new value.
func (s *modelStorageSuite) TestStorageProvisionScopeIDMachine(c *tc.C) {
	var id int
	err := s.DB().QueryRowContext(
		c.Context(),
		"SELECT id FROM storage_provision_scope WHERE scope = 'machine'",
	).Scan(&id)
	c.Check(err, tc.ErrorIsNil)
	c.Check(id, tc.Equals, 1)
}

// TestStorageProvisionScopeIDModel tests that the assumed value of 0
// correctly corresponds to model provision scope. If this test fails it means
// that all of the storage triggers need to be updated with the new value.
func (s *modelStorageSuite) TestStorageProvisionScopeIDModel(c *tc.C) {
	var id int
	err := s.DB().QueryRowContext(
		c.Context(),
		"SELECT id FROM storage_provision_scope WHERE scope = 'model'",
	).Scan(&id)
	c.Check(err, tc.ErrorIsNil)
	c.Check(id, tc.Equals, 0)
}

// TestNewModelFilesystemTrigger tests that new model provision scoped
// filesystems results in one change event with the filesystem id.
func (s *modelStorageSuite) TestNewModelFilesystemTrigger(c *tc.C) {
	_, id := s.newModelFilesystem(c)
	s.assertChangeEvent(c, "storage_filesystem_life_model_provisioning", id)
}

// TestDeleteModelFilesystemTrigger tests that deleting a model provision scoped
// filesystem results in one change event with the filesystem id.
func (s *modelStorageSuite) TestDeleteModelFilesystemTrigger(c *tc.C) {
	fsUUID, id := s.newModelFilesystem(c)
	s.assertChangeEvent(c, "storage_filesystem_life_model_provisioning", id)

	s.deleteFilesystem(c, fsUUID)
	s.assertChangeEvent(c, "storage_filesystem_life_model_provisioning", id)
}

// TestUpdateModelFilesystemUpdate tests that updating the life of model
// provision scoped filesystem results in one change event with the filesystem
// id.
func (s *modelStorageSuite) TestUpdateModelFilesystemTrigger(c *tc.C) {
	uuid, id := s.newModelFilesystem(c)
	s.assertChangeEvent(c, "storage_filesystem_life_model_provisioning", id)

	s.changeFilesystemLife(c, uuid, domainlife.Dying)
	s.assertChangeEvent(c, "storage_filesystem_life_model_provisioning", id)
}

// TestNewMachineFilesystemTrigger tests that a new filesystem that is machine
// provision scoped does not immediately result in a change event. It is
// expected that the change event will only happen when the first attachment
// is made for the filesystem. This is because it is impossible for a machine
// provisioner to watch filesystems that have not been attached to anything yet.
//
// NOTE: It doesn't matter if the filesystem attachment itself is machine or
// model scope provisioned. What matters here is that the filesystem itself
// holds the machine provision scope.
func (s *modelStorageSuite) TestNewMachineFilesystemTrigger(c *tc.C) {
	uuid, _ := s.newMachineFilesystem(c)
	// We don't worry about asserting no change here. The change event assertion
	// at the end checks for a single change event.

	netnode := s.newNetNode(c)
	s.newMachineFilesystemAttachment(c, uuid, netnode)

	s.assertChangeEvent(
		c, "storage_filesystem_life_machine_provisioning", netnode,
	)
}

// TestNewMachineFilesystemTriggerModelAttachment tests that a new filesystem
// that is machine provision scoped does not immediately result in a change
// event. It is expected that the change event will only happen when the first
// attachment is made for the filesystem. In this case we will are testing that
// with a machine provisioned filesystem and then an attachment that is model
// scoped the change event still fires. This is an important distinction as it
// is the provisioning of the filesystem that matters and not the attachment in
// this case.
//
// We want to see here that the trigger is never tempted to consider the
// provisioning scope of the attachment.
func (s *modelStorageSuite) TestNewMachineFilesystemTriggerModelAttachment(c *tc.C) {
	uuid, _ := s.newMachineFilesystem(c)
	// We don't worry about asserting no change here. The change event assertion
	// at the end checks for a single change event.

	netnode := s.newNetNode(c)
	s.newModelFilesystemAttachment(c, uuid, netnode)

	s.assertChangeEvent(
		c, "storage_filesystem_life_machine_provisioning", netnode,
	)
}

// TestNewMachineFilesystemTriggerDouble tests that a new filesystem that is
// machine provision scoped only creates one event for two attachment inserts.
// Filesystems that are machine provision scoped only get an insert change
// event triggered when the first attachment is made. If multiple attachments
// are made we want to see that only one change event is created.
//
// NOTE: This is a contrived test of sorts as Juju doesn't outright support
// multiple attachments but our DDL does support it. It makes sense to be
// supported for shared storage down the line. This test is not about asserting
// the business logic of Juju but the constraints asserted by the DDL and the
// storage triggers.
func (s *modelStorageSuite) TestNewMachineFilesystemTriggerDouble(c *tc.C) {
	uuid, _ := s.newMachineFilesystem(c)
	netnode1 := s.newNetNode(c)
	netnode2 := s.newNetNode(c)
	s.newMachineFilesystemAttachment(c, uuid, netnode1)
	s.newMachineFilesystemAttachment(c, uuid, netnode2)

	s.assertChangeEvent(
		c, "storage_filesystem_life_machine_provisioning", netnode1,
	)
}

// TestUpdateMachineFilesystemTrigger tests that updating the life of a
// fileystem which is machine provision scoped results in one change event. The
// change is expected to be the net node uuid of the filesystem's attachment.
func (s *modelStorageSuite) TestUpdateMachineFilesystemTrigger(c *tc.C) {
	uuid, _ := s.newMachineFilesystem(c)
	netnode := s.newNetNode(c)
	s.newMachineFilesystemAttachment(c, uuid, netnode)

	// Consume and check the initial insert event.
	s.assertChangeEvent(
		c, "storage_filesystem_life_machine_provisioning", netnode,
	)

	s.changeFilesystemLife(c, uuid, domainlife.Dying)
	s.assertChangeEvent(
		c, "storage_filesystem_life_machine_provisioning", netnode,
	)
}

// TestUpdateMachineFilesystemTriggerMultiple tests that updating the life of
// a filesystem which is machine provision scoped results in one change event
// for every distinct net node uuid in the filesystem's attachments.
func (s *modelStorageSuite) TestUpdateMachineFilesystemTriggerMultiple(c *tc.C) {
	uuid, _ := s.newMachineFilesystem(c)
	netnode1 := s.newNetNode(c)
	netnode2 := s.newNetNode(c)
	s.newMachineFilesystemAttachment(c, uuid, netnode1)
	s.newMachineFilesystemAttachment(c, uuid, netnode2)

	// Consume and check the initial insert event.
	s.assertChangeEvent(
		c, "storage_filesystem_life_machine_provisioning", netnode1,
	)

	s.changeFilesystemLife(c, uuid, domainlife.Dying)
	s.assertChangeEvent(
		c, "storage_filesystem_life_machine_provisioning", netnode1,
	)
	s.assertChangeEvent(
		c, "storage_filesystem_life_machine_provisioning", netnode2,
	)
}

// TestDeleteMachineFilesystemTrigger tests that deleting a fileystem which is
// machine provision scoped results in one change event. The change is expected
// to be the net node uuid of the filesystem's last attachment.
func (s *modelStorageSuite) TestDeleteMachineFilesystemTrigger(c *tc.C) {
	uuid, _ := s.newMachineFilesystem(c)
	netnode := s.newNetNode(c)
	fsAttUUID := s.newMachineFilesystemAttachment(c, uuid, netnode)

	// Consume and check the initial insert event.
	s.assertChangeEvent(
		c, "storage_filesystem_life_machine_provisioning", netnode,
	)

	// Remove the last attachment for the filesystem is what triggers the
	// filesystem event.
	s.deleteFilesystemAttachment(c, fsAttUUID)
	s.assertChangeEvent(
		c, "storage_filesystem_life_machine_provisioning", netnode,
	)
}

// TestNewModelFilesystemAttachmentTrigger tests that a new filesystem
// attachment that is model provision scoped results in one change event for the
// uuid of the filesystem attachment.
func (s *modelStorageSuite) TestNewModelFilesystemAttachmentTrigger(c *tc.C) {
	// We prove here that no matter the provision scope of the filesystem it is
	// the provision scope of the attachment that matters for the trigger.
	uuid1, _ := s.newModelFilesystem(c)
	uuid2, _ := s.newMachineFilesystem(c)
	netnode := s.newNetNode(c)

	fsAttUUID := s.newModelFilesystemAttachment(c, uuid1, netnode)
	s.assertChangeEvent(
		c, "storage_filesystem_attachment_life_model_provisioning", fsAttUUID,
	)

	fsAttUUID = s.newModelFilesystemAttachment(c, uuid2, netnode)
	s.assertChangeEvent(
		c, "storage_filesystem_attachment_life_model_provisioning", fsAttUUID,
	)
}

// TestUpdateModelFilesystemAttachmentTrigger tests that updating the life of a
// filesystem attachment that is model provision scoped results in one change
// event for the filesystem attachment uuid.
func (s *modelStorageSuite) TestUpdateModelFilesystemAttachmentTrigger(c *tc.C) {
	uuid, _ := s.newModelFilesystem(c)
	netnode := s.newNetNode(c)

	fsAttUUID := s.newModelFilesystemAttachment(c, uuid, netnode)
	// Check and consume the initial insert event.
	s.assertChangeEvent(
		c, "storage_filesystem_attachment_life_model_provisioning", fsAttUUID,
	)

	s.changeFilesystemAttachmentLife(c, fsAttUUID, domainlife.Dying)
	s.assertChangeEvent(
		c, "storage_filesystem_attachment_life_model_provisioning", fsAttUUID,
	)
}

// TestDeleteModelFilesystemAttachmentTrigger tests that deleting a fileystem
// attachment that is model provision scoped results in one change event for
// filesystem attachment uuid.
func (s *modelStorageSuite) TestDeleteModelFilesystemAttachmentTrigger(c *tc.C) {
	uuid, _ := s.newModelFilesystem(c)
	netnode := s.newNetNode(c)

	fsAttUUID := s.newModelFilesystemAttachment(c, uuid, netnode)
	// Check and consume the initial insert event.
	s.assertChangeEvent(
		c, "storage_filesystem_attachment_life_model_provisioning", fsAttUUID,
	)

	s.deleteFilesystemAttachment(c, fsAttUUID)
	s.assertChangeEvent(
		c, "storage_filesystem_attachment_life_model_provisioning", fsAttUUID,
	)
}

// TestNewMachineFilesystemAttachmentTrigger tests that a new filesystem
// attachment that is machine provision scoped results in one change event for
// the net node of the attachment.
func (s *modelStorageSuite) TestNewMachineFilesystemAttachmentTrigger(c *tc.C) {
	// We prove here that no matter the provision scope of the filesystem it is
	// the provision scope of the attachment that matters for the trigger.
	uuid1, _ := s.newModelFilesystem(c)
	uuid2, _ := s.newMachineFilesystem(c)
	netnode := s.newNetNode(c)

	s.newMachineFilesystemAttachment(c, uuid1, netnode)
	s.assertChangeEvent(
		c, "storage_filesystem_attachment_life_machine_provisioning", netnode,
	)

	s.newMachineFilesystemAttachment(c, uuid2, netnode)
	s.assertChangeEvent(
		c, "storage_filesystem_attachment_life_machine_provisioning", netnode,
	)
}

// TestUpdateMachineFilesystemAttachmentTrigger tests that updating the life of
// a filesystem attachment that is machine provision scoped results in one
// change event with the net node of the attachment.
func (s *modelStorageSuite) TestUpdateMachineFilesystemAttachmentTrigger(c *tc.C) {
	uuid, _ := s.newMachineFilesystem(c)
	netnode := s.newNetNode(c)

	fsAttUUID := s.newMachineFilesystemAttachment(c, uuid, netnode)
	// Check and consume the initial insert event.
	s.assertChangeEvent(
		c, "storage_filesystem_attachment_life_machine_provisioning", netnode,
	)

	s.changeFilesystemAttachmentLife(c, fsAttUUID, domainlife.Dying)
	s.assertChangeEvent(
		c, "storage_filesystem_attachment_life_machine_provisioning", netnode,
	)
}

// TestDeleteMachineFilesystemAttachmentTrigger tests that deleting a fileystem
// attachment that is machine provision scoped results in one change event of
// the net node for the attachment.
func (s *modelStorageSuite) TestDeleteMachineFilesystemAttachmentTrigger(c *tc.C) {
	uuid, _ := s.newMachineFilesystem(c)
	netnode := s.newNetNode(c)

	fsAttUUID := s.newMachineFilesystemAttachment(c, uuid, netnode)
	// Check and consume the initial insert event.
	s.assertChangeEvent(
		c, "storage_filesystem_attachment_life_machine_provisioning", netnode,
	)

	s.deleteFilesystemAttachment(c, fsAttUUID)
	s.assertChangeEvent(
		c, "storage_filesystem_attachment_life_machine_provisioning", netnode,
	)
}

// TestNewModelVolumeTrigger tests that new model provision scoped volumes
// results in one change event with the volume id.
func (s *modelStorageSuite) TestNewModelVolumeTrigger(c *tc.C) {
	_, id := s.newModelVolume(c)
	s.assertChangeEvent(c, "storage_volume_life_model_provisioning", id)
}

// TestUpdateModelVolumeTrigger tests that updating the life of a model
// provision scoped volume results in one change event with the volume id.
func (s *modelStorageSuite) TestUpdateModelVolumeTrigger(c *tc.C) {
	uuid, id := s.newModelVolume(c)
	s.assertChangeEvent(c, "storage_volume_life_model_provisioning", id)

	s.changeVolumeLife(c, uuid, domainlife.Dying)
	s.assertChangeEvent(c, "storage_volume_life_model_provisioning", id)
}

// TestDeleteModelVolumeTrigger tests that deleting a model provision scoped
// volume results in one change event with the volume id.
func (s *modelStorageSuite) TestDeleteModelVolumeTrigger(c *tc.C) {
	uuid, id := s.newModelVolume(c)
	s.assertChangeEvent(c, "storage_volume_life_model_provisioning", id)

	s.deleteVolume(c, uuid)
	s.assertChangeEvent(c, "storage_volume_life_model_provisioning", id)
}

// TestNewMachineVolumeTrigger tests that a new volume that is machine
// provision scoped does not immediately result in a change event. It is
// expected that the change event will only happen when the first attachment
// is made for the volume. This is because it is impossible for a machine
// provisioner to watch volumes that have not been attached to anything yet.
//
// NOTE: It doesn't matter if the volume attachment itself is machine or
// model scope provisioned. What matters here is that the volume itself
// holds the machine provision scope.
func (s *modelStorageSuite) TestNewMachineVolumeTrigger(c *tc.C) {
	uuid, _ := s.newMachineVolume(c)
	// We don't worry about asserting change here. The change event assertion
	// at the end checks for a single change event.

	netnode := s.newNetNode(c)
	s.newMachineVolumeAttachment(c, uuid, netnode)

	s.assertChangeEvent(
		c, "storage_volume_life_machine_provisioning", netnode,
	)
}

// TestNewMachineVolumeTriggerModelAttachment tests that a new volume that is
// machine provision scoped does not immediately result in a change event. It is
// expected that the change event will only happen when the first
// attachment is made for the volume. In this case we are testing that with a
// machine provision volume and then an attachment that is model
// scoped the change event still fires. This is an important distinction as it
// is the provisioning of the volume that matters and not the attachment in
// this case.
//
// We want to see here that the trigger is never tempted to consider the
// provisioning scope of the attachment.
func (s *modelStorageSuite) TestNewMachineVolumeTriggerModelAttachment(c *tc.C) {
	uuid, _ := s.newMachineVolume(c)
	// We don't worry about asserting no change here. The change event assertion
	// at the end checks for a single change event.

	netnode := s.newNetNode(c)
	s.newModelVolumeAttachment(c, uuid, netnode)

	s.assertChangeEvent(
		c, "storage_volume_life_machine_provisioning", netnode,
	)
}

// TestNewMachineVolumeTriggerDouble tests that a new volume that is machine
// provision scoped only creates one event for two or more attachment inserts.
// Volumes that are machine provision scoped only get an insert change
// event triggered when the first attachment is made. If multiple attachments
// are made we want to see that only one change event is created.
//
// NOTE: This is a contrived test of sorts as Juju doesn't outright support
// multiple attachments but our DDL does support it. It makes sense to be
// supported for shared storage down the line. This test is not about asserting
// the business logic of Juju but the constraints asserted by the DDL and the
// storage triggers.
func (s *modelStorageSuite) TestNewMachineVolumeTriggerDouble(c *tc.C) {
	uuid, _ := s.newMachineVolume(c)
	netnode1 := s.newNetNode(c)
	netnode2 := s.newNetNode(c)
	s.newMachineVolumeAttachment(c, uuid, netnode1)
	s.newMachineVolumeAttachment(c, uuid, netnode2)

	s.assertChangeEvent(
		c, "storage_volume_life_machine_provisioning", netnode1,
	)
}

// TestUpdateMachineVolumeTrigger tests that updating the life of a volume which
// is machine provision scoped results in one change event. The change is
// expected to be the net node uuid of the volume's attachment.
func (s *modelStorageSuite) TestUpdateMachineVolumeTrigger(c *tc.C) {
	uuid, _ := s.newMachineVolume(c)
	netnode := s.newNetNode(c)
	s.newMachineVolumeAttachment(c, uuid, netnode)

	// Consume and check the initial insert event.
	s.assertChangeEvent(
		c, "storage_volume_life_machine_provisioning", netnode,
	)

	s.changeVolumeLife(c, uuid, domainlife.Dying)
	s.assertChangeEvent(
		c, "storage_volume_life_machine_provisioning", netnode,
	)
}

// TestUpdateMachineVolumeTriggerMultiple tests that updating the life of
// a volume which is machine provision scoped results in one change event
// for every distinct net node uuid in the volume's attachments.
func (s *modelStorageSuite) TestUpdateMachineVolumeTriggerMultiple(c *tc.C) {
	uuid, _ := s.newMachineVolume(c)
	netnode1 := s.newNetNode(c)
	netnode2 := s.newNetNode(c)
	s.newMachineVolumeAttachment(c, uuid, netnode1)
	s.newMachineVolumeAttachment(c, uuid, netnode2)

	// Consume and check the initial insert event.
	s.assertChangeEvent(
		c, "storage_volume_life_machine_provisioning", netnode1,
	)

	// We expect that when updating the volume a change event is generated for
	// each distinct net node uuid in the volume's attachments.
	s.changeVolumeLife(c, uuid, domainlife.Dying)
	s.assertChangeEvent(
		c, "storage_volume_life_machine_provisioning", netnode1,
	)
	s.assertChangeEvent(
		c, "storage_volume_life_machine_provisioning", netnode2,
	)
}

// TestDeleteMachineVolumeTrigger tests that deleting a volume which is machine
// provision scoped results in one change event. The change is expected to be
// the net node uuid of the volume's last attachment.
func (s *modelStorageSuite) TestDeleteMachineVolumeTrigger(c *tc.C) {
	uuid, _ := s.newMachineVolume(c)
	netnode := s.newNetNode(c)
	vaUUID := s.newMachineVolumeAttachment(c, uuid, netnode)

	// Consume and check the initial insert event.
	s.assertChangeEvent(
		c, "storage_volume_life_machine_provisioning", netnode,
	)

	s.deleteVolumeAttachment(c, vaUUID)
	s.assertChangeEvent(
		c, "storage_volume_life_machine_provisioning", netnode,
	)
}

// TestNewModelVolumeAttachmentTrigger tests that a new volume
// attachment that is model provision scoped results in one change event.
func (s *modelStorageSuite) TestNewModelVolumeAttachmentTrigger(c *tc.C) {
	// We prove here that no matter the provision scope of the volume it is
	// the provision scope of the attachment that matters for the trigger.
	uuid1, _ := s.newModelVolume(c)
	uuid2, _ := s.newMachineVolume(c)
	netnode := s.newNetNode(c)

	vAttUUID := s.newModelVolumeAttachment(c, uuid1, netnode)
	s.assertChangeEvent(
		c, "storage_volume_attachment_life_model_provisioning", vAttUUID,
	)

	vAttUUID = s.newModelVolumeAttachment(c, uuid2, netnode)
	s.assertChangeEvent(
		c, "storage_volume_attachment_life_model_provisioning", vAttUUID,
	)
}

// TestUpdateModelVolumeAttachmentTrigger tests that updating the life of a
// volume attachment that is model provision scoped results in one change
// event.
func (s *modelStorageSuite) TestUpdateModelVolumeAttachmentTrigger(c *tc.C) {
	uuid, _ := s.newModelVolume(c)
	netnode := s.newNetNode(c)

	vAttUUID := s.newModelVolumeAttachment(c, uuid, netnode)
	// Check and consume the initial insert event.
	s.assertChangeEvent(
		c, "storage_volume_attachment_life_model_provisioning", vAttUUID,
	)

	s.changeVolumeAttachmentLife(c, vAttUUID, domainlife.Dying)
	s.assertChangeEvent(
		c, "storage_volume_attachment_life_model_provisioning", vAttUUID,
	)
}

// TestDeleteModelVolumeAttachmentTrigger tests that deleting a volume
// attachment that is model provision scoped results in one change event.
func (s *modelStorageSuite) TestDeleteModelVolumeAttachmentTrigger(c *tc.C) {
	uuid, _ := s.newModelVolume(c)
	netnode := s.newNetNode(c)

	vAttUUID := s.newModelVolumeAttachment(c, uuid, netnode)
	// Check and consume the initial insert event.
	s.assertChangeEvent(
		c, "storage_volume_attachment_life_model_provisioning", vAttUUID,
	)

	s.deleteVolumeAttachment(c, vAttUUID)
	s.assertChangeEvent(
		c, "storage_volume_attachment_life_model_provisioning", vAttUUID,
	)
}

// TestNewMachineVolumeAttachmentTrigger tests that a new volume
// attachment that is machine provision scoped results in one change event.
func (s *modelStorageSuite) TestNewMachineVolumeAttachmentTrigger(c *tc.C) {
	// We prove here that no matter the provision scope of the volume it is
	// the provision scope of the attachment that matters for the trigger.
	uuid1, _ := s.newModelVolume(c)
	uuid2, _ := s.newMachineVolume(c)
	netnode := s.newNetNode(c)

	s.newMachineVolumeAttachment(c, uuid1, netnode)
	s.assertChangeEvent(
		c, "storage_volume_attachment_life_machine_provisioning", netnode,
	)

	s.newMachineVolumeAttachment(c, uuid2, netnode)
	s.assertChangeEvent(
		c, "storage_volume_attachment_life_machine_provisioning", netnode,
	)
}

// TestUpdateMachineVolumeAttachmentTrigger tests that updating the life of
// a volume attachment that is machine provision scoped results in one
// change event.
func (s *modelStorageSuite) TestUpdateMachineVolumeAttachmentTrigger(c *tc.C) {
	uuid, _ := s.newMachineVolume(c)
	netnode := s.newNetNode(c)

	vAttUUID := s.newMachineVolumeAttachment(c, uuid, netnode)
	// Check and consume the initial insert event.
	s.assertChangeEvent(
		c, "storage_volume_attachment_life_machine_provisioning", netnode,
	)

	s.changeVolumeAttachmentLife(c, vAttUUID, domainlife.Dying)
	s.assertChangeEvent(
		c, "storage_volume_attachment_life_machine_provisioning", netnode,
	)
}

// TestDeleteMachineVolumeAttachmentTrigger tests that deleting a volume
// attachment that is machine provision scoped results in one change event.
func (s *modelStorageSuite) TestDeleteMachineVolumeAttachmentTrigger(c *tc.C) {
	uuid, _ := s.newMachineVolume(c)
	netnode := s.newNetNode(c)

	vAttUUID := s.newMachineVolumeAttachment(c, uuid, netnode)
	// Check and consume the initial insert event.
	s.assertChangeEvent(
		c, "storage_volume_attachment_life_machine_provisioning", netnode,
	)

	s.deleteVolumeAttachment(c, vAttUUID)
	s.assertChangeEvent(
		c, "storage_volume_attachment_life_machine_provisioning", netnode,
	)
}

// TestNewMachineVolumeAttachmentPlanTrigger tests that a new volume
// attachment plan that is machine provision scoped results in one change event.
func (s *modelStorageSuite) TestNewMachineVolumeAttachmentPlanTrigger(c *tc.C) {
	// We prove here that no matter the provision scope of the volume it is
	// the provision scope of the attachment that matters for the trigger.
	uuid1, _ := s.newModelVolume(c)
	uuid2, _ := s.newMachineVolume(c)
	netnode := s.newNetNode(c)

	s.newMachineVolumeAttachmentPlan(c, uuid1, netnode)
	s.assertChangeEvent(
		c, "storage_volume_attachment_plan_life_machine_provisioning", netnode,
	)

	s.newMachineVolumeAttachmentPlan(c, uuid2, netnode)
	s.assertChangeEvent(
		c, "storage_volume_attachment_plan_life_machine_provisioning", netnode,
	)
}

// TestUpdateMachineVolumeAttachmentPlanTrigger tests that updating the life of
// a volume attachment plan that is machine provision scoped results in one
// change event.
func (s *modelStorageSuite) TestUpdateMachineVolumeAttachmentPlanTrigger(c *tc.C) {
	uuid, _ := s.newMachineVolume(c)
	netnode := s.newNetNode(c)

	vAttUUID := s.newMachineVolumeAttachmentPlan(c, uuid, netnode)
	// Check and consume the initial insert event.
	s.assertChangeEvent(
		c, "storage_volume_attachment_plan_life_machine_provisioning", netnode,
	)

	s.changeVolumeAttachmentPlanLife(c, vAttUUID, domainlife.Dying)
	s.assertChangeEvent(
		c, "storage_volume_attachment_plan_life_machine_provisioning", netnode,
	)
}

// TestDeleteMachineVolumeAttachmentPlanTrigger tests that deleting a volume
// attachment plan that is machine provision scoped results in one change event.
func (s *modelStorageSuite) TestDeleteMachineVolumeAttachmentPlanTrigger(c *tc.C) {
	uuid, _ := s.newMachineVolume(c)
	netnode := s.newNetNode(c)

	vAttUUID := s.newMachineVolumeAttachmentPlan(c, uuid, netnode)
	// Check and consume the initial insert event.
	s.assertChangeEvent(
		c, "storage_volume_attachment_plan_life_machine_provisioning", netnode,
	)

	s.deleteVolumeAttachmentPlan(c, vAttUUID)
	s.assertChangeEvent(
		c, "storage_volume_attachment_plan_life_machine_provisioning", netnode,
	)
}

func (s *modelStorageSuite) TestCustomStorageAttachmentLifecycleUpdate(c *tc.C) {
	appUUID, charmUUID := s.newApplication(c, "foo")
	unitUUID, _ := s.newUnitWithNetNode(c, "foo/0", appUUID, charmUUID)
	poolUUID := s.newStoragePool(c, "foo", "foo", nil)
	storageInstanceUUID, _ := s.newStorageInstanceWithCharmUUID(c, charmUUID, poolUUID)
	saUUID := s.newStorageAttachment(c, storageInstanceUUID, unitUUID)

	nsID := s.getNamespaceID(c, "custom_storage_attachment_entities_storage_attachment_uuid")
	s.clearChangeEvents(c, nsID, saUUID)

	s.changeStorageAttachmentLife(c, saUUID, domainlife.Dying)
	s.assertChangeEvent(
		c, "custom_storage_attachment_entities_storage_attachment_uuid", saUUID,
	)
}

func (s *modelStorageSuite) TestCustomStorageAttachmentLifecycleDelete(c *tc.C) {
	appUUID, charmUUID := s.newApplication(c, "foo")
	unitUUID, _ := s.newUnitWithNetNode(c, "foo/0", appUUID, charmUUID)
	poolUUID := s.newStoragePool(c, "foo", "foo", nil)
	storageInstanceUUID, _ := s.newStorageInstanceWithCharmUUID(c, charmUUID, poolUUID)
	saUUID := s.newStorageAttachment(c, storageInstanceUUID, unitUUID)

	nsID := s.getNamespaceID(c, "custom_storage_attachment_entities_storage_attachment_uuid")
	s.clearChangeEvents(c, nsID, saUUID)

	s.deleteStorageAttachment(c, saUUID)
	s.assertChangeEvent(
		c, "custom_storage_attachment_entities_storage_attachment_uuid", saUUID,
	)
}

func (s *modelStorageSuite) TestCustomStorageAttachmentStorageInstanceFilesystemInsert(c *tc.C) {
	appUUID, charmUUID := s.newApplication(c, "foo")
	unitUUID, _ := s.newUnitWithNetNode(c, "foo/0", appUUID, charmUUID)
	poolUUID := s.newStoragePool(c, "foo", "foo", nil)
	storageInstanceUUID, _ := s.newStorageInstanceWithCharmUUID(c, charmUUID, poolUUID)
	saUUID := s.newStorageAttachment(c, storageInstanceUUID, unitUUID)

	fsUUID, _ := s.newMachineFilesystem(c)

	nsID := s.getNamespaceID(c, "custom_storage_attachment_entities_storage_attachment_uuid")
	s.clearChangeEvents(c, nsID, saUUID)

	s.newStorageInstanceFilesystem(c, storageInstanceUUID, fsUUID)
	s.assertChangeEvent(
		c, "custom_storage_attachment_entities_storage_attachment_uuid", saUUID,
	)
}

func (s *modelStorageSuite) TestCustomStorageAttachmentStorageInstanceVolumeInsert(c *tc.C) {
	appUUID, charmUUID := s.newApplication(c, "foo")
	unitUUID, _ := s.newUnitWithNetNode(c, "foo/0", appUUID, charmUUID)
	poolUUID := s.newStoragePool(c, "foo", "foo", nil)
	storageInstanceUUID, _ := s.newStorageInstanceWithCharmUUID(c, charmUUID, poolUUID)
	saUUID := s.newStorageAttachment(c, storageInstanceUUID, unitUUID)

	volumeUUID, _ := s.newMachineVolume(c)

	nsID := s.getNamespaceID(c, "custom_storage_attachment_entities_storage_attachment_uuid")
	s.clearChangeEvents(c, nsID, saUUID)

	s.newStorageInstanceVolume(c, storageInstanceUUID, volumeUUID)
	s.assertChangeEvent(
		c, "custom_storage_attachment_entities_storage_attachment_uuid", saUUID,
	)
}

func (s *modelStorageSuite) TestCustomStorageAttachmentStorageVolumeAttachmentInsert(c *tc.C) {
	appUUID, charmUUID := s.newApplication(c, "foo")
	unitUUID, netNodeUUID := s.newUnitWithNetNode(c, "foo/0", appUUID, charmUUID)
	poolUUID := s.newStoragePool(c, "foo", "foo", nil)
	storageInstanceUUID, _ := s.newStorageInstanceWithCharmUUID(c, charmUUID, poolUUID)
	saUUID := s.newStorageAttachment(c, storageInstanceUUID, unitUUID)
	volumeUUID, _ := s.newMachineVolume(c)
	s.newStorageInstanceVolume(c, storageInstanceUUID, volumeUUID)

	nsID := s.getNamespaceID(c, "custom_storage_attachment_entities_storage_attachment_uuid")
	s.clearChangeEvents(c, nsID, saUUID)

	_ = s.newMachineVolumeAttachment(c, volumeUUID, netNodeUUID)
	s.assertChangeEvent(
		c, "custom_storage_attachment_entities_storage_attachment_uuid", saUUID,
	)
}

func (s *modelStorageSuite) TestCustomStorageAttachmentStorageVolumeAttachmentUpdate(c *tc.C) {
	appUUID, charmUUID := s.newApplication(c, "foo")
	unitUUID, netNodeUUID := s.newUnitWithNetNode(c, "foo/0", appUUID, charmUUID)
	poolUUID := s.newStoragePool(c, "foo", "foo", nil)
	storageInstanceUUID, _ := s.newStorageInstanceWithCharmUUID(c, charmUUID, poolUUID)
	saUUID := s.newStorageAttachment(c, storageInstanceUUID, unitUUID)
	volumeUUID, _ := s.newMachineVolume(c)
	s.newStorageInstanceVolume(c, storageInstanceUUID, volumeUUID)
	vaUUID := s.newMachineVolumeAttachment(c, volumeUUID, netNodeUUID)

	nsID := s.getNamespaceID(c, "custom_storage_attachment_entities_storage_attachment_uuid")
	s.clearChangeEvents(c, nsID, saUUID)

	s.changeVolumeAttachmentLife(c, vaUUID, domainlife.Dying)
	s.assertChangeEvent(
		c, "custom_storage_attachment_entities_storage_attachment_uuid", saUUID,
	)
}

func (s *modelStorageSuite) TestCustomStorageAttachmentStorageVolumeAttachmentDelete(c *tc.C) {
	appUUID, charmUUID := s.newApplication(c, "foo")
	unitUUID, netNodeUUID := s.newUnitWithNetNode(c, "foo/0", appUUID, charmUUID)
	poolUUID := s.newStoragePool(c, "foo", "foo", nil)
	storageInstanceUUID, _ := s.newStorageInstanceWithCharmUUID(c, charmUUID, poolUUID)
	saUUID := s.newStorageAttachment(c, storageInstanceUUID, unitUUID)
	volumeUUID, _ := s.newMachineVolume(c)
	s.newStorageInstanceVolume(c, storageInstanceUUID, volumeUUID)
	vaUUID := s.newMachineVolumeAttachment(c, volumeUUID, netNodeUUID)

	nsID := s.getNamespaceID(c, "custom_storage_attachment_entities_storage_attachment_uuid")
	s.clearChangeEvents(c, nsID, saUUID)

	s.deleteVolumeAttachment(c, vaUUID)
	s.assertChangeEvent(
		c, "custom_storage_attachment_entities_storage_attachment_uuid", saUUID,
	)
}

func (s *modelStorageSuite) TestCustomStorageAttachmentStorageFilesystemAttachmentInsert(c *tc.C) {
	appUUID, charmUUID := s.newApplication(c, "foo")
	unitUUID, netNodeUUID := s.newUnitWithNetNode(c, "foo/0", appUUID, charmUUID)
	poolUUID := s.newStoragePool(c, "foo", "foo", nil)
	storageInstanceUUID, _ := s.newStorageInstanceWithCharmUUID(c, charmUUID, poolUUID)
	saUUID := s.newStorageAttachment(c, storageInstanceUUID, unitUUID)
	fsUUID, _ := s.newMachineFilesystem(c)
	s.newStorageInstanceFilesystem(c, storageInstanceUUID, fsUUID)

	nsID := s.getNamespaceID(c, "custom_storage_attachment_entities_storage_attachment_uuid")
	s.clearChangeEvents(c, nsID, saUUID)

	_ = s.newMachineFilesystemAttachment(c, fsUUID, netNodeUUID)
	s.assertChangeEvent(
		c, "custom_storage_attachment_entities_storage_attachment_uuid", saUUID,
	)
}

func (s *modelStorageSuite) TestCustomStorageAttachmentStorageFilesystemAttachmentUpdate(c *tc.C) {
	appUUID, charmUUID := s.newApplication(c, "foo")
	unitUUID, netNodeUUID := s.newUnitWithNetNode(c, "foo/0", appUUID, charmUUID)
	poolUUID := s.newStoragePool(c, "foo", "foo", nil)
	storageInstanceUUID, _ := s.newStorageInstanceWithCharmUUID(c, charmUUID, poolUUID)
	saUUID := s.newStorageAttachment(c, storageInstanceUUID, unitUUID)
	fsUUID, _ := s.newMachineFilesystem(c)
	s.newStorageInstanceFilesystem(c, storageInstanceUUID, fsUUID)
	fsaUUID := s.newMachineFilesystemAttachment(c, fsUUID, netNodeUUID)

	nsID := s.getNamespaceID(c, "custom_storage_attachment_entities_storage_attachment_uuid")
	s.clearChangeEvents(c, nsID, saUUID)

	s.changeFilesystemAttachmentLife(c, fsaUUID, domainlife.Dying)
	s.assertChangeEvent(
		c, "custom_storage_attachment_entities_storage_attachment_uuid", saUUID,
	)
}

func (s *modelStorageSuite) TestCustomStorageAttachmentStorageFilesystemAttachmentDelete(c *tc.C) {
	appUUID, charmUUID := s.newApplication(c, "foo")
	unitUUID, netNodeUUID := s.newUnitWithNetNode(c, "foo/0", appUUID, charmUUID)
	poolUUID := s.newStoragePool(c, "foo", "foo", nil)
	storageInstanceUUID, _ := s.newStorageInstanceWithCharmUUID(c, charmUUID, poolUUID)
	saUUID := s.newStorageAttachment(c, storageInstanceUUID, unitUUID)
	fsUUID, _ := s.newMachineFilesystem(c)
	s.newStorageInstanceFilesystem(c, storageInstanceUUID, fsUUID)
	fsaUUID := s.newMachineFilesystemAttachment(c, fsUUID, netNodeUUID)

	nsID := s.getNamespaceID(c, "custom_storage_attachment_entities_storage_attachment_uuid")
	s.clearChangeEvents(c, nsID, saUUID)

	s.deleteFilesystemAttachment(c, fsaUUID)
	s.assertChangeEvent(
		c, "custom_storage_attachment_entities_storage_attachment_uuid", saUUID,
	)
}

func (s *modelStorageSuite) TestCustomStorageAttachmentBlockDeviceUpdate(c *tc.C) {
	appUUID, charmUUID := s.newApplication(c, "foo")
	unitUUID, netNodeUUID := s.newUnitWithNetNode(c, "foo/0", appUUID, charmUUID)
	poolUUID := s.newStoragePool(c, "foo", "foo", nil)
	storageInstanceUUID, _ := s.newStorageInstanceWithCharmUUID(c, charmUUID, poolUUID)
	saUUID := s.newStorageAttachment(c, storageInstanceUUID, unitUUID)
	volumeUUID, _ := s.newMachineVolume(c)
	s.newStorageInstanceVolume(c, storageInstanceUUID, volumeUUID)
	vaUUID := s.newMachineVolumeAttachment(c, volumeUUID, netNodeUUID)
	machineUUID := s.newMachine(c, netNodeUUID)
	blockDeviceUUID := s.newBlockDevice(c, machineUUID)
	s.changeVolumeAttachmentBlockDevice(c, vaUUID, blockDeviceUUID)

	nsID := s.getNamespaceID(c, "custom_storage_attachment_entities_storage_attachment_uuid")
	s.clearChangeEvents(c, nsID, saUUID)

	s.changeBlockDeviceMountPoint(c, blockDeviceUUID, "/mnt/foo")
	s.assertChangeEvent(
		c, "custom_storage_attachment_entities_storage_attachment_uuid", saUUID,
	)
}

func (s *modelStorageSuite) TestCustomStorageAttachmentBlockDeviceLinkDeviceInsert(c *tc.C) {
	appUUID, charmUUID := s.newApplication(c, "foo")
	unitUUID, netNodeUUID := s.newUnitWithNetNode(c, "foo/0", appUUID, charmUUID)
	poolUUID := s.newStoragePool(c, "foo", "foo", nil)
	storageInstanceUUID, _ := s.newStorageInstanceWithCharmUUID(c, charmUUID, poolUUID)
	saUUID := s.newStorageAttachment(c, storageInstanceUUID, unitUUID)
	volumeUUID, _ := s.newMachineVolume(c)
	s.newStorageInstanceVolume(c, storageInstanceUUID, volumeUUID)
	vaUUID := s.newMachineVolumeAttachment(c, volumeUUID, netNodeUUID)
	machineUUID := s.newMachine(c, netNodeUUID)
	blockDeviceUUID := s.newBlockDevice(c, machineUUID)
	s.changeVolumeAttachmentBlockDevice(c, vaUUID, blockDeviceUUID)

	nsID := s.getNamespaceID(c, "custom_storage_attachment_entities_storage_attachment_uuid")
	s.clearChangeEvents(c, nsID, saUUID)

	s.newBlockDeviceLinkDevice(c, blockDeviceUUID, machineUUID)
	s.assertChangeEvent(
		c, "custom_storage_attachment_entities_storage_attachment_uuid", saUUID,
	)
}

func (s *modelStorageSuite) TestCustomStorageAttachmentBlockDeviceLinkDeviceUpdate(c *tc.C) {
	appUUID, charmUUID := s.newApplication(c, "foo")
	unitUUID, netNodeUUID := s.newUnitWithNetNode(c, "foo/0", appUUID, charmUUID)
	poolUUID := s.newStoragePool(c, "foo", "foo", nil)
	storageInstanceUUID, _ := s.newStorageInstanceWithCharmUUID(c, charmUUID, poolUUID)
	saUUID := s.newStorageAttachment(c, storageInstanceUUID, unitUUID)
	volumeUUID, _ := s.newMachineVolume(c)
	s.newStorageInstanceVolume(c, storageInstanceUUID, volumeUUID)
	vaUUID := s.newMachineVolumeAttachment(c, volumeUUID, netNodeUUID)
	machineUUID := s.newMachine(c, netNodeUUID)
	blockDeviceUUID := s.newBlockDevice(c, machineUUID)
	s.changeVolumeAttachmentBlockDevice(c, vaUUID, blockDeviceUUID)
	s.newBlockDeviceLinkDevice(c, blockDeviceUUID, machineUUID)

	nsID := s.getNamespaceID(c, "custom_storage_attachment_entities_storage_attachment_uuid")
	s.clearChangeEvents(c, nsID, saUUID)

	s.renameBlockDeviceLinkDevice(c, blockDeviceUUID, "foo")
	s.assertChangeEvent(
		c, "custom_storage_attachment_entities_storage_attachment_uuid", saUUID,
	)
}

func (s *modelStorageSuite) TestCustomStorageAttachmentBlockDeviceLinkDeviceDelete(c *tc.C) {
	appUUID, charmUUID := s.newApplication(c, "foo")
	unitUUID, netNodeUUID := s.newUnitWithNetNode(c, "foo/0", appUUID, charmUUID)
	poolUUID := s.newStoragePool(c, "foo", "foo", nil)
	storageInstanceUUID, _ := s.newStorageInstanceWithCharmUUID(c, charmUUID, poolUUID)
	saUUID := s.newStorageAttachment(c, storageInstanceUUID, unitUUID)
	volumeUUID, _ := s.newMachineVolume(c)
	s.newStorageInstanceVolume(c, storageInstanceUUID, volumeUUID)
	vaUUID := s.newMachineVolumeAttachment(c, volumeUUID, netNodeUUID)
	machineUUID := s.newMachine(c, netNodeUUID)
	blockDeviceUUID := s.newBlockDevice(c, machineUUID)
	s.changeVolumeAttachmentBlockDevice(c, vaUUID, blockDeviceUUID)
	s.newBlockDeviceLinkDevice(c, blockDeviceUUID, machineUUID)

	nsID := s.getNamespaceID(c, "custom_storage_attachment_entities_storage_attachment_uuid")
	s.clearChangeEvents(c, nsID, saUUID)

	s.deleteBlockDeviceLinkDevice(c, blockDeviceUUID)
	s.assertChangeEvent(
		c, "custom_storage_attachment_entities_storage_attachment_uuid", saUUID,
	)
}
