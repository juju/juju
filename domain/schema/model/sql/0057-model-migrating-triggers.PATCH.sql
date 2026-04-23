-- This PATCH adds change_log triggers for model_migrating.
-- The table itself was created in 0037-model-migrating.PATCH.sql;
-- adding triggers separately keeps the namespace registration and
-- trigger DDL in the same SQL-level mechanism as the rest of the
-- post-patch files.

-- insert namespace for ModelMigrating
INSERT INTO change_log_namespace VALUES (10041, 'model_migrating', 'ModelMigrating changes based on model_uuid');

-- insert trigger for ModelMigrating
CREATE TRIGGER trg_log_model_migrating_insert
AFTER INSERT ON model_migrating FOR EACH ROW
BEGIN
INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
VALUES (1, 10041, new.model_uuid, DATETIME('now', 'utc'));
END;

-- delete trigger for ModelMigrating
CREATE TRIGGER trg_log_model_migrating_delete
AFTER DELETE ON model_migrating FOR EACH ROW
BEGIN
INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
VALUES (4, 10041, old.model_uuid, DATETIME('now', 'utc'));
END;
