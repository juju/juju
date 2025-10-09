// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"embed"
	"fmt"
	"sort"

	"github.com/juju/juju/core/database/schema"
	"github.com/juju/juju/domain/schema/model/triggers"
)

//go:generate go run ./../../generate/triggergen -db=model -destination=./model/triggers/blockdevice-triggers.gen.go -package=triggers -tables=block_device
//go:generate go run ./../../generate/triggergen -db=model -destination=./model/triggers/model-triggers.gen.go -package=triggers -tables=model_config
//go:generate go run ./../../generate/triggergen -db=model -destination=./model/triggers/objectstore-triggers.gen.go -package=triggers -tables=object_store_metadata_path
//go:generate go run ./../../generate/triggergen -db=model -destination=./model/triggers/secret-triggers.gen.go -package=triggers -tables=secret_metadata,secret_rotation,secret_revision,secret_revision_expire,secret_revision_obsolete,secret_revision,secret_reference,secret_deleted_value_ref
//go:generate go run ./../../generate/triggergen -db=model -destination=./model/triggers/network-triggers.gen.go -package=triggers -tables=subnet,ip_address
//go:generate go run ./../../generate/triggergen -db=model -destination=./model/triggers/machine-triggers.gen.go -package=triggers -tables=machine,machine_lxd_profile
//go:generate go run ./../../generate/triggergen -db=model -destination=./model/triggers/machine-cloud-instance-triggers.gen.go -package=triggers -tables=machine_cloud_instance
//go:generate go run ./../../generate/triggergen -db=model -destination=./model/triggers/machine-requires-reboot-triggers.gen.go -package=triggers -tables=machine_requires_reboot
//go:generate go run ./../../generate/triggergen -db=model -destination=./model/triggers/application-triggers.gen.go -package=triggers -tables=application,application_config_hash,application_setting,charm,application_scale,port_range,application_exposed_endpoint_space,application_exposed_endpoint_cidr
//go:generate go run ./../../generate/triggergen -db=model -destination=./model/triggers/unit-triggers.gen.go -package triggers -tables=unit,unit_principal,unit_resolved
//go:generate go run ./../../generate/triggergen -db=model -destination=./model/triggers/relation-triggers.gen.go -package=triggers -tables=relation_application_settings_hash,relation_unit_settings_hash,relation_unit,relation,relation_status,application_endpoint
//go:generate go run ./../../generate/triggergen -db=model -destination=./model/triggers/cleanup-triggers.gen.go -package=triggers -tables=removal
//go:generate go run ./../../generate/triggergen -db=model -destination=./model/triggers/operation-triggers.gen.go -package=triggers -tables=operation_task_log
//go:generate go run ./../../generate/triggergen -db=model -destination=./model/triggers/crossmodelrelation-triggers.gen.go -package=triggers -tables=application_remote_offerer

//go:generate go run ./../../generate/fktriggergen -db=model -destination=./model/triggers/fk-triggers.gen.go -package=triggers

//go:embed model/sql/*.sql
var modelSchemaDir embed.FS

const (
	customNamespaceUnitLifecycle tableNamespaceID = iota
	customNamespaceMachineLifecycle
	customNamespaceMachineLifeAndStartTime
	customNamespaceMachineUnitLifecycle
	customNamespaceStorageFilesystemLifeMachineProvisioning
	customNamespaceStorageFilesystemLifeModelProvisioning
	customNamespaceStorageFilesystemAttachmentLifeMachineProvisioning
	customNamespaceStorageFilesystemAttachmentLifeModelProvisioning
	customNamespaceStorageVolumeLifeMachineProvisioning
	customNamespaceStorageVolumeLifeModelProvisioning
	customNamespaceStorageVolumeAttachmentLifeMachineProvisioning
	customNamespaceStorageVolumeAttachmentLifeModelProvisioning
	customNamespaceStorageVolumeAttachmentPlanLifeMachineProvisioning
	customNamespaceUnitRemovalLifecycle
	customNamespaceMachineRemovalLifecycle
	customNamespaceApplicationRemovalLifecycle
	customNamespaceRelationRemovalLifecycle
	customNamespaceModelLifeRemovalLifecycle
	customNamespaceStorageAttachmentRelatedEntities
	customNamespaceStorageAttachmentLifecycle
	customNamespaceOperatingTaskStatusPending
	customNamespaceOperatingTaskStatusPendingOrAborting
)

const (
	tableModelConfig tableNamespaceID = iota + reservedCustomNamespaceIDOffset
	tableModelObjectStoreMetadata
	tableBlockDeviceMachine
	tableSecretMetadataAutoPrune
	tableSecretRotation
	tableSecretRevisionObsolete
	tableSecretRevisionExpire
	tableSecretRevision
	tableSecretReference
	tableSubnet
	tableMachine
	tableMachineLxdProfile
	tableMachineCloudInstance
	tableMachineRequireReboot
	tableCharm
	tableUnit
	tableUnitPrincipal
	tableUnitResolved
	tableApplicationScale
	tablePortRange
	tableApplicationExposedEndpointSpace
	tableApplicationExposedEndpointCIDR
	tableSecretDeletedValueRef
	tableApplication
	tableRemoval
	tableApplicationConfigHash
	tableApplicationSetting
	tableAgentVersion
	tableRelationApplicationSettingsHash
	tableRelationUnitSettingsHash
	tableRelation
	tableRelationStatus
	tableRelationUnit
	tableIpAddress
	tableApplicationEndpoint
	tableOperationTaskLog
	tableCrossModelRelationApplicationRemoteOfferers
)

// ModelDDL is used to create model databases.
func ModelDDL() *schema.Schema {
	entries, err := modelSchemaDir.ReadDir("model/sql")
	if err != nil {
		panic(err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}

	sort.Slice(names, func(i, j int) bool {
		return names[i] < names[j]
	})

	patches := make([]func() schema.Patch, len(names))
	for i, name := range names {
		data, err := modelSchemaDir.ReadFile(fmt.Sprintf("model/sql/%s", name))
		if err != nil {
			panic(err)
		}

		patches[i] = func() schema.Patch {
			return schema.MakePatch(string(data))
		}
	}

	// Changestream triggers.
	if EnableGenerated {
		patches = append(patches,
			triggers.ChangeLogTriggersForBlockDevice("machine_uuid", tableBlockDeviceMachine),
			triggers.ChangeLogTriggersForModelConfig("key", tableModelConfig),
			triggers.ChangeLogTriggersForObjectStoreMetadataPath("path", tableModelObjectStoreMetadata),
			triggers.ChangeLogTriggersForSecretMetadata("secret_id", tableSecretMetadataAutoPrune),
			triggers.ChangeLogTriggersForSecretRotation("secret_id", tableSecretRotation),
			triggers.ChangeLogTriggersForSecretRevisionObsolete("revision_uuid", tableSecretRevisionObsolete),
			triggers.ChangeLogTriggersForSecretRevisionExpire("revision_uuid", tableSecretRevisionExpire),
			triggers.ChangeLogTriggersForSecretRevision("uuid", tableSecretRevision),
			triggers.ChangeLogTriggersForSecretReference("secret_id", tableSecretReference),
			triggers.ChangeLogTriggersForSubnet("uuid", tableSubnet),
			triggers.ChangeLogTriggersForMachine("uuid", tableMachine),
			triggers.ChangeLogTriggersForMachineLxdProfile("machine_uuid", tableMachineLxdProfile),
			triggers.ChangeLogTriggersForMachineCloudInstance("machine_uuid", tableMachineCloudInstance),
			triggers.ChangeLogTriggersForMachineRequiresReboot("machine_uuid", tableMachineRequireReboot),
			triggers.ChangeLogTriggersForCharm("uuid", tableCharm),
			triggers.ChangeLogTriggersForUnit("uuid", tableUnit),
			// NOTE: we emit the uuid of the principal unit, not the subordinate, when
			// there is a change on the unit_principal table.
			triggers.ChangeLogTriggersForUnitPrincipal("principal_uuid", tableUnitPrincipal),
			triggers.ChangeLogTriggersForUnitResolved("unit_uuid", tableUnitResolved),
			triggers.ChangeLogTriggersForApplicationScale("application_uuid", tableApplicationScale),
			triggers.ChangeLogTriggersForPortRange("unit_uuid", tablePortRange),
			triggers.ChangeLogTriggersForApplicationExposedEndpointSpace("application_uuid",
				tableApplicationExposedEndpointSpace),
			triggers.ChangeLogTriggersForApplicationExposedEndpointCidr("application_uuid",
				tableApplicationExposedEndpointCIDR),
			triggers.ChangeLogTriggersForSecretDeletedValueRef("revision_uuid", tableSecretDeletedValueRef),
			triggers.ChangeLogTriggersForApplication("uuid", tableApplication),
			triggers.ChangeLogTriggersForRemoval("uuid", tableRemoval),
			triggers.ChangeLogTriggersForApplicationConfigHash("application_uuid", tableApplicationConfigHash),
			triggers.ChangeLogTriggersForApplicationSetting("application_uuid", tableApplicationSetting),
			triggers.ChangeLogTriggersForRelationApplicationSettingsHash("relation_endpoint_uuid",
				tableRelationApplicationSettingsHash),
			triggers.ChangeLogTriggersForRelationUnitSettingsHash("relation_unit_uuid",
				tableRelationUnitSettingsHash),
			triggers.ChangeLogTriggersForRelation("uuid",
				tableRelation),
			triggers.ChangeLogTriggersForRelationStatus("relation_uuid",
				tableRelationStatus),
			triggers.ChangeLogTriggersForRelationUnit("unit_uuid", tableRelationUnit),
			triggers.ChangeLogTriggersForIpAddress("net_node_uuid", tableIpAddress),
			triggers.ChangeLogTriggersForApplicationEndpoint("application_uuid", tableApplicationEndpoint),
			triggers.ChangeLogTriggersForOperationTaskLog("task_uuid", tableOperationTaskLog),
			triggers.ChangeLogTriggersForApplicationRemoteOfferer("uuid",
				tableCrossModelRelationApplicationRemoteOfferers),
		)
	}

	// Generic triggers.
	patches = append(patches,
		triggersForImmutableTable("model", "", "model table is immutable, only insertions are allowed"),

		// The charm is unmodifiable.
		// There is a lot of assumptions in the code that the charm is immutable
		// from modification. This is a safety net to ensure that the charm is
		// not modified.
		triggersForUnmodifiableTable("charm_action", "charm_action table is unmodifiable, only insertions and deletions are allowed"),
		triggersForUnmodifiableTable("charm_config", "charm_config table is unmodifiable, only insertions and deletions are allowed"),
		triggersForUnmodifiableTable("charm_container_mount", "charm_container_mount table is unmodifiable, only insertions and deletions are allowed"),
		triggersForUnmodifiableTable("charm_container", "charm_container table is unmodifiable, only insertions and deletions are allowed"),
		triggersForUnmodifiableTable("charm_device", "charm_device table is unmodifiable, only insertions and deletions are allowed"),
		triggersForUnmodifiableTable("charm_extra_binding", "charm_extra_binding table is unmodifiable, only insertions and deletions are allowed"),
		triggersForUnmodifiableTable("charm_hash", "charm_hash table is unmodifiable, only insertions and deletions are allowed"),
		triggersForUnmodifiableTable("charm_manifest_base", "charm_manifest base table is unmodifiable, only insertions and deletions are allowed"),
		triggersForUnmodifiableTable("charm_metadata", "charm_metadata table is unmodifiable, only insertions and deletions are allowed"),
		triggersForUnmodifiableTable("charm_relation", "charm_relation table is unmodifiable, only insertions and deletions are allowed"),
		triggersForUnmodifiableTable("charm_resource", "charm_resource table is unmodifiable, only insertions and deletions are allowed"),
		triggersForUnmodifiableTable("charm_storage", "charm_storage table is unmodifiable, only insertions and deletions are allowed"),
		triggersForUnmodifiableTable("charm_term", "charm_term table is unmodifiable, only insertions and deletions are allowed"),

		// Machine controller is unmodifiable.
		triggersForUnmodifiableTable("application_controller", "application_controller table is unmodifiable, only insertions and deletions are allowed"),

		// Offer endpoints are unmodifiable.
		triggersForUnmodifiableTable("offer_endpoint", "offer_endpoint table is unmodifiable, only insertions and deletions are allowed"),

		// Secret permissions do not allow subject or scope to be updated.
		triggerGuardForTable("secret_permission",
			"OLD.subject_type_id <> NEW.subject_type_id OR OLD.scope_uuid <> NEW.scope_uuid OR OLD.scope_type_id <> NEW.scope_type_id",
			"secret permission subjects and scopes must be identical",
		),

		triggerGuardForTable("sequence",
			"OLD.namespace = NEW.namespace AND NEW.value <= OLD.value",
			"sequence number must monotonically increase",
		),

		// Storage pool origin cannot be changed.
		triggerGuardForTable(
			"storage_pool",
			"OLD.origin_id <> NEW.origin_id",
			"storage pool origin cannot be changed",
		),

		// Ensure that entities with life values can not transition backwards.
		triggerGuardForLife("application"),
		triggerGuardForLife("unit"),
		triggerGuardForLife("machine"),
		triggerGuardForLife("machine_cloud_instance"),
		triggerGuardForLife("storage_instance"),
		triggerGuardForLife("storage_attachment"),
		triggerGuardForLife("storage_volume"),
		triggerGuardForLife("storage_volume_attachment"),
		triggerGuardForLife("storage_filesystem"),
		triggerGuardForLife("storage_filesystem_attachment"),
		triggerGuardForLife("storage_volume_attachment_plan"),

		// Add a custom namespace that only watches for insert and delete
		// operations for entities.
		triggerEntityLifecycleByNameForTable("unit", customNamespaceUnitLifecycle),
		triggerEntityLifecycleByNameForTable("machine", customNamespaceMachineLifecycle),
		triggerMachineUnitLifecycle(customNamespaceMachineUnitLifecycle),

		triggerEntityLifecycleByFieldForTable("application", "uuid", customNamespaceApplicationRemovalLifecycle),
		triggerEntityLifecycleByFieldForTable("machine", "uuid", customNamespaceMachineRemovalLifecycle),
		triggerEntityLifecycleByFieldForTable("unit", "uuid", customNamespaceUnitRemovalLifecycle),
		triggerEntityLifecycleByFieldForTable("relation", "uuid", customNamespaceRelationRemovalLifecycle),
		triggerEntityLifecycleByFieldForTable("model_life", "model_uuid", customNamespaceModelLifeRemovalLifecycle),
		triggerEntityLifecycleByFieldForTable("storage_attachment", "unit_uuid", customNamespaceStorageAttachmentLifecycle),

		// Ensure that 3 tables related to operations are immutable.
		triggersForUnmodifiableTable("operation_parameter", "operation_parameter table is unmodifiable, only insertions and deletions are allowed"),
		triggersForUnmodifiableTable("operation_machine_task", "operation_machine_task table is unmodifiable, only insertions and deletions are allowed"),
		triggersForUnmodifiableTable("operation_unit_task", "operation_unit_task table is unmodifiable, only insertions and deletions are allowed"),
	)

	patches = append(patches, func() schema.Patch {
		return schema.MakePatch(fmt.Sprintf(`
INSERT INTO change_log_namespace VALUES (%[1]d, 'custom_machine_lifecycle_start_time', 'Machine life or agent start time changes');

CREATE TRIGGER trg_log_machine_insert_life_start_time
AFTER INSERT ON machine FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, %[1]d, NEW.name, DATETIME('now'));
END;

CREATE TRIGGER trg_log_machine_update_life_start_time
AFTER UPDATE ON machine FOR EACH ROW
WHEN 
	NEW.life_id != OLD.life_id OR
	(NEW.agent_started_at != OLD.agent_started_at OR (NEW.agent_started_at IS NOT NULL AND OLD.agent_started_at IS NULL) OR (NEW.agent_started_at IS NULL AND OLD.agent_started_at IS NOT NULL))
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, %[1]d, OLD.name, DATETIME('now'));
END;

CREATE TRIGGER trg_log_machine_delete_life_start_time
AFTER DELETE ON machine FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, %[1]d, OLD.name, DATETIME('now'));
END;
`, customNamespaceMachineLifeAndStartTime))
	})

	// For agent_version we only care if the single row is updated.
	// We emit the new target agent version.
	patches = append(patches, func() schema.Patch {
		return schema.MakePatch(fmt.Sprintf(`
INSERT INTO change_log_namespace VALUES (%[1]d, 'agent_version', 'Agent version changes based on target version');

CREATE TRIGGER trg_log_agent_version_update
AFTER UPDATE ON agent_version FOR EACH ROW
WHEN
	NEW.target_version != OLD.target_version
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, %[1]d, NEW.target_version, DATETIME('now'));
END;
`,
			tableAgentVersion))
	})

	// Ensures that for an offer uuid, the application_endpoint included
	// in the offer_endpoint table are for the same application. There is
	// a corresponding trigger to make the offer_endpoint table
	// immutable.
	patches = append(patches, func() schema.Patch {
		return schema.MakePatch(`
CREATE TRIGGER trg_ensure_single_app_per_offer
BEFORE INSERT ON offer_endpoint
FOR EACH ROW
BEGIN
    -- Check if the new endpoint_uuid has a different application_uuid than
    -- existing ones for the same offer_uuid
    SELECT RAISE(ABORT, 'All endpoints for an offer must belong to the same application')
    WHERE EXISTS (
        SELECT 1
        FROM  offer_endpoint oe
        JOIN  application_endpoint ae_new ON ae_new.uuid = NEW.endpoint_uuid
        JOIN  application_endpoint ae_existing ON ae_existing.uuid = oe.endpoint_uuid
        WHERE oe.offer_uuid = NEW.offer_uuid
        AND   ae_new.application_uuid <> ae_existing.application_uuid
    );
END;	`)
	})

	// Ensure that an operation task can only be linked to a machine OR a unit.
	patches = append(patches, func() schema.Patch {
		return schema.MakePatch(`
CREATE TRIGGER trg_insert_machine_task_if_not_unit_task
BEFORE INSERT ON operation_machine_task
WHEN EXISTS (
    SELECT 1 FROM operation_unit_task WHERE task_uuid = NEW.task_uuid
)
BEGIN
    SELECT RAISE(ABORT, 'Task is already linked to a unit, cannot be added for a machine');
END;

CREATE TRIGGER trg_insert_unit_task_if_not_machine_task
BEFORE INSERT ON operation_unit_task
WHEN EXISTS (
    SELECT 1 FROM operation_machine_task WHERE task_uuid = NEW.task_uuid
)
BEGIN
    SELECT RAISE(ABORT, 'Task is already linked to a machine, cannot be added for a unit');
END;
	`)
	})

	// Ensure that a unit cannot be linked to a CMR application.
	patches = append(patches, func() schema.Patch {
		return schema.MakePatch(`
CREATE TRIGGER trg_insert_unit_for_cmr_app
AFTER INSERT ON unit
WHEN EXISTS (
    SELECT 1
    FROM unit AS u
    JOIN charm AS c ON u.charm_uuid = c.uuid
    WHERE u.uuid = NEW.uuid AND c.source_id == 2
)
BEGIN
	SELECT RAISE(ABORT, 'Adding a unit to a CMR application is not allowed');
END;
	`)
	})

	patches = append(patches, customModelTriggers()...)

	// Debug triggers.
	if EnableGenerated {
		if EnableDebug {
			patches = append(patches,
				triggers.FKDebugTriggers(),
			)
		}
	}

	modelSchema := schema.New()
	for _, fn := range patches {
		modelSchema.Add(fn())
	}
	return modelSchema
}
