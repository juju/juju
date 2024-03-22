// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"github.com/juju/juju/core/database/schema"
)

// secretBackendSchema provides a helper function for generating a secret backend related DDL for the controller database.
func secretBackendSchema() schema.Patch {
	return schema.MakePatch(`
-- Controller database tables for secret backends.

CREATE TABLE
    secret_backend_type (
        id INT PRIMARY KEY,
        type TEXT NOT NULL,
        description TEXT,
        CONSTRAINT chk_empty_type
            CHECK(type != ''),
        CONSTRAINT uniq_secret_backend_type_type
            UNIQUE(type)
    );

INSERT INTO secret_backend_type VALUES
    (0, 'internal', 'the juju internal secret backend'),
    (1, 'vault', 'the vault secret backend'),
    (2, 'kubernetes', 'the kubernetes secret backend');

CREATE TABLE
    secret_backend (
        uuid TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        backend_type TEXT NOT NULL,
        token_rotate_interval INT,
        CONSTRAINT chk_empty_name
            CHECK(name != ''),
        CONSTRAINT fk_secret_backend_type
            FOREIGN KEY (backend_type)
            REFERENCES secret_backend_type (type)
    );

CREATE UNIQUE INDEX idx_secret_backend_name ON secret_backend (name);

-- We need to ensure that the internal and kubernetes backends are immutable.
-- They are created by the controller during bootstrap time.
-- The reason why we need these triggers is because we want to ensure that they
-- should never ever be deleted or updated.
CREATE TRIGGER trg_secret_backend_immutable_backends_update
    BEFORE UPDATE ON secret_backend
    FOR EACH ROW
    WHEN OLD.backend_type IN ('internal', 'kubernetes')
    BEGIN
        SELECT RAISE(FAIL, 'secret backends with type internal or kubernetes are immutable');
    END;

CREATE TRIGGER trg_secret_backend_immutable_backends_delete
    BEFORE DELETE ON secret_backend
    FOR EACH ROW
    WHEN OLD.backend_type IN ('internal', 'kubernetes')
    BEGIN
        SELECT RAISE(FAIL, 'secret backends with type internal or kubernetes are immutable');
    END;

CREATE TABLE
    secret_backend_config (
        backend_uuid TEXT NOT NULL,
        name TEXT NOT NULL,
        content TEXT NOT NULL,
        CONSTRAINT chk_empty_name
            CHECK(name != ''),
        CONSTRAINT chk_empty_content
            CHECK(content != ''),
        CONSTRAINT pk_secret_backend_config
            PRIMARY KEY (backend_uuid, name),
        CONSTRAINT fk_secret_backend_config_backend_uuid
            FOREIGN KEY (backend_uuid)
            REFERENCES secret_backend (uuid)
    );

CREATE TABLE
    secret_backend_rotation (
        backend_uuid TEXT PRIMARY KEY,
        next_rotation_time DATETIME NOT NULL,
        CONSTRAINT fk_secret_backend_rotation_secret_backend_uuid
            FOREIGN KEY (backend_uuid)
            REFERENCES secret_backend (uuid)
    );

CREATE TABLE
    model_secret_backend (
        model_uuid TEXT PRIMARY KEY,
        -- TODO: change the secret_backend_uuid to NOT NULL once we start to insert the
        -- internal and kubernetes secret backends into the secret_backend table.
        secret_backend_uuid TEXT,
        CONSTRAINT fk_model_secret_backend_model_uuid
            FOREIGN KEY (model_uuid)
            REFERENCES model_list (uuid),
        CONSTRAINT fk_model_secret_backend_secret_backend_uuid
            FOREIGN KEY (secret_backend_uuid)
            REFERENCES secret_backend (uuid)
    );

CREATE VIEW v_model_secret_backend AS
    SELECT
        msb.model_uuid AS uuid,
        mm.name,
        mt.type,
        msb.secret_backend_uuid
    FROM model_secret_backend msb
    INNER JOIN model_metadata mm ON msb.model_uuid = mm.model_uuid
    INNER JOIN model_type mt ON mm.model_type_id = mt.id;
`)
}

// secretSchema provides a helper function for generating a secret related DDL for the model database.
func secretSchema() schema.Patch {
	return schema.MakePatch(`
-- Model database tables for secrets.

CREATE TABLE
    secret_rotate_policy (
        id INT PRIMARY KEY,
        policy TEXT NOT NULL,
        CONSTRAINT chk_empty_policy
            CHECK(policy != '')
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

CREATE TABLE
    secret (
        uuid TEXT PRIMARY KEY,
        version INT,
        description TEXT,
        rotate_policy TEXT,
        auto_prune BOOLEAN NOT NULL DEFAULT (FALSE),
        create_time DATETIME NOT NULL DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc')),
        update_time DATETIME NOT NULL DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc')),
        CONSTRAINT fk_secret_rotate_policy
            FOREIGN KEY (rotate_policy)
            REFERENCES secret_rotate_policy (policy)
    );

CREATE TABLE
    secret_rotation (
        secret_uuid TEXT PRIMARY KEY,
        next_rotation_time DATETIME NOT NULL,
        CONSTRAINT fk_secret_rotation_secret_uuid
            FOREIGN KEY (secret_uuid)
            REFERENCES secret (uuid)
    );

-- 1:1
CREATE TABLE
    secret_value_ref (
        revision_uuid TEXT PRIMARY KEY,
        -- backend_uuid is the UUID of the backend in the controller database.
        backend_uuid TEXT NOT NULL,
        revision_id TEXT NOT NULL,
        CONSTRAINT fk_secret_value_ref_secret_revision_uuid
            FOREIGN KEY (revision_uuid)
            REFERENCES secret_revision (uuid)
    );

-- 1:many
CREATE TABLE
    secret_content (
        revision_uuid TEXT NOT NULL,
        name TEXT NOT NULL,
        content TEXT NOT NULL,
        CONSTRAINT chk_empty_name
            CHECK(name != ''),
        CONSTRAINT chk_empty_content
            CHECK(content != ''),
        CONSTRAINT pk_secret_content_revision_uuid_name
            PRIMARY KEY (revision_uuid,name),
        CONSTRAINT fk_secret_content_secret_revision_uuid
            FOREIGN KEY (revision_uuid)
            REFERENCES secret_revision (uuid)
    );

CREATE INDEX idx_secret_content_revision_uuid ON secret_content (revision_uuid);

CREATE TABLE
    secret_revision (
        uuid TEXT PRIMARY KEY,
        secret_uuid TEXT NOT NULL,
        revision INT NOT NULL,
        obsolete BOOLEAN NOT NULL DEFAULT (FALSE),
        -- pending_delete is true if the revision is to be deleted.
        -- It will not be drained to a new active backend.
        pending_delete BOOLEAN NOT NULL DEFAULT (FALSE),
        create_time DATETIME NOT NULL DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc')),
        update_time DATETIME NOT NULL DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc')),
        CONSTRAINT fk_secret_revision_secret_uuid 
            FOREIGN KEY (secret_uuid)
            REFERENCES secret (uuid)
    );

CREATE UNIQUE INDEX idx_secret_revision_secret_uuid_revision ON secret_revision (secret_uuid,revision);

CREATE TABLE
    secret_revision_expire (
        revision_uuid TEXT PRIMARY KEY,
        next_expire_time DATETIME NOT NULL,
        CONSTRAINT fk_secret_revision_expire_revision_uuid
            FOREIGN KEY (revision_uuid)
            REFERENCES secret_revision (uuid)
    );

CREATE TABLE
    secret_application_owner (
        secret_uuid TEXT PRIMARY KEY,
        application_uuid TEXT NOT NULL,
        label TEXT,
        CONSTRAINT fk_secret_application_owner_secret_uuid
            FOREIGN KEY (secret_uuid)
            REFERENCES secret (uuid),
        CONSTRAINT fk_secret_application_owner_application_uuid
            FOREIGN KEY (application_uuid)
            REFERENCES application (uuid)
    );

-- We need to ensure the label is unique per the application.
CREATE UNIQUE INDEX idx_secret_application_owner_label ON secret_application_owner (label,application_uuid);

CREATE TABLE
    secret_unit_owner (
        secret_uuid TEXT PRIMARY KEY,
        unit_uuid TEXT NOT NULL,
        label TEXT,
        CONSTRAINT fk_secret_unit_owner_secret_uuid
            FOREIGN KEY (secret_uuid)
            REFERENCES secret (uuid),
        CONSTRAINT fk_secret_unit_owner_unit_uuid
            FOREIGN KEY (unit_uuid)
            REFERENCES unit (uuid)
    );

-- We need to ensure the label is unique per unit.
CREATE UNIQUE INDEX idx_secret_unit_owner_label ON secret_unit_owner (label,unit_uuid);

CREATE TABLE
    secret_model_owner (
        secret_uuid TEXT PRIMARY KEY,
        label TEXT,
        CONSTRAINT fk_secret_model_owner_secret_uuid
            FOREIGN KEY (secret_uuid)
            REFERENCES secret (uuid)
    );

CREATE UNIQUE INDEX idx_secret_model_owner_label ON secret_model_owner (label);

CREATE TABLE
    secret_application_consumer (
        uuid TEXT PRIMARY KEY,
        secret_uuid TEXT NOT NULL,
        application_uuid TEXT NOT NULL,
        label TEXT,
        current_revision INT NOT NULL,
        CONSTRAINT fk_secret_application_consumer_secret_uuid
            FOREIGN KEY (secret_uuid)
            REFERENCES secret (uuid),
        CONSTRAINT fk_secret_application_consumer_application_uuid
            FOREIGN KEY (application_uuid)
            REFERENCES application (uuid)
    );
CREATE UNIQUE INDEX idx_secret_application_consumer_secret_uuid_application_uuid ON secret_application_consumer (secret_uuid,application_uuid);
CREATE UNIQUE INDEX idx_secret_application_consumer_label ON secret_application_consumer (label,application_uuid);

CREATE TABLE
    secret_unit_consumer (
        uuid TEXT PRIMARY KEY,
        secret_uuid TEXT NOT NULL,
        unit_uuid TEXT NOT NULL,
        label TEXT,
        current_revision INT NOT NULL, 
        CONSTRAINT fk_secret_unit_consumer_secret_uuid
            FOREIGN KEY (secret_uuid)
            REFERENCES secret (uuid),
        CONSTRAINT fk_secret_unit_consumer_unit_uuid
            FOREIGN KEY (unit_uuid)
            REFERENCES unit (uuid)
    );

CREATE UNIQUE INDEX idx_secret_unit_consumer_secret_uuid_unit_uuid ON secret_unit_consumer (secret_uuid,unit_uuid);
CREATE UNIQUE INDEX idx_secret_unit_consumer_label ON secret_unit_consumer (label,unit_uuid);

CREATE TABLE
    secret_remote_application_consumer (
        uuid TEXT PRIMARY KEY,
        secret_uuid TEXT NOT NULL,
        application_uuid TEXT NOT NULL,
        current_revision INT NOT NULL,
        CONSTRAINT fk_secret_remote_application_consumer_secret_uuid
            FOREIGN KEY (secret_uuid)
            REFERENCES secret (uuid),
        CONSTRAINT fk_secret_remote_application_consumer_application_uuid
            FOREIGN KEY (application_uuid)
            REFERENCES application (uuid)
    );

CREATE UNIQUE INDEX idx_secret_remote_application_consumer_secret_uuid_application_uuid ON secret_remote_application_consumer (secret_uuid,application_uuid);

CREATE TABLE
    secret_remote_unit_consumer (
        uuid TEXT PRIMARY KEY,
        secret_uuid TEXT NOT NULL,
        unit_uuid TEXT NOT NULL,
        current_revision INT NOT NULL,
        CONSTRAINT fk_secret_remote_unit_consumer_secret_uuid
            FOREIGN KEY (secret_uuid)
            REFERENCES secret (uuid),
        CONSTRAINT fk_secret_remote_unit_consumer_unit_uuid
            FOREIGN KEY (unit_uuid)
            REFERENCES unit (uuid)
    );

CREATE UNIQUE INDEX idx_secret_remote_unit_consumer_secret_uuid_unit_uuid ON secret_remote_unit_consumer (secret_uuid,unit_uuid);

CREATE TABLE
    secret_role (
        id INT PRIMARY KEY,
        role TEXT
    );

CREATE UNIQUE INDEX idx_secret_role_role ON secret_role (role);

INSERT INTO secret_role VALUES
    (0, 'view'),
    (1, 'rotate'),
    (2, 'manage');

-- TODO: probably we need a model level permission table like the
-- recently added "permission" table in the controller database.
CREATE TABLE
    secret_permission (
        uuid TEXT PRIMARY KEY,
        scope TEXT NOT NULL,
        subject TEXT NOT NULL,
        role TEXT NOT NULL,
        CONSTRAINT chk_empty_scope
            CHECK(scope != ''),
        CONSTRAINT chk_empty_subject
            CHECK(subject != ''),
        CONSTRAINT fk_secret_permission_secret_role_id
            FOREIGN KEY (role)
            REFERENCES secret_role (role)
    );
`)
}
