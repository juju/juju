// Code generated by triggergen. DO NOT EDIT.

package triggers

import (
	"fmt"

	"github.com/juju/juju/core/database/schema"
)


// ChangeLogTriggersForModelAgent generates the triggers for the
// model_agent table.
func ChangeLogTriggersForModelAgent(columnName string, namespaceID int) func() schema.Patch {
	return func() schema.Patch {
		return schema.MakePatch(fmt.Sprintf(`
-- insert namespace for ModelAgent
INSERT INTO change_log_namespace VALUES (%[2]d, 'model_agent', 'ModelAgent changes based on %[1]s');

-- insert trigger for ModelAgent
CREATE TRIGGER trg_log_model_agent_insert
AFTER INSERT ON model_agent FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, %[2]d, NEW.%[1]s, DATETIME('now'));
END;

-- update trigger for ModelAgent
CREATE TRIGGER trg_log_model_agent_update
AFTER UPDATE ON model_agent FOR EACH ROW
WHEN 
	NEW.model_uuid != OLD.model_uuid OR
	NEW.previous_version != OLD.previous_version OR
	NEW.target_version != OLD.target_version 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, %[2]d, OLD.%[1]s, DATETIME('now'));
END;
-- delete trigger for ModelAgent
CREATE TRIGGER trg_log_model_agent_delete
AFTER DELETE ON model_agent FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, %[2]d, OLD.%[1]s, DATETIME('now'));
END;`, columnName, namespaceID))
	}
}
