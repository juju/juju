// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"testing"

	"github.com/juju/tc"

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

// assertChangeEvent asserts that a single change event exists for the provided
// namespace and changed value. If successful the matching change event will be
// deleted from the database so subsequent calls can be made to this func within
// a single test.
func (s *modelStorageSuite) assertChangeEvent(
	c *tc.C, namespace string, changed string,
) {
	row := s.DB().QueryRowContext(
		c.Context(),
		"SELECT id FROM change_log_namespace WHERE namespace = ?",
		namespace,
	)
	var nsID int
	err := row.Scan(&nsID)
	c.Assert(err, tc.ErrorIsNil)

	row = s.DB().QueryRowContext(
		c.Context(),
		`
SELECT COUNT(*)
FROM   change_log
WHERE  namespace_id = ?
AND    changed = ?
`,
		nsID, changed)

	var count int
	err = row.Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(count, tc.Equals, 1)

	_, err = s.DB().ExecContext(
		c.Context(),
		"DELETE FROM change_log WHERE namespace_id = ? AND changed = ?",
		nsID, changed,
	)
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
func (s *modelStorageSuite) TestUpdateModelFilesystemUpdate(c *tc.C) {
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
// with a machine provision filesystem and then an attachment that is model
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
