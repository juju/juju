-- Model database tables for secrets.

CREATE TABLE secret_rotate_policy (
    id INT PRIMARY KEY,
    policy TEXT NOT NULL,
    CONSTRAINT chk_empty_policy
    CHECK (policy != '')
);

CREATE UNIQUE INDEX idx_secret_rotate_policy_policy ON secret_rotate_policy (policy);

INSERT INTO secret_rotate_policy VALUES
(0, 'never'),
(1, 'hourly'),
(2, 'daily'),
(3, 'weekly'),
(4, 'monthly'),
(5, 'quarterly'),
(6, 'yearly');

CREATE TABLE secret (
    id TEXT PRIMARY KEY
);

-- secret_reference stores details about
-- secrets hosted by another model and
-- is used on the consumer side of cross
-- model secrets.
CREATE TABLE secret_reference (
    secret_id TEXT NOT NULL PRIMARY KEY,
    latest_revision INT NOT NULL,
    CONSTRAINT fk_secret_id
    FOREIGN KEY (secret_id)
    REFERENCES secret (id)
);

CREATE TABLE secret_metadata (
    secret_id TEXT NOT NULL PRIMARY KEY,
    version INT NOT NULL,
    description TEXT,
    rotate_policy_id INT NOT NULL,
    auto_prune BOOLEAN NOT NULL DEFAULT (FALSE),
    create_time DATETIME NOT NULL DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc')),
    update_time DATETIME NOT NULL DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc')),
    CONSTRAINT fk_secret_id
    FOREIGN KEY (secret_id)
    REFERENCES secret (id),
    CONSTRAINT fk_secret_rotate_policy
    FOREIGN KEY (rotate_policy_id)
    REFERENCES secret_rotate_policy (id)
);

CREATE TABLE secret_rotation (
    secret_id TEXT NOT NULL PRIMARY KEY,
    next_rotation_time DATETIME NOT NULL,
    CONSTRAINT fk_secret_rotation_secret_metadata_id
    FOREIGN KEY (secret_id)
    REFERENCES secret_metadata (secret_id)
);

-- 1:1
CREATE TABLE secret_value_ref (
    revision_uuid TEXT NOT NULL PRIMARY KEY,
    -- backend_uuid is the UUID of the backend in the controller database.
    backend_uuid TEXT NOT NULL,
    revision_id TEXT NOT NULL,
    CONSTRAINT fk_secret_value_ref_secret_revision_uuid
    FOREIGN KEY (revision_uuid)
    REFERENCES secret_revision (uuid)
);

-- 1:many
CREATE TABLE secret_content (
    revision_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    content TEXT NOT NULL,
    CONSTRAINT chk_empty_name
    CHECK (name != ''),
    CONSTRAINT chk_empty_content
    CHECK (content != ''),
    CONSTRAINT pk_secret_content_revision_uuid_name
    PRIMARY KEY (revision_uuid, name),
    CONSTRAINT fk_secret_content_secret_revision_uuid
    FOREIGN KEY (revision_uuid)
    REFERENCES secret_revision (uuid)
);

CREATE INDEX idx_secret_content_revision_uuid ON secret_content (revision_uuid);

CREATE TABLE secret_revision (
    uuid TEXT NOT NULL PRIMARY KEY,
    secret_id TEXT NOT NULL,
    revision INT NOT NULL,
    create_time DATETIME NOT NULL DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc')),
    CONSTRAINT fk_secret_revision_secret_metadata_id
    FOREIGN KEY (secret_id)
    REFERENCES secret_metadata (secret_id)
);

CREATE UNIQUE INDEX idx_secret_revision_secret_id_revision ON secret_revision (secret_id, revision);

CREATE TABLE secret_revision_obsolete (
    revision_uuid TEXT NOT NULL PRIMARY KEY,
    obsolete BOOLEAN NOT NULL DEFAULT (FALSE),
    -- pending_delete is true if the revision is to be deleted.
    -- It will not be drained to a new active backend.
    pending_delete BOOLEAN NOT NULL DEFAULT (FALSE),
    CONSTRAINT fk_secret_revision_obsolete_revision_uuid
    FOREIGN KEY (revision_uuid)
    REFERENCES secret_revision (uuid)
);

CREATE TABLE secret_revision_expire (
    revision_uuid TEXT NOT NULL PRIMARY KEY,
    expire_time DATETIME NOT NULL,
    CONSTRAINT fk_secret_revision_expire_revision_uuid
    FOREIGN KEY (revision_uuid)
    REFERENCES secret_revision (uuid)
);

CREATE TABLE secret_application_owner (
    secret_id TEXT NOT NULL,
    application_uuid TEXT NOT NULL,
    label TEXT,
    CONSTRAINT fk_secret_application_owner_secret_metadata_id
    FOREIGN KEY (secret_id)
    REFERENCES secret_metadata (secret_id),
    CONSTRAINT fk_secret_application_owner_application_uuid
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    PRIMARY KEY (secret_id, application_uuid)
);

CREATE INDEX idx_secret_application_owner_secret_id ON secret_application_owner (secret_id);
-- We need to ensure the label is unique per the application.
CREATE UNIQUE INDEX idx_secret_application_owner_label ON secret_application_owner (label, application_uuid) WHERE label != '';

CREATE TABLE secret_unit_owner (
    secret_id TEXT NOT NULL,
    unit_uuid TEXT NOT NULL,
    label TEXT,
    CONSTRAINT fk_secret_unit_owner_secret_metadata_id
    FOREIGN KEY (secret_id)
    REFERENCES secret_metadata (secret_id),
    CONSTRAINT fk_secret_unit_owner_unit_uuid
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    PRIMARY KEY (secret_id, unit_uuid)
);

CREATE INDEX idx_secret_unit_owner_secret_id ON secret_unit_owner (secret_id);
-- We need to ensure the label is unique per unit.
CREATE UNIQUE INDEX idx_secret_unit_owner_label ON secret_unit_owner (label, unit_uuid) WHERE label != '';

CREATE TABLE secret_model_owner (
    secret_id TEXT NOT NULL PRIMARY KEY,
    label TEXT,
    CONSTRAINT fk_secret_model_owner_secret_metadata_id
    FOREIGN KEY (secret_id)
    REFERENCES secret_metadata (secret_id)
);

CREATE UNIQUE INDEX idx_secret_model_owner_label ON secret_model_owner (label) WHERE label != '';

CREATE TABLE secret_unit_consumer (
    secret_id TEXT NOT NULL,
    -- source model uuid may be this model or a different model
    -- possibly on another controller
    source_model_uuid TEXT NOT NULL,
    unit_uuid TEXT NOT NULL,
    label TEXT,
    current_revision INT NOT NULL,
    CONSTRAINT fk_secret_unit_consumer_unit_uuid
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    CONSTRAINT fk_secret_unit_consumer_secret_id
    FOREIGN KEY (secret_id)
    REFERENCES secret (id)
);

CREATE UNIQUE INDEX idx_secret_unit_consumer_secret_id_unit_uuid ON secret_unit_consumer (secret_id, unit_uuid);
CREATE UNIQUE INDEX idx_secret_unit_consumer_label ON secret_unit_consumer (label, unit_uuid) WHERE label != '';

-- This table records the tracked revisions from
-- units in the consuming model for cross model secrets.
CREATE TABLE secret_remote_unit_consumer (
    secret_id TEXT NOT NULL,
    -- unit_id is the anonymised name of the unit
    -- from the consuming model.
    unit_id TEXT NOT NULL,
    current_revision INT NOT NULL,
    CONSTRAINT fk_secret_remote_unit_consumer_secret_metadata_id
    FOREIGN KEY (secret_id)
    REFERENCES secret_metadata (secret_id)
);

CREATE UNIQUE INDEX idx_secret_remote_unit_consumer_secret_id_unit_id ON secret_remote_unit_consumer (secret_id, unit_id);

CREATE TABLE secret_role (
    id INT PRIMARY KEY,
    role TEXT
);

CREATE UNIQUE INDEX idx_secret_role_role ON secret_role (role);

INSERT INTO secret_role VALUES
(0, 'none'),
(1, 'view'),
(2, 'manage');

CREATE TABLE secret_grant_subject_type (
    id INT PRIMARY KEY,
    type TEXT
);

INSERT INTO secret_grant_subject_type VALUES
(0, 'unit'),
(1, 'application'),
(2, 'model'),
(3, 'remote-application');

CREATE TABLE secret_grant_scope_type (
    id INT PRIMARY KEY,
    type TEXT
);

INSERT INTO secret_grant_scope_type VALUES
(0, 'unit'),
(1, 'application'),
(2, 'model'),
(3, 'relation');

CREATE TABLE secret_permission (
    secret_id TEXT NOT NULL,
    role_id INT NOT NULL,
    -- subject_uuid is the entity which
    -- has been granted access to a secret.
    -- It will be an application, unit, or model uuid.
    subject_uuid TEXT NOT NULL,
    subject_type_id INT NOT NULL,
    -- scope_uuid is the entity which
    -- defines the scope of the grant.
    -- It will be an application, unit, relation, or model uuid.
    scope_uuid TEXT NOT NULL,
    scope_type_id TEXT NOT NULL,
    CONSTRAINT pk_secret_permission_secret_id_subject_uuid
    PRIMARY KEY (secret_id, subject_uuid),
    CONSTRAINT chk_empty_scope_uuid
    CHECK (scope_uuid != ''),
    CONSTRAINT chk_empty_subject_uuid
    CHECK (subject_uuid != ''),
    CONSTRAINT fk_secret_permission_secret_id
    FOREIGN KEY (secret_id)
    REFERENCES secret_metadata (secret_id),
    CONSTRAINT fk_secret_permission_secret_role_id
    FOREIGN KEY (role_id)
    REFERENCES secret_role (id),
    CONSTRAINT fk_secret_permission_secret_grant_subject_type_id
    FOREIGN KEY (subject_type_id)
    REFERENCES secret_grant_subject_type (id),
    CONSTRAINT fk_secret_permission_secret_grant_scope_type_id
    FOREIGN KEY (scope_type_id)
    REFERENCES secret_grant_scope_type (id)
);

CREATE INDEX idx_secret_permission_secret_id ON secret_permission (secret_id);
CREATE INDEX idx_secret_permission_subject_uuid_subject_type_id ON secret_permission (subject_uuid, subject_type_id);

CREATE VIEW v_secret_permission AS
SELECT
    sp.secret_id,
    sp.role_id,
    sp.subject_type_id,
    sp.scope_type_id,
    -- subject_id is the natural id of the subject entity (uuid for model)
    (CASE
        WHEN sp.subject_type_id = 0 THEN suu.unit_id
        WHEN sp.subject_type_id = 1 THEN sua.name
        WHEN sp.subject_type_id = 2 THEN m.uuid
        -- TODO: we don't have a remote-application table yet
        WHEN sp.subject_type_id = 3 THEN sp.subject_uuid
    END) AS subject_id,
    -- scope_id is the natural id of the scope entity (uuid for model)
    (CASE
        WHEN sp.scope_type_id = 0 THEN scu.unit_id
        WHEN sp.scope_type_id = 1 THEN sca.name
        WHEN sp.scope_type_id = 2 THEN m.uuid
        -- TODO: we don't have a relation table yet
        WHEN sp.scope_type_id = 3 THEN sp.scope_uuid
    END) AS scope_id
FROM secret_permission AS sp
LEFT JOIN unit AS suu ON sp.subject_uuid = suu.uuid
LEFT JOIN application AS sua ON sp.subject_uuid = sua.uuid
LEFT JOIN unit AS scu ON sp.scope_uuid = scu.uuid
LEFT JOIN application AS sca ON sp.scope_uuid = sca.uuid
INNER JOIN model AS m;
