-- The application_endpoint ties an application's relation definition to an
-- endpoint binding via a space. A null space_uuid represents the endpoint
-- is bound to the application's default space. Each relation has 2 endpoints,
-- unless it is a peer relation. The space and charm relation combine represent
-- the endpoint binding of this application endpoint.
CREATE TABLE application_endpoint (
    uuid TEXT NOT NULL PRIMARY KEY,
    application_uuid TEXT NOT NULL,
    space_uuid TEXT,
    charm_relation_uuid TEXT NOT NULL,
    CONSTRAINT fk_application_uuid
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_space_uuid
    FOREIGN KEY (space_uuid)
    REFERENCES space (uuid),
    CONSTRAINT fk_charm_relation_uuid
    FOREIGN KEY (charm_relation_uuid)
    REFERENCES charm_relation (uuid)
);

CREATE INDEX idx_application_endpoint_app
ON application_endpoint (application_uuid);

CREATE UNIQUE INDEX idx_application_endpoint_app_relation
ON application_endpoint (application_uuid, charm_relation_uuid);

-- The application_endpoint ties an application's relation definition to an
-- endpoint binding via a space. Only endpoint bindings which differ from the
-- application default binding will be listed.
CREATE TABLE application_extra_endpoint (
    application_uuid TEXT NOT NULL,
    space_uuid TEXT,
    charm_extra_binding_uuid TEXT NOT NULL,
    CONSTRAINT fk_application_uuid
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_space_uuid
    FOREIGN KEY (space_uuid)
    REFERENCES space (uuid),
    CONSTRAINT fk_charm_extra_binding_uuid
    FOREIGN KEY (charm_extra_binding_uuid)
    REFERENCES charm_extra_binding (uuid),
    PRIMARY KEY (application_uuid, charm_extra_binding_uuid)
);

CREATE INDEX idx_application_extra_endpoint_app
ON application_extra_endpoint (application_uuid);

CREATE UNIQUE INDEX idx_application_extra_endpoint_app_relation
ON application_extra_endpoint (application_uuid, charm_extra_binding_uuid);

-- The relation_endpoint table links a relation to a single
-- application endpoint. If the relation is of type peer,
-- there will be one row in the table. If the relation has
-- a provider and requirer endpoint, there will be two rows
-- in the table.
CREATE TABLE relation_endpoint (
    uuid TEXT NOT NULL PRIMARY KEY,
    relation_uuid TEXT NOT NULL,
    endpoint_uuid TEXT NOT NULL,
    CONSTRAINT fk_relation_uuid
    FOREIGN KEY (relation_uuid)
    REFERENCES relation (uuid),
    CONSTRAINT fk_endpoint_uuid
    FOREIGN KEY (endpoint_uuid)
    REFERENCES application_endpoint (uuid)
);

CREATE UNIQUE INDEX idx_relation_endpoint
ON relation_endpoint (relation_uuid, endpoint_uuid);

-- The relation table represents a relation between two
-- applications, or a peer relation.
CREATE TABLE relation (
    uuid TEXT NOT NULL PRIMARY KEY,
    life_id INT NOT NULL,
    relation_id INT NOT NULL,
    suspended BOOLEAN DEFAULT FALSE,
    suspended_reason TEXT,
    -- NOTE: the scope of a relation is not just the same as the scope of either
    -- of it's endpoints. It's a property we need to consider as intrinsic to
    -- the relation itself. This is because a relation is considered
    -- container-scoped if either of it's endpoints are container-scoped.
    scope_id INT NOT NULL,
    CONSTRAINT fk_relation_life
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_relation_scope
    FOREIGN KEY (scope_id)
    REFERENCES charm_relation_scope (id)
);

CREATE UNIQUE INDEX idx_relation_id
ON relation (relation_id);

-- The relation_unit table links a relation to a specific unit.
CREATE TABLE relation_unit (
    uuid TEXT NOT NULL PRIMARY KEY,
    relation_endpoint_uuid TEXT NOT NULL,
    unit_uuid TEXT NOT NULL,
    CONSTRAINT fk_relation_unit_uuid
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    CONSTRAINT fk_relation_uuid
    FOREIGN KEY (relation_endpoint_uuid)
    REFERENCES relation_endpoint (uuid)
);

CREATE UNIQUE INDEX idx_relation_unit
ON relation_unit (relation_endpoint_uuid, unit_uuid);

-- The relation_unit_setting holds key value pair settings
-- for a relation at the unit level. Keys must be unique
-- per unit.
CREATE TABLE relation_unit_setting (
    relation_unit_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT,
    CONSTRAINT chk_key_empty CHECK ("key" != ''),
    CONSTRAINT fk_relation_unit_uuid
    FOREIGN KEY (relation_unit_uuid)
    REFERENCES relation_unit (uuid),
    PRIMARY KEY (relation_unit_uuid, "key")
);

CREATE INDEX idx_relation_unit_setting_unit
ON relation_unit_setting (relation_unit_uuid);

-- relation_unit_settings_hash holds a hash of all settings for a relation unit.
-- It allows watchers to easily determine when the relation units settings have
-- changed.
CREATE TABLE relation_unit_settings_hash (
    relation_unit_uuid TEXT NOT NULL PRIMARY KEY,
    sha256 TEXT NOT NULL,
    CONSTRAINT fk_relation_unit_setting_hash_relation_unit
    FOREIGN KEY (relation_unit_uuid)
    REFERENCES relation_unit (uuid)
);

-- relation_unit_setting_archive is used to fullfil a contract we have, whereby
-- the settings for a relation unit are accessible for the lifetime of a
-- relation, regardless of whether the unit has departed the relation, or even
-- exists any longer.
-- Upon leaving scope, we copy the unit's relation settings into this table.
-- Accessing relation settings via the relation-get hook tool will cause Juju to
-- check this table if the requested unit is not in scope.
-- We need no triggers for this table, because we copy the settings before doing
-- the relation_unit_settings deletion, and once copied they are static until
-- the relation itself is deleted.
CREATE TABLE relation_unit_setting_archive (
    relation_uuid TEXT NOT NULL,
    unit_name TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT,
    CONSTRAINT fk_relation_uuid
    FOREIGN KEY (relation_uuid)
    REFERENCES relation (uuid),
    PRIMARY KEY (relation_uuid, unit_name, "key")
);

-- The relation_application_setting holds key value pair settings
-- for a relation at the application level. Keys must be unique
-- per application.
CREATE TABLE relation_application_setting (
    relation_endpoint_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT,
    CONSTRAINT chk_key_empty CHECK ("key" != ''),
    CONSTRAINT fk_relation_endpoint_uuid
    FOREIGN KEY (relation_endpoint_uuid)
    REFERENCES relation_endpoint (uuid),
    PRIMARY KEY (relation_endpoint_uuid, "key")
);

CREATE INDEX idx_relation_ep_setting_ep
ON relation_application_setting (relation_endpoint_uuid);

-- relation_application_settings_hash holds a hash of all application settings
-- for a relation endpoint. It allows watchers to easily determine when the
-- relations application settings have changed.
CREATE TABLE relation_application_settings_hash (
    relation_endpoint_uuid TEXT NOT NULL PRIMARY KEY,
    sha256 TEXT NOT NULL,
    CONSTRAINT fk_relation_application_setting_hash_relation_endpoint
    FOREIGN KEY (relation_endpoint_uuid)
    REFERENCES relation_endpoint (uuid)
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
(4, 'suspended'),
(5, 'error');

-- The relation_status maps a relation to its status
-- as defined in the relation_status_type table.
CREATE TABLE relation_status (
    relation_uuid TEXT NOT NULL PRIMARY KEY,
    relation_status_type_id TEXT NOT NULL,
    message TEXT,
    updated_at DATETIME,
    CONSTRAINT fk_relation_uuid
    FOREIGN KEY (relation_uuid)
    REFERENCES relation (uuid),
    CONSTRAINT fk_relation_status_type_id
    FOREIGN KEY (relation_status_type_id)
    REFERENCES relation_status_type (id)
);

CREATE VIEW v_application_endpoint AS
SELECT
    ae.uuid AS application_endpoint_uuid,
    cr.name AS endpoint_name,
    ae.application_uuid,
    a.name AS application_name,
    cr.interface,
    cr.optional,
    cr.capacity,
    crr.name AS role,
    crs.name AS scope
FROM application_endpoint AS ae
JOIN application AS a ON ae.application_uuid = a.uuid
JOIN charm_relation AS cr ON ae.charm_relation_uuid = cr.uuid
JOIN charm_relation_role AS crr ON cr.role_id = crr.id
JOIN charm_relation_scope AS crs ON cr.scope_id = crs.id;

CREATE VIEW v_relation_endpoint AS
SELECT
    re.uuid AS relation_endpoint_uuid,
    re.endpoint_uuid AS application_endpoint_uuid,
    re.relation_uuid,
    ae.application_uuid,
    a.name AS application_name,
    cr.name AS endpoint_name,
    cr.interface,
    cr.optional,
    cr.capacity,
    crr.name AS role,
    crs.name AS scope
FROM relation_endpoint AS re
JOIN relation AS r ON re.relation_uuid = r.uuid
JOIN application_endpoint AS ae ON re.endpoint_uuid = ae.uuid
JOIN application AS a ON ae.application_uuid = a.uuid
JOIN charm_relation AS cr ON ae.charm_relation_uuid = cr.uuid
JOIN charm_relation_role AS crr ON cr.role_id = crr.id
JOIN charm_relation_scope AS crs ON r.scope_id = crs.id;

CREATE VIEW v_relation_endpoint_identifier AS
SELECT
    re.relation_uuid,
    a.name AS application_name,
    cr.name AS endpoint_name
FROM relation_endpoint AS re
JOIN application_endpoint AS ae ON re.endpoint_uuid = ae.uuid
JOIN charm_relation AS cr ON ae.charm_relation_uuid = cr.uuid
JOIN application AS a ON ae.application_uuid = a.uuid;

CREATE VIEW v_relation_status AS
SELECT
    rs.relation_uuid,
    rst.name AS status,
    rs.message,
    rs.updated_at
FROM relation_status AS rs
JOIN relation_status_type AS rst ON rs.relation_status_type_id = rst.id;
