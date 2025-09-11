// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"fmt"

	"github.com/juju/juju/core/database/schema"
)

// customModelTriggers returns the list of custom trigger patches to add to the
// model's schema.
func customModelTriggers() []func() schema.Patch {
	return []func() schema.Patch{
		// Setup triggers for lifecycle events on filesystems in the model that
		// are machine provisioned.
		storageLifeMachineProvisioningTrigger(
			"filesystem",
			customNamespaceStorageFilesystemLifeMachineProvisioning,
		),

		// Setup triggers for lifecycle events on filesystems in the model that
		// are model provisioned.
		storageLifeModelProvisioningTrigger(
			"filesystem",
			"filesystem_id",
			customNamespaceStorageFilesystemLifeModelProvisioning,
		),

		// Setup triggers for lifecycle events on filesystem attachments in the
		// model that are machine provisioned.
		storageAttachmentLifeMachineProvisioningTrigger(
			"filesystem_attachment",
			customNamespaceStorageFilesystemAttachmentLifeMachineProvisioning,
		),

		// Setup triggers for lifecycle events on filesystem attachments in the
		// model that are model provisioned.
		storageLifeModelProvisioningTrigger(
			"filesystem_attachment",
			"uuid",
			customNamespaceStorageFilesystemAttachmentLifeModelProvisioning,
		),

		// Setup triggers for lifecycle events on volumes in the model that are
		// machine provisioned.
		storageLifeMachineProvisioningTrigger(
			"volume",
			customNamespaceStorageVolumeLifeMachineProvisioning,
		),

		// Setup triggers for lifecycle events on volumes in the model that are
		// model provisioned.
		storageLifeModelProvisioningTrigger(
			"volume",
			"volume_id",
			customNamespaceStorageVolumeLifeModelProvisioning,
		),

		// Setup triggers for lifecycle events on volume attachments in the
		// model that are machine provisioned.
		storageAttachmentLifeMachineProvisioningTrigger(
			"volume_attachment",
			customNamespaceStorageVolumeAttachmentLifeMachineProvisioning,
		),

		// Setup triggers for lifecycle events on filesystem attachments in the
		// model that are model provisioned.
		storageLifeModelProvisioningTrigger(
			"volume_attachment",
			"uuid",
			customNamespaceStorageVolumeAttachmentLifeModelProvisioning,
		),

		// Setup triggers for lifecycle events on volume attachment plans in the
		// model that are machine provisioned.
		storageAttachmentLifeMachineProvisioningTrigger(
			"volume_attachment_plan",
			customNamespaceStorageVolumeAttachmentPlanLifeMachineProvisioning,
		),

		// Setup triggers for events on storage attachment related entities in the
		// model.
		storageAttachmentRelatedEntitiesTrigger(
			customNamespaceStorageAttachmentRelatedEntities,
		),

		// Setup trigger for operation task status set to aborting.
		operationTaskStatusAbortingTrigger(
			customNamespaceOperatingTaskStatusAborting,
		),
	}
}

// storageAttachmentLifeMachineProvisioningTrigger creates triggers for storage
// attachment entities in the model that get provisioned by a machine. The
// triggers created will update the change_log for the provided namespace. The
// change value used will be the net_node_uuid value of the new attachment
// record. With the exception of the delete trigger, which uses the old
// net_node_uuid value. The triggers will only create change events when the
// attachment entity has had a change in life.
//
// To be able to use this trigger for a storage entity, the entity table must:
// - have a net_node_uuid column describing the net node the attachment is to be
// attached to.
// - have a life_id column referenced to the life table.
func storageAttachmentLifeMachineProvisioningTrigger(
	storageAttachmentTable string,
	namespace int,
) func() schema.Patch {
	stmt := fmt.Sprintf(`
-- insert namespace for storage attachment entity change.
INSERT INTO change_log_namespace
VALUES (%[2]d,
	'storage_%[1]s_life_machine_provisioning',
	'lifecycle changes for storage %[1]s, that are machined provisioned');

-- insert trigger for storage attachment entity.
CREATE TRIGGER trg_log_storage_%[1]s_insert_life_machine_provisioning
AFTER INSERT ON storage_%[1]s
FOR EACH ROW
	WHEN NEW.provision_scope_id = 1
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, %[2]d, NEW.net_node_uuid, DATETIME('now'));
END;

-- update trigger for storage attachment entity.
CREATE TRIGGER trg_log_storage_%[1]s_update_life_machine_provisioning
AFTER UPDATE ON storage_%[1]s
FOR EACH ROW
	WHEN NEW.provision_scope_id = 1
	AND NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, %[2]d, NEW.net_node_uuid, DATETIME('now'));
END;

-- delete trigger for storage attachment entity. Note the use of the OLD value
-- in the trigger.
CREATE TRIGGER trg_log_storage_%[1]s_delete_life_machine_provisioning
AFTER DELETE ON storage_%[1]s
FOR EACH ROW
	WHEN OLD.provision_scope_id = 1
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, %[2]d, OLD.net_node_uuid, DATETIME('now'));
END;
`,
		storageAttachmentTable, namespace,
	)

	return func() schema.Patch { return schema.MakePatch(stmt) }
}

// storageLifeMachineProvisioningTrigger creates triggers for storage entities
// in the model that get provisioned by a machine. The triggers created will
// update the change_log for the provided namespace. The change value(s) used
// will be all of the distinct net_node_uuids that the storage entity is
// attached to. The triggers will only create change events when the
// storage entity has had a change in life.
//
// No change event is generated on an initial insert into the storage entity.
// This is because no machine can provision the entity until it is attached to
// a net node of a machine. Instead when the first attachment record is made
// for the storage entity the change event is generated.
//
// No change event is generated when the storage entity is deleted. This is
// because you can't have a net node attached to a storage entity that is being
// deleted. This would break the RI of the storage tables. Instead we emit the
// final change event when the last attachment record is deleted. It is the net
// node of the last attachment record that is emitted as the change value.
//
// To be able to use this trigger for a storage entity, the entity table must:
// - Have a child table with an _attachment suffix.
// - The child _attachment table must have a net_node_uuid column.
// - The child _attachment table must have a storage entity uuid column. The
// form of this will be `storage_<storage_table>_uuid`.
// - Have a life_id column referenced to the life table.
func storageLifeMachineProvisioningTrigger(
	storageTable string,
	namespace int,
) func() schema.Patch {
	stmt := fmt.Sprintf(`
-- insert namespace for storage entity change.
INSERT INTO change_log_namespace
VALUES (%[2]d,
        'storage_%[1]s_life_machine_provisioning',
		'lifecycle changes for storage %[1]s, that are machined provisioned');

-- insert trigger for storage entity attachment table. This assumes the storage
-- entity has a child table with an _attachment suffix.
CREATE TRIGGER trg_log_storage_%[1]s_insert_life_machine_provisioning_on_attachment
AFTER INSERT ON storage_%[1]s_attachment FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 1,
           %[2]d,
           NEW.net_node_uuid,
           DATETIME('now')
    FROM   storage_%[1]s s
    WHERE  1 == (SELECT COUNT(*)
                 FROM   storage_%[1]s_attachment
                 WHERE  storage_%[1]s_uuid = NEW.storage_%[1]s_uuid)
    AND    s.uuid = NEW.storage_%[1]s_uuid
    AND    s.provision_scope_id = 1;
END;

-- update trigger for storage entity.
CREATE TRIGGER trg_log_storage_%[1]s_update_life_machine_provisioning
AFTER UPDATE ON storage_%[1]s
FOR EACH ROW
	WHEN NEW.provision_scope_id = 1
	AND  NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT DISTINCT 2,
           			%[2]d,
           			a.net_node_uuid,
           			DATETIME('now')
    FROM  storage_%[1]s_attachment a
    WHERE storage_%[1]s_uuid = NEW.uuid;
END;

-- delete trigger for storage entity. Note the use of the OLD value in the
-- trigger.
CREATE TRIGGER trg_log_storage_%[1]s_delete_life_machine_provisioning_last_attachment
AFTER DELETE ON storage_%[1]s_attachment FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT DISTINCT 4,
           			%[2]d,
           			OLD.net_node_uuid,
           			DATETIME('now')
    FROM   storage_%[1]s s
    WHERE  0 == (SELECT COUNT(*)
                 FROM   storage_%[1]s_attachment
                 WHERE  storage_%[1]s_uuid = OLD.storage_%[1]s_uuid)
    AND    s.uuid = OLD.storage_%[1]s_uuid
    AND    s.provision_scope_id = 1;
END;
`,
		storageTable, namespace,
	)

	return func() schema.Patch { return schema.MakePatch(stmt) }
}

// storageLifeMachineProvisioningTrigger creates triggers for storage entities
// in the model that get provisioned by the model. The triggers created will
// update the change_log for the provided namespace. The change value(s) used
// will the value of the `changeColumn` column of the storage entity. The
// triggers will only create change events when the storage entity has had a
// change in life.
//
// To be able to use this trigger for a storage entity, the entity table must:
// - Have a life_id column referenced to the life table.
func storageLifeModelProvisioningTrigger(
	storageTable string,
	changeColumn string,
	namespace int,
) func() schema.Patch {
	stmt := fmt.Sprintf(`
-- insert namespace for storage entity
INSERT INTO change_log_namespace
VALUES (%[3]d,
		'storage_%[1]s_life_model_provisioning',
		'lifecycle changes for storage %[1]s, that are model provisioned');

-- insert trigger for storage entity.
CREATE TRIGGER trg_log_storage_%[1]s_insert_life_model_provisioning
AFTER INSERT ON storage_%[1]s
FOR EACH ROW
	WHEN NEW.provision_scope_id = 0
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, %[3]d, NEW.%[2]s, DATETIME('now'));
END;

-- update trigger for storage entity.
CREATE TRIGGER trg_log_storage_%[1]s_update_life_model_provisioning
AFTER UPDATE ON storage_%[1]s
FOR EACH ROW
	WHEN NEW.provision_scope_id = 0
	AND NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, %[3]d, NEW.%[2]s, DATETIME('now'));
END;

-- delete trigger for storage entity. Note the use of the OLD value in the
-- trigger.
CREATE TRIGGER trg_log_storage_%[1]s_delete_life_model_provisioning
AFTER DELETE ON storage_%[1]s
FOR EACH ROW
	WHEN OLD.provision_scope_id = 0
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, %[3]d, OLD.%[2]s, DATETIME('now'));
END;
`,
		storageTable, changeColumn, namespace,
	)

	return func() schema.Patch { return schema.MakePatch(stmt) }
}

// storageAttachmentRelatedEntitiesTrigger creates triggers for storage attachment
// related entities in the model. The triggers created will update the change_log
// for the provided namespace. The change value used will always be the relevant
// storage_attachment primary key(uuid).
func storageAttachmentRelatedEntitiesTrigger(namespace int) func() schema.Patch {
	stmt := fmt.Sprintf(`
-- insert namespace for storage attachment.
INSERT INTO change_log_namespace
VALUES (%[1]d,
		'custom_storage_attachment_entities_storage_attachment_uuid',
		'Changes for storage provisioning process');

-- storage_attachment for life update.
CREATE TRIGGER trg_log_custom_storage_attachment_lifecycle_update
AFTER UPDATE ON storage_attachment FOR EACH ROW
WHEN
	NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, %[1]d, NEW.uuid, DATETIME('now'));
END;

-- storage_attachment for delete.
CREATE TRIGGER trg_log_custom_storage_attachment_lifecycle_delete
AFTER DELETE ON storage_attachment FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, %[1]d, OLD.uuid, DATETIME('now'));
END;

-- storage_instance_filesystem for insert.
CREATE TRIGGER trg_log_custom_storage_attachment_storage_instance_filesystem_insert
AFTER INSERT ON storage_instance_filesystem FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 1, %[1]d, sa.uuid, DATETIME('now')
	FROM storage_attachment sa
	WHERE sa.storage_instance_uuid = NEW.storage_instance_uuid;
END;

-- storage_instance_volume for insert.
CREATE TRIGGER trg_log_custom_storage_attachment_storage_instance_volume_insert
AFTER INSERT ON storage_instance_volume FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 1, %[1]d, sa.uuid, DATETIME('now')
	FROM storage_attachment sa
	WHERE sa.storage_instance_uuid = NEW.storage_instance_uuid;
END;

-- storage_volume_attachment for insert.
CREATE TRIGGER trg_log_custom_storage_attachment_storage_volume_attachment_insert
AFTER INSERT ON storage_volume_attachment FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 1, %[1]d, sa.uuid, DATETIME('now')
	FROM storage_instance_volume siv
	JOIN storage_attachment sa ON sa.storage_instance_uuid = siv.storage_instance_uuid
	WHERE siv.storage_volume_uuid = NEW.storage_volume_uuid;
END;

-- storage_volume_attachment for update.
CREATE TRIGGER trg_log_custom_storage_attachment_storage_volume_attachment_update
AFTER UPDATE ON storage_volume_attachment FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 2, %[1]d, sa.uuid, DATETIME('now')
	FROM storage_instance_volume siv
	JOIN storage_attachment sa ON sa.storage_instance_uuid = siv.storage_instance_uuid
	WHERE siv.storage_volume_uuid = NEW.storage_volume_uuid;
END;

-- storage_volume_attachment for delete.
CREATE TRIGGER trg_log_custom_storage_attachment_storage_volume_attachment_delete
AFTER DELETE ON storage_volume_attachment FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 4, %[1]d, sa.uuid, DATETIME('now')
	FROM storage_instance_volume siv
	JOIN storage_attachment sa ON sa.storage_instance_uuid = siv.storage_instance_uuid
	WHERE siv.storage_volume_uuid = OLD.storage_volume_uuid;
END;

-- storage_filesystem_attachment for insert.
CREATE TRIGGER trg_log_custom_storage_attachment_storage_filesystem_attachment_insert
AFTER INSERT ON storage_filesystem_attachment FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 1, %[1]d, sa.uuid, DATETIME('now')
	FROM storage_instance_filesystem sif
	JOIN storage_attachment sa ON sa.storage_instance_uuid = sif.storage_instance_uuid
	WHERE sif.storage_filesystem_uuid = NEW.storage_filesystem_uuid;
END;

-- storage_filesystem_attachment for update.
CREATE TRIGGER trg_log_custom_storage_attachment_storage_filesystem_attachment_update
AFTER UPDATE ON storage_filesystem_attachment FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 2, %[1]d, sa.uuid, DATETIME('now')
	FROM storage_instance_filesystem sif
	JOIN storage_attachment sa ON sa.storage_instance_uuid = sif.storage_instance_uuid
	WHERE sif.storage_filesystem_uuid = NEW.storage_filesystem_uuid;
END;

-- storage_filesystem_attachment for delete.
CREATE TRIGGER trg_log_custom_storage_attachment_storage_filesystem_attachment_delete
AFTER DELETE ON storage_filesystem_attachment FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 4, %[1]d, sa.uuid, DATETIME('now')
	FROM storage_instance_filesystem sif
	JOIN storage_attachment sa ON sa.storage_instance_uuid = sif.storage_instance_uuid
	WHERE sif.storage_filesystem_uuid = OLD.storage_filesystem_uuid;
END;

-- block_device for update.
CREATE TRIGGER trg_log_custom_storage_attachment_block_device_update
AFTER UPDATE ON block_device FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 2, %[1]d, sa.uuid, DATETIME('now')
	FROM storage_volume_attachment sva
	JOIN storage_instance_volume siv ON siv.storage_volume_uuid = sva.storage_volume_uuid
	JOIN storage_attachment sa ON sa.storage_instance_uuid = siv.storage_instance_uuid
	WHERE sva.block_device_uuid = NEW.uuid;
END;

-- block_device_link_device for insert.
CREATE TRIGGER trg_log_custom_storage_attachment_block_device_link_device_insert
AFTER INSERT ON block_device_link_device FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 1, %[1]d, sa.uuid, DATETIME('now')
	FROM storage_volume_attachment sva
	JOIN storage_instance_volume siv ON siv.storage_volume_uuid = sva.storage_volume_uuid
	JOIN storage_attachment sa ON sa.storage_instance_uuid = siv.storage_instance_uuid
	WHERE sva.block_device_uuid = NEW.block_device_uuid;
END;

-- block_device_link_device for update.
CREATE TRIGGER trg_log_custom_storage_attachment_block_device_link_device_update
AFTER UPDATE ON block_device_link_device FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 2, %[1]d, sa.uuid, DATETIME('now')
	FROM storage_volume_attachment sva
	JOIN storage_instance_volume siv ON siv.storage_volume_uuid = sva.storage_volume_uuid
	JOIN storage_attachment sa ON sa.storage_instance_uuid = siv.storage_instance_uuid
	WHERE sva.block_device_uuid = NEW.block_device_uuid;
END;

-- block_device_link_device for delete.
CREATE TRIGGER trg_log_custom_storage_attachment_block_device_link_device_delete
AFTER DELETE ON block_device_link_device FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 4, %[1]d, sa.uuid, DATETIME('now')
	FROM storage_volume_attachment sva
	JOIN storage_instance_volume siv ON siv.storage_volume_uuid = sva.storage_volume_uuid
	JOIN storage_attachment sa ON sa.storage_instance_uuid = siv.storage_instance_uuid
	WHERE sva.block_device_uuid = OLD.block_device_uuid;
END;
`[1:], namespace)
	return func() schema.Patch { return schema.MakePatch(stmt) }
}

// operationTaskStatusAbortingTrigger creates a trigger for operation task's status
// values changing to ABORTING.
func operationTaskStatusAbortingTrigger(namespace int) func() schema.Patch {
	stmt := fmt.Sprintf(`
INSERT INTO change_log_namespace
VALUES (%[1]d,
        'custom_operation_task_status_aborting',
        'Operation task status changes to ABORTING');

CREATE TRIGGER trg_log_custom_operation_task_status_aborting
AFTER UPDATE ON operation_task_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 2, %[1]d, ots.task_uuid, DATETIME('now')
    FROM operation_task_status AS ots
    JOIN operation_task_status_value AS otsv ON ots.status_id = otsv.id
    WHERE otsv.status = 'aborting';
END;
`,
		namespace)
	return func() schema.Patch { return schema.MakePatch(stmt) }
}
