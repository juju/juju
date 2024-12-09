-- Copyright 2024 Canonical Ltd.
-- Licensed under the AGPLv3, see LICENCE file for details.

-- The relation table represents relations possible for
-- an application. Until life and the relation id are set,
-- the relation is not active in the model.
CREATE TABLE relation (
    uuid TEXT NOT NULL PRIMARY KEY,
    application_uuid TEXT NOT NULL,
    life_id INT,
    relation_id INT,
    CONSTRAINT fk_relation_life
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_application_uuid
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid)
);

-- The relation_unit table links a relation to a specific unit.
CREATE TABLE relation_unit (
    uuid TEXT NOT NULL PRIMARY KEY,
    relation_uuid TEXT NOT NULL,
    unit_uuid TEXT NOT NULL,
    in_scope BOOLEAN DEFAULT FALSE,
    departing BOOLEAN DEFAULT FALSE,
    CONSTRAINT fk_relation_unit_uuid
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    CONSTRAINT fk_relation_uuid
    FOREIGN KEY (relation_uuid)
    REFERENCES relation (uuid)
);

CREATE UNIQUE INDEX idx_relation_unit
ON relation_unit (relation_uuid, unit_uuid);

-- The relation_unit_setting holds key value
-- pair settings for a relation at the unit
-- level. Keys must be unique.
CREATE TABLE relation_unit_setting (
    relation_unit_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT,
    CONSTRAINT chk_key_empty CHECK ("key" != ''),
    CONSTRAINT fk_relation_unit_uuid
    FOREIGN KEY (relation_unit_uuid)
    REFERENCES relation_unit_uuid (uuid),
    PRIMARY KEY (relation_unit_uuid, "key")
);

-- The relation_application_setting holds key value
-- pair settings for a relation at the application
-- level. Keys must be unique.
CREATE TABLE relation_application_setting (
    relation_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT,
    CONSTRAINT chk_key_empty CHECK ("key" != ''),
    CONSTRAINT fk_relation_uuid
    FOREIGN KEY (relation_uuid)
    REFERENCES relation (uuid),
    PRIMARY KEY (relation_uuid, "key")
);

-- The application_endpoint ties a relation to its application,
-- relation definition and endpoint bindings. Each relation has
-- has 2 endpoints, unless it is a peer relation. The space
-- and charm relation combine represent the endpoint binding of
-- this application endpoint.
CREATE TABLE application_endpoint (
    relation_uuid TEXT NOT NULL PRIMARY KEY,
    application_uuid TEXT NOT NULL,
    space_uuid TEXT NOT NULL,
    charm_relation_uuid TEXT NOT NULL,
    CONSTRAINT fk_application_uuid
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_relation_uuid
    FOREIGN KEY (relation_uuid)
    REFERENCES relation (uuid),
    CONSTRAINT fk_space_uuid
    FOREIGN KEY (space_uuid)
    REFERENCES "space" (uuid),
    CONSTRAINT fk_charm_relation_uuid
    FOREIGN KEY (charm_relation_uuid)
    REFERENCES charm_relation (uuid)
);

-- The relation_status maps a relation to its status
-- as defined in the relation_status_type table.
CREATE TABLE relation_status (
    relation_uuid TEXT NOT NULL PRIMARY KEY,
    relation_status_type_id TEXT NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT fk_relation_uuid
    FOREIGN KEY (relation_uuid)
    REFERENCES relation (uuid),
    CONSTRAINT fk_relation_status_type_id
    FOREIGN KEY (relation_status_type_id)
    REFERENCES relation_status_type (id)
);

CREATE TABLE relation_status_type (
    id TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_relation_status_type_name
ON relation_status_type (name);

INSERT INTO relation_status_type VALUES
(0, 'joining'),
(1, 'joined'),
(2, 'broken'),
(3, 'suspending'),
(4, 'suspended');

-- The relation_suspended_reason table keeps track of
-- the reason a relation was suspended. Providing a
-- reason is option for a user.
CREATE TABLE relation_suspended_reason (
    relation_uuid TEXT NOT NULL PRIMARY KEY,
    value TEXT NOT NULL,
    CONSTRAINT fk_relation_uuid
    FOREIGN KEY (relation_uuid)
    REFERENCES relation (uuid)
);

-- The relation_sequence table is used to keep track of the
-- sequence number for relation IDs within a model. Each
-- active application endpoint pair has a relation ID.
CREATE TABLE relation_sequence (
     -- The sequence number will start at 0 for each model and will be
     -- incremented.
     sequence INT NOT NULL DEFAULT 0
);
