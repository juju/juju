-- Patch 0033: staged deletions of model dqlite databases.
--
-- When a model is purged from the controller database while its dqlite
-- database must outlive the purge transaction (source-side model migration
-- REAP), a row is staged here inside that same transaction. The model DB
-- deleter worker on each controller node watches this table, deletes the
-- database, and removes the row on success, retrying on failure.
--
-- The table is deliberately standalone with no FK to model: the model row is
-- gone by the time a row exists here.
CREATE TABLE model_database_deletion (
    namespace TEXT NOT NULL PRIMARY KEY,
    created_at TIMESTAMP NOT NULL
);

-- The SQL linter doesn't support TRIGGER statements.
-- noqa: disable=all

-- insert namespace for ModelDatabaseDeletion
-- (10021 was left unused by patch 0031; it is not back-filled here.)
INSERT INTO change_log_namespace VALUES (10023, 'model_database_deletion', 'ModelDatabaseDeletion changes based on namespace');

-- insert trigger for ModelDatabaseDeletion
CREATE TRIGGER trg_log_model_database_deletion_insert
AFTER INSERT ON model_database_deletion FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10023, NEW.namespace, DATETIME('now', 'utc'));
END;

-- update trigger for ModelDatabaseDeletion
--
-- The purge transaction upserts a staged row (ON CONFLICT DO UPDATE), so a
-- re-staged deletion must still wake the worker.
CREATE TRIGGER trg_log_model_database_deletion_update
AFTER UPDATE ON model_database_deletion FOR EACH ROW
WHEN
    NEW.namespace != OLD.namespace OR
    NEW.created_at != OLD.created_at
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10023, OLD.namespace, DATETIME('now', 'utc'));
END;

-- delete trigger for ModelDatabaseDeletion
CREATE TRIGGER trg_log_model_database_deletion_delete
AFTER DELETE ON model_database_deletion FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10023, OLD.namespace, DATETIME('now', 'utc'));
END;
-- noqa: enable=all
