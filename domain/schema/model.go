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

//go:embed model/sql/*.sql
var modelSchemaDir embed.FS

const (
	customNamespaceUnitInsertDelete tableNamespaceID = iota
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
		triggers.ChangeLogTriggersForApplicationExposedEndpointSpace("application_uuid", tableApplicationExposedEndpointSpace),
		triggers.ChangeLogTriggersForApplicationExposedEndpointCidr("application_uuid", tableApplicationExposedEndpointCIDR),
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
	)

	// Generic triggers.
	patches = append(patches,
		triggersForImmutableTable("model", "", "model table is immutable, only insertions are allowed"),
		// The "built-in" storage pools are immutable, it can only be inserted once.
		triggersForImmutableTable("storage_pool", "OLD.origin_id = 2", "built-in storage_pools are immutable, only insertions are allowed"),

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
	)

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

	// Add a custom namespace that only watches for insert and delete operations
	// for units. This is used, for example, by the deployer which manages running
	// agents, so only needs to know about new units, or removed unites.
	patches = append(patches, func() schema.Patch {
		return schema.MakePatch(fmt.Sprintf(`
INSERT INTO change_log_namespace VALUES (%[1]d, 'unit_insert_delete', 'Unit insert or delete changes only');

CREATE TRIGGER trg_log_unit_insert_delete_insert
AFTER INSERT ON unit FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, %[1]d, NEW.name, DATETIME('now'));
END;

CREATE TRIGGER trg_log_unit_insert_delete_delete
AFTER DELETE ON unit FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, %[1]d, OLD.name, DATETIME('now'));
END;
`, customNamespaceUnitInsertDelete))
	})

	modelSchema := schema.New()
	for _, fn := range patches {
		modelSchema.Add(fn())
	}
	return modelSchema
}
