/**
  IMPORTANT FOR MERGE:

  This comment is a placeholder to remember to remove DEFAULT values from secret_metadata and secret_revision DATETIME
  fields. Those fields should be populated by the application and need to be removed from the schema.

  However, this is not trivial to delete as a PATCH a default value, because it can be done only by dropping and adding
  the column, which is not possible if the column is not nullable. Another way would be to drop the table and
  recreate it, but that would be a big pain.
 */

-- Secret revisions have update time too. A revision is updated when one of its fields is updated, even
-- if the revision content is not changed.
ALTER TABLE secret_revision ADD COLUMN update_time DATETIME; -- NOT NULL should be added on merge

-- TODO on merge: reintroduce the generated trigger on secret_revision (check what has been removed in the commit
--  which introduce comment)

DROP TRIGGER trg_log_secret_revision_update;

-- sqlfluff doesn't support TRIGGER statements
-- noqa: disable=all
CREATE TRIGGER trg_log_secret_revision_update
AFTER UPDATE ON secret_revision FOR EACH ROW
    WHEN
        NEW.uuid != OLD.uuid OR
        NEW.secret_id != OLD.secret_id OR
        NEW.revision != OLD.revision OR
        NEW.create_time != OLD.create_time OR
        (NEW.update_time != OLD.update_time OR
            (NEW.update_time IS NOT NULL AND OLD.update_time IS NULL) OR
            (NEW.update_time IS NULL AND OLD.update_time IS NOT NULL))
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10007, "uuid", DATETIME('now', 'utc'));
END;