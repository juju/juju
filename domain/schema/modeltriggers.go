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

		// Setup trigger for operation task status changes to PENDING.
		operationTaskStatusPendingTrigger(
			customNamespaceOperatingTaskStatusPending,
		),

		// Setup trigger for operation task status changes to PENDING or
		// ABORTING.
		operationTaskStatusPendingOrAbortingTrigger(
			customNamespaceOperatingTaskStatusPendingOrAborting,
		),

		// Setup trigger for relation unit changes by the relation
		// endpoint UUID.
		relationUnitByEndpointUUID(
			customNamespaceRelationUnitByEndpointUUID,
		),

		// Setup trigger for deleted secret revisions notifying
		// the secretURI/revision identifier.
		deletedSecretRevision(
			customNamespaceDeletedSecretRevisionID,
		),

		// Setup triggers for unit agent status changes.
		unitAgentStatusTriggers(
			customNamespaceUnitAgentStatus,
		),

		// Setup triggers for unit workload status changes.
		unitWorkloadStatusTriggers(
			customNamespaceUnitWorkloadStatus,
		),

		// Setup triggers for k8s pod status changes.
		k8sPodStatusTriggers(
			customNamespaceK8sPodStatus,
		),

		// Setup trigger for relation life or suspended changes.
		relationLifeSuspended(
			customNamespaceRelationLifeSuspended,
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

// operationTaskStatusPendingTrigger creates a trigger for operation task's
// status values inserted (in PENDING state) and changing to PENDING.
// NOTE: Our (current) implementation does not allow moving a task (back) to
// the PENDING state, so one could deduct that this trigger is equivalent to
// a trigger on the operation_task table for INSERTs. However, having this
// trigger is more explicit with respect to what the watcher is actually
// emitting.
func operationTaskStatusPendingTrigger(namespace int) func() schema.Patch {
	stmt := fmt.Sprintf(`
INSERT INTO change_log_namespace
VALUES (%[1]d,
        'custom_operation_task_status_pending',
        'Operation task status changes to PENDING');

CREATE TRIGGER trg_log_custom_operation_task_status_pending_insert
AFTER INSERT ON operation_task_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 1, %[1]d, ots.task_uuid, DATETIME('now')
    FROM operation_task_status AS ots
    JOIN operation_task_status_value AS otsv ON ots.status_id = otsv.id
    WHERE ots.task_uuid = NEW.task_uuid 
    AND otsv.status = 'pending';
END;
        
CREATE TRIGGER trg_log_custom_operation_task_status_pending_update
AFTER UPDATE ON operation_task_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 2, %[1]d, ots.task_uuid, DATETIME('now')
    FROM operation_task_status AS ots
    JOIN operation_task_status_value AS otsv ON ots.status_id = otsv.id
    WHERE ots.task_uuid = NEW.task_uuid 
    AND otsv.status = 'pending';
END;
`,
		namespace)
	return func() schema.Patch { return schema.MakePatch(stmt) }
}

// operationTaskStatusPendingOrAbortingTrigger creates a trigger for operation
// task's status values inserted (in PENDING state) and  changing to PENDING
// or ABORTING.
func operationTaskStatusPendingOrAbortingTrigger(namespace int) func() schema.Patch {
	stmt := fmt.Sprintf(`
INSERT INTO change_log_namespace
VALUES (%[1]d,
        'custom_operation_task_status_pending_or_aborting',
        'Operation task status changes to PENDING or ABORTING');
        
CREATE TRIGGER trg_log_custom_operation_task_status_pending_or_aborting_insert
AFTER INSERT ON operation_task_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 1, %[1]d, ots.task_uuid, DATETIME('now')
    FROM operation_task_status AS ots
    JOIN operation_task_status_value AS otsv ON ots.status_id = otsv.id
    WHERE ots.task_uuid = NEW.task_uuid 
    AND (
        otsv.status = 'aborting'
        OR otsv.status = 'pending');
END;

CREATE TRIGGER trg_log_custom_operation_task_status_pending_or_aborting_update
AFTER UPDATE ON operation_task_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 2, %[1]d, ots.task_uuid, DATETIME('now')
    FROM operation_task_status AS ots
    JOIN operation_task_status_value AS otsv ON ots.status_id = otsv.id
    WHERE ots.task_uuid = NEW.task_uuid 
    AND (
        otsv.status = 'aborting'
        OR otsv.status = 'pending');
END;
`,
		namespace)
	return func() schema.Patch { return schema.MakePatch(stmt) }
}

// relationUnitByEndpointUUID generates the triggers for the
// relation_unit table based on the relation endpoint UUID.
func relationUnitByEndpointUUID(namespaceID int) func() schema.Patch {
	return func() schema.Patch {
		return schema.MakePatch(fmt.Sprintf(`
-- insert namespace for RelationUnit
INSERT INTO change_log_namespace
VALUES (%[1]d,
        'custom_relation_unit_by_endpoint_uuid',
        'RelationUnit changes based on relation_endpoint_uuid');

-- insert trigger for RelationUnit
CREATE TRIGGER trg_log_custom_relation_unit_insert
AFTER INSERT ON relation_unit FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, %[1]d, NEW.relation_endpoint_uuid, DATETIME('now'));
END;

-- delete trigger for RelationUnit
CREATE TRIGGER trg_log_custom_relation_unit_delete
AFTER DELETE ON relation_unit FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, %[1]d, OLD.relation_endpoint_uuid, DATETIME('now'));
END;`, namespaceID))
	}
}

func deletedSecretRevision(namespaceID int) func() schema.Patch {
	return func() schema.Patch {
		return schema.MakePatch(fmt.Sprintf(`
-- insert namespace for record.
INSERT INTO change_log_namespace
VALUES (%[1]d,
        'custom_deleted_secret_revision_by_id',
        'Deleted secret revisions based on uri/revision_id');

-- delete trigger for secret revision.
CREATE TRIGGER trg_log_custom_secret_revision_delete
AFTER DELETE ON secret_revision FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, %[1]d, CONCAT(OLD.secret_id, '/', OLD.revision), DATETIME('now', 'utc'));
END;`, namespaceID))
	}
}

// unitAgentStatusTriggers generates the triggers for the unit_agent_status
// table. Whenever a unit_agent_status is updated, an application's status could
// change. So we want to emit an event with the application's uuid
func unitAgentStatusTriggers(namespaceID int) func() schema.Patch {
	return func() schema.Patch {
		return schema.MakePatch(fmt.Sprintf(`
-- insert namespace for unit_agent_status
INSERT INTO change_log_namespace
VALUES (%[1]d,
        'custom_unit_agent_status',
        'Unit agent status changes');

-- insert trigger for unit_agent_status
CREATE TRIGGER trg_log_custom_unit_agent_status_insert
AFTER INSERT ON unit_agent_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 1, %[1]d, u.application_uuid, DATETIME('now')
    FROM unit AS u
    WHERE u.uuid = NEW.unit_uuid;
END;

-- update trigger for unit_agent_status
CREATE TRIGGER trg_log_custom_unit_agent_status_update
AFTER UPDATE ON unit_agent_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 2, %[1]d, u.application_uuid, DATETIME('now')
    FROM unit AS u
    WHERE u.uuid = NEW.unit_uuid;
END;

-- delete trigger for unit_agent_status
CREATE TRIGGER trg_log_custom_unit_agent_status_delete
AFTER DELETE ON unit_agent_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 4, %[1]d, u.application_uuid, DATETIME('now')
    FROM unit AS u
    WHERE u.uuid = OLD.unit_uuid;
END;
`, namespaceID))
	}
}

// unitWorkloadStatusTriggers generates the triggers for the unit_workload_status
// table. Whenever a unit_workload_status is updated, an application's status
// could change. So we want to emit an event with the application's uuid
func unitWorkloadStatusTriggers(namespaceID int) func() schema.Patch {
	return func() schema.Patch {
		return schema.MakePatch(fmt.Sprintf(`
-- insert namespace for unit_workload_status
INSERT INTO change_log_namespace
VALUES (%[1]d,
        'custom_unit_workload_status',
        'Unit workload status changes');

-- insert trigger for unit_workload_status
CREATE TRIGGER trg_log_custom_unit_workload_status_insert
AFTER INSERT ON unit_workload_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 1, %[1]d, u.application_uuid, DATETIME('now')
    FROM unit AS u
    WHERE u.uuid = NEW.unit_uuid;
END;

-- update trigger for unit_workload_status
CREATE TRIGGER trg_log_custom_unit_workload_status_update
AFTER UPDATE ON unit_workload_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 2, %[1]d, u.application_uuid, DATETIME('now')
    FROM unit AS u
    WHERE u.uuid = NEW.unit_uuid;
END;

-- delete trigger for unit_workload_status
CREATE TRIGGER trg_log_custom_unit_workload_status_delete
AFTER DELETE ON unit_workload_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 4, %[1]d, u.application_uuid, DATETIME('now')
    FROM unit AS u
    WHERE u.uuid = OLD.unit_uuid;
END;
`, namespaceID))
	}
}

// k8sPodStatusTriggers generates the triggers for the k8s_pod_status table.
// Whenever a k8s_pod_status is updated, an application's status could change.
// So we want to emit an event with the application's uuid
func k8sPodStatusTriggers(namespaceID int) func() schema.Patch {
	return func() schema.Patch {
		return schema.MakePatch(fmt.Sprintf(`
-- insert namespace for k8s_pod_status
INSERT INTO change_log_namespace
VALUES (%[1]d,
        'custom_k8s_pod_status',
        'K8s pod status changes');

-- insert trigger for k8s_pod_status
CREATE TRIGGER trg_log_custom_k8s_pod_status_insert
AFTER INSERT ON k8s_pod_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 1, %[1]d, u.application_uuid, DATETIME('now')
    FROM unit AS u
    WHERE u.uuid = NEW.unit_uuid;
END;

-- update trigger for k8s_pod_status
CREATE TRIGGER trg_log_custom_k8s_pod_status_update
AFTER UPDATE ON k8s_pod_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 2, %[1]d, u.application_uuid, DATETIME('now')
    FROM unit AS u
    WHERE u.uuid = NEW.unit_uuid;
END;

-- delete trigger for k8s_pod_status
CREATE TRIGGER trg_log_custom_k8s_pod_status_delete
AFTER DELETE ON k8s_pod_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 4, %[1]d, u.application_uuid, DATETIME('now')
    FROM unit AS u
    WHERE u.uuid = OLD.unit_uuid;
END;
`, namespaceID))
	}
}

// relationLifeSuspended generates the triggers for the
// relation table based on the relation  UUID.
func relationLifeSuspended(namespaceID int) func() schema.Patch {
	return func() schema.Patch {
		return schema.MakePatch(fmt.Sprintf(`
-- insert namespace for Relation
INSERT INTO change_log_namespace
VALUES (%[1]d,
        'custom_relation_life_suspended',
        'Life or Suspended changes for a relation');

-- update trigger for Relation
CREATE TRIGGER trg_log_custom_relation_life_suspended_update
AFTER UPDATE ON relation FOR EACH ROW
WHEN
    NEW.life_id != OLD.life_id OR
    NEW.suspended != OLD.suspended
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, %[1]d, NEW.uuid, DATETIME('now'));
END;

-- delete trigger for Relation
CREATE TRIGGER trg_log_custom_relation_life_suspended_delete
AFTER DELETE ON relation FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, %[1]d, OLD.uuid, DATETIME('now'));
END;`, namespaceID))
	}
}
