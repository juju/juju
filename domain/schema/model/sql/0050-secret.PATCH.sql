/*
 This patch helps to mitigate the issue when migrating a consumer model where
 few units consume secrets from an application on the offerer model (through a
 cross-model relation).

 The issue is that the secret_reference table contains an owner_application_uuid
 column, which is used to determine which remote application the secret belongs
 to. Through this reference, if the remote application is deleted, the owned
 secrets can be from the local model. However, the model description which is
 provided during a migration doesn't contain this information.
 This patch updates the secret_reference table to allow delaying the retrieval
 of the remote application information to the next secret update.

 TODO <<<<< TO MERGE THIS >>>>>
  you need to restore the trigger generation in domain/schema/model.go:
  - model/triggers/secret-triggers.gen.go : should also contains secret_reference
  - regenerate the triggers
  - replace changeLogTriggersForSecretReference by
    triggers.ChangeLogTriggersForSecretReference in the trigger declaration.
 */

CREATE TABLE secret_reference_new (
    secret_id TEXT NOT NULL PRIMARY KEY,
    latest_revision INT NOT NULL,

    -- owner_application_uuid is the application that owns the secret.
    -- It is used to determine which remote (offerer) application the secret
    -- belongs to. It is leveraged when the remote application is deleted from
    -- the point of view of the local model (consumer), in order to delete the
    -- secret from the local model.
    owner_application_uuid TEXT,

    -- updated_at is either the creation time of the reference, or the time it
    -- was migrated, or the last time it was fetched from a unit, and thus
    -- when the owner_application_uuid was defined.
    updated_at DATETIME NOT NULL,

    -- migrated is true if the reference was migrated from the old table.
    -- In this case, owner_application_uuid may be empty.
    migrated BOOLEAN NOT NULL DEFAULT false,
    CONSTRAINT fk_secret_id
    FOREIGN KEY (secret_id)
    REFERENCES secret (id),
    CONSTRAINT fk_secret_reference_application_uuid
    FOREIGN KEY (owner_application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT chk_owned_or_migrated
    CHECK ((owner_application_uuid IS NOT null AND owner_application_uuid != '') OR migrated)
);

INSERT INTO secret_reference_new
SELECT
    secret_id,
    latest_revision,
    owner_application_uuid,
    (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc')) AS updated_at,
    false AS migrated
FROM secret_reference;

-- Code after this point doesn't need to be merged, as long as the triggers are regenerated

-- sqlfluff doesn't support TRIGGER statements
-- noqa: disable=all
DROP TABLE secret_reference;
ALTER TABLE secret_reference_new RENAME TO secret_reference;

-- insert trigger for SecretReference
CREATE TRIGGER trg_log_secret_reference_insert
AFTER INSERT ON secret_reference FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10008, NEW.secret_id, DATETIME('now', 'utc'));
END;

-- update trigger for SecretReference
CREATE TRIGGER trg_log_secret_reference_update
AFTER UPDATE ON secret_reference FOR EACH ROW
WHEN
	NEW.secret_id != OLD.secret_id OR
	NEW.latest_revision != OLD.latest_revision OR
	(NEW.owner_application_uuid != OLD.owner_application_uuid OR (NEW.owner_application_uuid IS NOT NULL AND OLD.owner_application_uuid IS NULL) OR (NEW.owner_application_uuid IS NULL AND OLD.owner_application_uuid IS NOT NULL)) OR
	NEW.updated_at != OLD.updated_at OR
	NEW.migrated != OLD.migrated
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10008, OLD.secret_id, DATETIME('now', 'utc'));
END;

-- delete trigger for SecretReference
CREATE TRIGGER trg_log_secret_reference_delete
AFTER DELETE ON secret_reference FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10008, OLD.secret_id, DATETIME('now', 'utc'));
END;
-- noqa: enable=all
