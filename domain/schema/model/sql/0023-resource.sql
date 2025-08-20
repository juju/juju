CREATE TABLE resource_origin_type (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_resource_origin_name
ON resource_origin_type (name);

INSERT INTO resource_origin_type VALUES
(0, 'upload'),
(1, 'store');

CREATE TABLE resource_state (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_resource_state
ON resource_state (name);

-- Resource state values:
-- Available is the application resource which will be used by any units
-- at this point in time.
-- Potential indicates there is a different revision of the resource available
-- in a repository. Used to let users know a resource can be upgraded.
INSERT INTO resource_state VALUES
(0, 'available'),
(1, 'potential');

CREATE TABLE resource (
    uuid TEXT NOT NULL PRIMARY KEY,
    -- This indicates the resource name for the specific
    -- revision for which this resource is downloaded.
    charm_uuid TEXT NOT NULL,
    charm_resource_name TEXT NOT NULL,
    revision INT,
    origin_type_id INT NOT NULL,
    state_id INT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    -- last_polled is when the repository was last polled for new resource
    -- revisions. Only set if resource_state is 1 ("potential").
    last_polled TIMESTAMP,
    CONSTRAINT fk_charm_resource
    FOREIGN KEY (charm_uuid, charm_resource_name)
    REFERENCES charm_resource (charm_uuid, name),
    CONSTRAINT fk_resource_origin_type_id
    FOREIGN KEY (origin_type_id)
    REFERENCES resource_origin_type (id),
    CONSTRAINT fk_resource_state_id
    FOREIGN KEY (state_id)
    REFERENCES resource_state (id)
);

-- Links applications to the resources that they are *using*.
-- This resource may in turn be linked through to a *different* charm than the
-- application is using, because the charm_resource_name field indicates the
-- charm revision that it was acquired for at the time.
CREATE TABLE application_resource (
    resource_uuid TEXT NOT NULL PRIMARY KEY,
    application_uuid TEXT NOT NULL,
    CONSTRAINT fk_application_uuid
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_resource_uuid
    FOREIGN KEY (resource_uuid)
    REFERENCES resource (uuid)
);

-- Links a resource to an application which does not exist yet.
CREATE TABLE pending_application_resource (
    resource_uuid TEXT NOT NULL PRIMARY KEY,
    application_name TEXT NOT NULL,
    CONSTRAINT fk_resource_uuid
    FOREIGN KEY (resource_uuid)
    REFERENCES resource (uuid)
);

CREATE TABLE resource_retrieved_by_type (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_resource_retrieved_by_type
ON resource_retrieved_by_type (name);

INSERT INTO resource_retrieved_by_type VALUES
(0, 'user'),
(1, 'unit'),
(2, 'application');

CREATE TABLE resource_retrieved_by (
    resource_uuid TEXT NOT NULL PRIMARY KEY,
    retrieved_by_type_id INT NOT NULL,
    -- Name is the entity who retrieved the resource blob:
    --   The name of the user who uploaded the resource.
    --   Unit or application name of which triggered the download
    --     from a repository.
    name TEXT NOT NULL,
    CONSTRAINT fk_resource
    FOREIGN KEY (resource_uuid)
    REFERENCES resource (uuid),
    CONSTRAINT fk_resource_retrieved_by_type
    FOREIGN KEY (retrieved_by_type_id)
    REFERENCES resource_retrieved_by_type (id)
);

-- This is a resource used by to a unit.
CREATE TABLE unit_resource (
    resource_uuid TEXT NOT NULL,
    unit_uuid TEXT NOT NULL,
    added_at TIMESTAMP NOT NULL,
    CONSTRAINT fk_resource_uuid
    FOREIGN KEY (resource_uuid)
    REFERENCES resource (uuid),
    CONSTRAINT fk_resource_unit_uuid
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    PRIMARY KEY (resource_uuid, unit_uuid)
);

-- This is the actual store for container image resources. The metadata
-- necessary to retrieve the OCI Image from a registry.
CREATE TABLE resource_container_image_metadata_store (
    storage_key TEXT NOT NULL PRIMARY KEY,
    registry_path TEXT NOT NULL,
    username TEXT,
    password TEXT
);

-- Link table between a file resource and where its stored.
CREATE TABLE resource_file_store (
    resource_uuid TEXT NOT NULL PRIMARY KEY,
    store_uuid TEXT NOT NULL,
    size INTEGER, -- in bytes
    sha384 TEXT,
    CONSTRAINT fk_resource_uuid
    FOREIGN KEY (resource_uuid)
    REFERENCES resource (uuid),
    CONSTRAINT fk_store_uuid
    FOREIGN KEY (store_uuid)
    REFERENCES object_store_metadata (uuid)
);

-- Link table between a container image resource and where its stored.
CREATE TABLE resource_image_store (
    resource_uuid TEXT NOT NULL PRIMARY KEY,
    store_storage_key TEXT NOT NULL,
    size INTEGER, -- in bytes
    sha384 TEXT,
    CONSTRAINT fk_resource_uuid
    FOREIGN KEY (resource_uuid)
    REFERENCES resource (uuid),
    CONSTRAINT fk_store_uuid
    FOREIGN KEY (store_storage_key)
    REFERENCES resource_container_image_metadata_store (storage_key)
);

-- View of all resources with plain text enum types, and solved fields from charm table
CREATE VIEW v_resource AS
SELECT
    r.uuid,
    r.charm_resource_name AS name,
    r.created_at,
    r.revision,
    rot.name AS origin_type,
    rs.name AS state,
    rrb.name AS retrieved_by,
    rrbt.name AS retrieved_by_type,
    cr.path,
    cr.description,
    crk.name AS kind_name,
    -- Select the size and sha384 from whichever store contains the resource
    -- blob.
    COALESCE(rfs.size, ris.size) AS size,
    COALESCE(rfs.sha384, ris.sha384) AS sha384
FROM resource AS r
JOIN charm_resource AS cr ON r.charm_uuid = cr.charm_uuid AND r.charm_resource_name = cr.name
JOIN charm_resource_kind AS crk ON cr.kind_id = crk.id
JOIN resource_origin_type AS rot ON r.origin_type_id = rot.id
JOIN resource_state AS rs ON r.state_id = rs.id
LEFT JOIN resource_retrieved_by AS rrb ON r.uuid = rrb.resource_uuid
LEFT JOIN resource_retrieved_by_type AS rrbt ON rrb.retrieved_by_type_id = rrbt.id
LEFT JOIN resource_file_store AS rfs ON r.uuid = rfs.resource_uuid
LEFT JOIN resource_image_store AS ris ON r.uuid = ris.resource_uuid;

-- View of all resources linked to application
CREATE VIEW v_application_resource AS
SELECT
    r.uuid,
    r.name,
    r.created_at,
    r.revision,
    r.origin_type,
    r.state,
    r.retrieved_by,
    r.retrieved_by_type,
    r.path,
    r.description,
    r.kind_name,
    r.size,
    r.sha384,
    ar.application_uuid,
    a.name AS application_name
FROM v_resource AS r
LEFT JOIN application_resource AS ar ON r.uuid = ar.resource_uuid
LEFT JOIN application AS a ON ar.application_uuid = a.uuid;

-- View of all resources linked to units
CREATE VIEW v_unit_resource AS
SELECT
    r.uuid,
    r.name,
    r.created_at,
    r.revision,
    r.origin_type,
    r.state,
    r.retrieved_by,
    r.retrieved_by_type,
    r.path,
    r.description,
    r.kind_name,
    r.size,
    r.sha384,
    ur.unit_uuid,
    u.name AS unit_name,
    a.uuid AS application_uuid,
    a.name AS application_name
FROM v_resource AS r
JOIN unit_resource AS ur ON r.uuid = ur.resource_uuid
JOIN unit AS u ON ur.unit_uuid = u.uuid
JOIN application AS a ON u.application_uuid = a.uuid
