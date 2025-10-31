// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"fmt"

	"github.com/juju/juju/core/database/schema"
)

const (
	// reservedCustomNamespaceIDOffset is the offset we use for standard
	// auto-generated namespaces to give ourselves space to add our own custom
	// namespaces. This is to prevent collisions with the trigger based
	// namespace IDs.
	// The namespace IDs indicate uniqueness not order, so we can safely have
	// gaps in the IDs.
	reservedCustomNamespaceIDOffset = 10000
)

type tableNamespaceID = int

// triggersForImmutableTable returns a function that creates triggers to prevent
// updates and deletes on the given table. The tableName is the name of the
// table to create the triggers for. The condition is an optional SQL condition
// that must be met for the trigger to be executed. The errMsg is the error
// message that will be returned if the trigger is fired.
func triggersForImmutableTable(tableName, condition, errMsg string) func() schema.Patch {
	if condition != "" {
		condition = fmt.Sprintf(`
    WHEN %s`[1:], condition)
	}
	return func() schema.Patch {
		stmt := fmt.Sprintf(`
CREATE TRIGGER trg_%[1]s_immutable_update
    BEFORE UPDATE ON %[1]s
    FOR EACH ROW
%[2]s
    BEGIN
        SELECT RAISE(FAIL, '%[3]s');
    END;

CREATE TRIGGER trg_%[1]s_immutable_delete
    BEFORE DELETE ON %[1]s
    FOR EACH ROW
%[2]s
    BEGIN
        SELECT RAISE(FAIL, '%[3]s');
    END;`[1:], tableName, condition, errMsg)
		return schema.MakePatch(stmt)
	}
}

// triggersForUnmodifiableTable returns a function that creates triggers to
// prevent updates on the given table. The tableName is the name of the table to
// create the triggers for. The errMsg is the error message that
// will be returned if the trigger is fired.
func triggersForUnmodifiableTable(tableName, errMsg string) func() schema.Patch {
	return func() schema.Patch {
		stmt := fmt.Sprintf(`
CREATE TRIGGER trg_%[1]s_immutable_update
    BEFORE UPDATE ON %[1]s
    FOR EACH ROW
    BEGIN
        SELECT RAISE(FAIL, '%[2]s');
    END;
`[1:], tableName, errMsg)
		return schema.MakePatch(stmt)
	}
}

// triggerGuardForTable returns a function that creates triggers to prevent
// updates on the given table when the specified condition (guard) is met. The
// tableName is the name of the table to create the triggers for. The condition
// is an optional SQL condition that must be met for the trigger to be executed.
// The errMsg is the error message that will be returned if the trigger is
// fired.
func triggerGuardForTable(tableName, condition, errMsg string) func() schema.Patch {
	return func() schema.Patch {
		stmt := fmt.Sprintf(`
CREATE TRIGGER trg_%[1]s_guard_update
    BEFORE UPDATE ON %[1]s
    FOR EACH ROW
        WHEN %[2]s
    BEGIN
        SELECT RAISE(FAIL, '%[3]s');
    END;`[1:], tableName, condition, errMsg)
		return schema.MakePatch(stmt)
	}
}

func triggerEntityLifecycleByNameForTable(tableName string, namespace int) func() schema.Patch {
	return triggerEntityLifecycleByFieldForTable(tableName, "name", namespace)
}

func triggerEntityLifecycleByFieldForTable(tableName, field string, namespace int) func() schema.Patch {
	return func() schema.Patch {
		stmt := fmt.Sprintf(`
INSERT INTO change_log_namespace VALUES (%[1]d, 'custom_%[2]s_%[3]s_lifecycle', 'Changes to the lifecycle of %[2]s (%[3]s) entities');

CREATE TRIGGER trg_log_custom_%[2]s_%[3]s_lifecycle_insert
AFTER INSERT ON %[2]s FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, %[1]d, NEW.%[3]s, DATETIME('now', 'utc'));
END;

CREATE TRIGGER trg_log_custom_%[2]s_%[3]s_lifecycle_update
AFTER UPDATE ON %[2]s FOR EACH ROW
WHEN
	NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, %[1]d, OLD.%[3]s, DATETIME('now', 'utc'));
END;

CREATE TRIGGER trg_log_custom_%[2]s_%[3]s_lifecycle_delete
AFTER DELETE ON %[2]s FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, %[1]d, OLD.%[3]s, DATETIME('now', 'utc'));
END;`[1:], namespace, tableName, field)
		return schema.MakePatch(stmt)
	}
}

func triggerMachineLifecycleWithDependants(namespace int) func() schema.Patch {
	return func() schema.Patch {
		stmt := fmt.Sprintf(`
INSERT INTO change_log_namespace VALUES (%[1]d, 'custom_machine_uuid_lifecycle_with_dependants', 'Changes to the lifecycle of machines, machine units and storage entities for the machine');

-- machine life triggers
CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_machine_insert
AFTER INSERT ON machine FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES(1, %[1]d, NEW.uuid, DATETIME('now'));
END;

CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_machine_update
AFTER UPDATE ON machine FOR EACH ROW
WHEN
    NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES(2, %[1]d, OLD.uuid, DATETIME('now'));
END;

CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_machine_delete
AFTER DELETE ON machine FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES(4, %[1]d, OLD.uuid, DATETIME('now'));
END;

-- machine parent (child) life triggers
CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_machine_parent_insert
AFTER INSERT ON machine_parent FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES(1, %[1]d, NEW.parent_uuid, DATETIME('now'));
END;

CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_machine_parent_update
AFTER UPDATE ON machine FOR EACH ROW
WHEN
    NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 2, %[1]d, mp.parent_uuid, DATETIME('now')
    FROM machine_parent AS mp
    WHERE mp.machine_uuid = OLD.uuid;
END;

CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_machine_parent_delete
AFTER DELETE ON machine FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 4, %[1]d, mp.parent_uuid, DATETIME('now')
    FROM machine_parent AS mp
    WHERE mp.machine_uuid = OLD.uuid;
END;

-- unit on machine life triggers
CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_unit_insert
AFTER INSERT ON unit FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 1, %[1]d, m.uuid, DATETIME('now', 'utc')
    FROM machine AS m
    WHERE m.net_node_uuid = NEW.net_node_uuid;
END;

CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_unit_update
AFTER UPDATE ON unit FOR EACH ROW
WHEN
    NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 2, %[1]d, m.uuid, DATETIME('now', 'utc')
    FROM machine AS m
    WHERE m.net_node_uuid = OLD.net_node_uuid;
END;

CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_unit_delete
AFTER DELETE ON unit FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 4, %[1]d, m.uuid, DATETIME('now', 'utc')
    FROM machine m
    WHERE m.net_node_uuid = OLD.net_node_uuid;
END;

-- machine_filesystem delete trigger
CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_machine_filesystem_delete
AFTER DELETE ON machine_filesystem FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES(4, %[1]d, OLD.machine_uuid, DATETIME('now'));
END;

-- machine_volume delete trigger
CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_machine_volume_delete
AFTER DELETE ON machine_volume FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES(4, %[1]d, OLD.machine_uuid, DATETIME('now'));
END;

-- storage_filesystem_attachment on machine net node delete trigger
CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_storage_filesystem_attachment_delete
AFTER DELETE ON storage_filesystem_attachment FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 4, %[1]d, m.uuid, DATETIME('now')
    FROM machine AS m
    WHERE m.net_node_uuid = OLD.net_node_uuid;
END;

-- storage_volume_attachment on machine net node delete trigger
CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_storage_volume_attachment_delete
AFTER DELETE ON storage_volume_attachment FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 4, %[1]d, m.uuid, DATETIME('now')
    FROM machine AS m
    WHERE m.net_node_uuid = OLD.net_node_uuid;
END;`[1:], namespace)
		return schema.MakePatch(stmt)
	}
}

func triggerGuardForLife(tableName string) func() schema.Patch {
	return func() schema.Patch {
		stmt := fmt.Sprintf(`
CREATE TRIGGER trg_%[1]s_guard_life
    BEFORE UPDATE ON %[1]s
    FOR EACH ROW
    WHEN NEW.life_id < OLD.life_id
    BEGIN
        SELECT RAISE(FAIL, 'Cannot transition life for %[1]s backwards');
    END;`[1:], tableName)
		return schema.MakePatch(stmt)
	}
}
