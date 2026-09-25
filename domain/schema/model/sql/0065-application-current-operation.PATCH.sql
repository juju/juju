/*
Replace the application_scale table with the application_provisioning_state
table. The scaling boolean column is replaced by a current_operation column,
so that multiple provisioning operation types (scale, storage-update) can be
represented in a single field.

Existing rows are migrated so that scaling = TRUE becomes
current_operation = 'scale' and all other rows become current_operation = ''.

Dropping application_scale also removes the changestream triggers that were
created for it. They are recreated here for application_provisioning_state,
reusing the existing change log namespace (id 10018, registered as
"application_scale" — this matches tableApplicationScale in
domain/schema/model.go and keeps existing watcher subscriptions working).
The namespace row already exists in both fresh and upgraded databases, as it
is inserted before post-patch files are applied, so it must not be inserted
again here.
 */

CREATE TABLE application_provisioning_state (
    application_uuid TEXT NOT NULL PRIMARY KEY,
    scale INT,
    scale_target INT,
    current_operation TEXT NOT NULL DEFAULT '',
    CONSTRAINT fk_application_provisioning_state_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid)
);

INSERT INTO application_provisioning_state
SELECT
    application_uuid,
    scale,
    scale_target,
    CASE WHEN scaling = TRUE THEN 'scale' ELSE '' END AS current_operation
FROM application_scale;

DROP TABLE application_scale;

-- insert trigger for ApplicationProvisioningState
CREATE TRIGGER trg_log_application_provisioning_state_insert
AFTER INSERT ON application_provisioning_state FOR EACH ROW
BEGIN
INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
VALUES (1, 10018, new.application_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for ApplicationProvisioningState
-- sqlfluff doesn't support TRIGGER statements
-- noqa: disable=all
CREATE TRIGGER trg_log_application_provisioning_state_update
AFTER UPDATE ON application_provisioning_state FOR EACH ROW
WHEN
    NEW.application_uuid != OLD.application_uuid OR
    (NEW.scale != OLD.scale OR (NEW.scale IS NOT NULL AND OLD.scale IS NULL) OR (NEW.scale IS NULL AND OLD.scale IS NOT NULL)) OR
    (NEW.scale_target != OLD.scale_target OR (NEW.scale_target IS NOT NULL AND OLD.scale_target IS NULL) OR (NEW.scale_target IS NULL AND OLD.scale_target IS NOT NULL)) OR
    NEW.current_operation != OLD.current_operation
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10018, OLD.application_uuid, DATETIME('now', 'utc'));
END;
-- noqa: enable=all

-- delete trigger for ApplicationProvisioningState
CREATE TRIGGER trg_log_application_provisioning_state_delete
AFTER DELETE ON application_provisioning_state FOR EACH ROW
BEGIN
INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
VALUES (4, 10018, old.application_uuid, DATETIME('now', 'utc'));
END;
