// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package triggers

import (
	"fmt"

	"github.com/juju/juju/core/database/schema"
)

// The application_scale changestream triggers are hand-maintained, unlike
// the rest of the triggers in this package. The table only exists in the
// base schema: patch 0065 replaces it with application_provisioning_state
// before the schema reaches its final state, and triggergen introspects
// the final schema, so it cannot generate triggers for the table.
//
// The triggers are still required for fresh databases. They run in the
// base schema phase, where the table exists, and they register the
// application_scale change log namespace that patch 0065 reuses for the
// replacement table's triggers.

// ChangeLogTriggersForApplicationScale generates the triggers for the
// application_scale table.
func ChangeLogTriggersForApplicationScale(columnName string, namespaceID int) func() schema.Patch {
	return func() schema.Patch {
		return schema.MakePatch(fmt.Sprintf(`
-- insert namespace for ApplicationScale
INSERT INTO change_log_namespace VALUES (%[2]d, 'application_scale', 'ApplicationScale changes based on %[1]s');

-- insert trigger for ApplicationScale
CREATE TRIGGER trg_log_application_scale_insert
AFTER INSERT ON application_scale FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, %[2]d, NEW.%[1]s, DATETIME('now', 'utc'));
END;

-- update trigger for ApplicationScale
CREATE TRIGGER trg_log_application_scale_update
AFTER UPDATE ON application_scale FOR EACH ROW
WHEN 
	NEW.application_uuid != OLD.application_uuid OR
	(NEW.scale != OLD.scale OR (NEW.scale IS NOT NULL AND OLD.scale IS NULL) OR (NEW.scale IS NULL AND OLD.scale IS NOT NULL)) OR
	(NEW.scale_target != OLD.scale_target OR (NEW.scale_target IS NOT NULL AND OLD.scale_target IS NULL) OR (NEW.scale_target IS NULL AND OLD.scale_target IS NOT NULL)) OR
	(NEW.scaling != OLD.scaling OR (NEW.scaling IS NOT NULL AND OLD.scaling IS NULL) OR (NEW.scaling IS NULL AND OLD.scaling IS NOT NULL)) 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, %[2]d, OLD.%[1]s, DATETIME('now', 'utc'));
END;
-- delete trigger for ApplicationScale
CREATE TRIGGER trg_log_application_scale_delete
AFTER DELETE ON application_scale FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, %[2]d, OLD.%[1]s, DATETIME('now', 'utc'));
END;`, columnName, namespaceID))
	}
}
