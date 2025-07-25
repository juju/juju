// Code generated by triggergen. DO NOT EDIT.

package triggers

import (
	"fmt"

	"github.com/juju/juju/core/database/schema"
)


// ChangeLogTriggersForControllerApiAddress generates the triggers for the
// controller_api_address table.
func ChangeLogTriggersForControllerApiAddress(columnName string, namespaceID int) func() schema.Patch {
	return func() schema.Patch {
		return schema.MakePatch(fmt.Sprintf(`
-- insert namespace for ControllerApiAddress
INSERT INTO change_log_namespace VALUES (%[2]d, 'controller_api_address', 'ControllerApiAddress changes based on %[1]s');

-- insert trigger for ControllerApiAddress
CREATE TRIGGER trg_log_controller_api_address_insert
AFTER INSERT ON controller_api_address FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, %[2]d, NEW.%[1]s, DATETIME('now'));
END;

-- update trigger for ControllerApiAddress
CREATE TRIGGER trg_log_controller_api_address_update
AFTER UPDATE ON controller_api_address FOR EACH ROW
WHEN 
	NEW.controller_id != OLD.controller_id OR
	NEW.address != OLD.address OR
	(NEW.is_agent != OLD.is_agent OR (NEW.is_agent IS NOT NULL AND OLD.is_agent IS NULL) OR (NEW.is_agent IS NULL AND OLD.is_agent IS NOT NULL)) OR
	NEW.scope != OLD.scope 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, %[2]d, OLD.%[1]s, DATETIME('now'));
END;
-- delete trigger for ControllerApiAddress
CREATE TRIGGER trg_log_controller_api_address_delete
AFTER DELETE ON controller_api_address FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, %[2]d, OLD.%[1]s, DATETIME('now'));
END;`, columnName, namespaceID))
	}
}

// ChangeLogTriggersForControllerConfig generates the triggers for the
// controller_config table.
func ChangeLogTriggersForControllerConfig(columnName string, namespaceID int) func() schema.Patch {
	return func() schema.Patch {
		return schema.MakePatch(fmt.Sprintf(`
-- insert namespace for ControllerConfig
INSERT INTO change_log_namespace VALUES (%[2]d, 'controller_config', 'ControllerConfig changes based on %[1]s');

-- insert trigger for ControllerConfig
CREATE TRIGGER trg_log_controller_config_insert
AFTER INSERT ON controller_config FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, %[2]d, NEW.%[1]s, DATETIME('now'));
END;

-- update trigger for ControllerConfig
CREATE TRIGGER trg_log_controller_config_update
AFTER UPDATE ON controller_config FOR EACH ROW
WHEN 
	NEW.key != OLD.key OR
	(NEW.value != OLD.value OR (NEW.value IS NOT NULL AND OLD.value IS NULL) OR (NEW.value IS NULL AND OLD.value IS NOT NULL)) 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, %[2]d, OLD.%[1]s, DATETIME('now'));
END;
-- delete trigger for ControllerConfig
CREATE TRIGGER trg_log_controller_config_delete
AFTER DELETE ON controller_config FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, %[2]d, OLD.%[1]s, DATETIME('now'));
END;`, columnName, namespaceID))
	}
}

// ChangeLogTriggersForControllerNode generates the triggers for the
// controller_node table.
func ChangeLogTriggersForControllerNode(columnName string, namespaceID int) func() schema.Patch {
	return func() schema.Patch {
		return schema.MakePatch(fmt.Sprintf(`
-- insert namespace for ControllerNode
INSERT INTO change_log_namespace VALUES (%[2]d, 'controller_node', 'ControllerNode changes based on %[1]s');

-- insert trigger for ControllerNode
CREATE TRIGGER trg_log_controller_node_insert
AFTER INSERT ON controller_node FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, %[2]d, NEW.%[1]s, DATETIME('now'));
END;

-- update trigger for ControllerNode
CREATE TRIGGER trg_log_controller_node_update
AFTER UPDATE ON controller_node FOR EACH ROW
WHEN 
	NEW.controller_id != OLD.controller_id OR
	(NEW.dqlite_node_id != OLD.dqlite_node_id OR (NEW.dqlite_node_id IS NOT NULL AND OLD.dqlite_node_id IS NULL) OR (NEW.dqlite_node_id IS NULL AND OLD.dqlite_node_id IS NOT NULL)) OR
	(NEW.dqlite_bind_address != OLD.dqlite_bind_address OR (NEW.dqlite_bind_address IS NOT NULL AND OLD.dqlite_bind_address IS NULL) OR (NEW.dqlite_bind_address IS NULL AND OLD.dqlite_bind_address IS NOT NULL)) 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, %[2]d, OLD.%[1]s, DATETIME('now'));
END;
-- delete trigger for ControllerNode
CREATE TRIGGER trg_log_controller_node_delete
AFTER DELETE ON controller_node FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, %[2]d, OLD.%[1]s, DATETIME('now'));
END;`, columnName, namespaceID))
	}
}

// ChangeLogTriggersForExternalController generates the triggers for the
// external_controller table.
func ChangeLogTriggersForExternalController(columnName string, namespaceID int) func() schema.Patch {
	return func() schema.Patch {
		return schema.MakePatch(fmt.Sprintf(`
-- insert namespace for ExternalController
INSERT INTO change_log_namespace VALUES (%[2]d, 'external_controller', 'ExternalController changes based on %[1]s');

-- insert trigger for ExternalController
CREATE TRIGGER trg_log_external_controller_insert
AFTER INSERT ON external_controller FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, %[2]d, NEW.%[1]s, DATETIME('now'));
END;

-- update trigger for ExternalController
CREATE TRIGGER trg_log_external_controller_update
AFTER UPDATE ON external_controller FOR EACH ROW
WHEN 
	NEW.uuid != OLD.uuid OR
	(NEW.alias != OLD.alias OR (NEW.alias IS NOT NULL AND OLD.alias IS NULL) OR (NEW.alias IS NULL AND OLD.alias IS NOT NULL)) OR
	NEW.ca_cert != OLD.ca_cert 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, %[2]d, OLD.%[1]s, DATETIME('now'));
END;
-- delete trigger for ExternalController
CREATE TRIGGER trg_log_external_controller_delete
AFTER DELETE ON external_controller FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, %[2]d, OLD.%[1]s, DATETIME('now'));
END;`, columnName, namespaceID))
	}
}

