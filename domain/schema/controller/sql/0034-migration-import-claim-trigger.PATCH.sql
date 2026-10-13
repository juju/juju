-- Patch 0034: changestream triggers for the target-side model migration
-- import claim.
--
-- The migration flag decides whether a model's workers may run. Its phase
-- read is direction-agnostic (modelmigration service MigrationPhase): a live
-- row in model_migration_import reports IMPORT, which is non-terminal, so
-- gated workers stay parked for the whole import. WatchMigrationActivity
-- watches this namespace alongside the export phase namespace, so the flag
-- flips when a claim is created and, critically, when it is deleted - the
-- moment an imported model becomes usable.
--
-- Namespace 10021 was reserved but left unused by patch 0031 (see the note in
-- 0033); this back-fills it so the import claim slots in with the other
-- migration namespaces 10019-10022.

-- The SQL linter doesn't support TRIGGER statements.
-- noqa: disable=all

-- insert namespace for ModelMigrationImport
INSERT INTO change_log_namespace VALUES (10021, 'model_migration_import', 'ModelMigrationImport changes based on model_uuid');

-- insert trigger for ModelMigrationImport
CREATE TRIGGER trg_log_model_migration_import_insert
AFTER INSERT ON model_migration_import FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10021, NEW.model_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for ModelMigrationImport
CREATE TRIGGER trg_log_model_migration_import_update
AFTER UPDATE ON model_migration_import FOR EACH ROW
WHEN
    NEW.uuid != OLD.uuid OR
    NEW.model_uuid != OLD.model_uuid OR
    NEW.source_migration_uuid != OLD.source_migration_uuid OR
    NEW.phase_type_id != OLD.phase_type_id OR
    NEW.updated_at != OLD.updated_at
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10021, OLD.model_uuid, DATETIME('now', 'utc'));
END;

-- delete trigger for ModelMigrationImport
CREATE TRIGGER trg_log_model_migration_import_delete
AFTER DELETE ON model_migration_import FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10021, OLD.model_uuid, DATETIME('now', 'utc'));
END;
-- noqa: enable=all
