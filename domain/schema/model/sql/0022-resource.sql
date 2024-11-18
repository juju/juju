CREATE TABLE resource_origin_type (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_resource_origin_name
ON resource_origin_type (name);

INSERT INTO resource_origin_type VALUES
(0, 'uploaded'),
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
    application_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    revision INT,
    origin_type_id INT NOT NULL,
    state_id INT NOT NULL,
    size INT,
    hash TEXT,
    hash_type_id TEXT,
    created_at TIMESTAMP NOT NULL,
    CONSTRAINT fk_resource_application_uuid
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_resource_name
    FOREIGN KEY (name)
    REFERENCES resource_meta (name),
    CONSTRAINT fk_resource_origin_type_id
    FOREIGN KEY (origin_type_id)
    REFERENCES resource_origin_type (id),
    CONSTRAINT fk_resource_state_id
    FOREIGN KEY (state_id)
    REFERENCES resource_state (id),
    CONSTRAINT fk_resource_hash_type_id
    FOREIGN KEY (hash_type_id)
    REFERENCES object_store_metadata_hash_type (id)
);

CREATE UNIQUE INDEX idx_resource ON resource (application_uuid, name, state_id);

CREATE TABLE resource_meta (
    application_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    type_id INT NOT NULL,
    path TEXT,
    description TEXT,
    CONSTRAINT fk_resource_type_id
    FOREIGN KEY (type_id)
    REFERENCES charm_resource_kind (id),
    PRIMARY KEY (application_uuid, name)
);

CREATE UNIQUE INDEX idx_resource_meta ON resource_meta (application_uuid, name);

CREATE TABLE resource_supplied_by_type (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_resource_supplied_by_type
ON resource_supplied_by_type (name);

INSERT INTO resource_supplied_by_type VALUES
(0, 'user'),
(1, 'unit'),
(2, 'application');

CREATE TABLE resource_supplied_by (
    uuid TEXT NOT NULL PRIMARY KEY,
    supplied_by_type_id INT NOT NULL,
    -- Name is the entity who supplied the resource blob:
    --   The name of the user who uploaded the resource.
    --   Unit or application name of which triggered the download
    --     from a repository.
    name TEXT NOT NULL,
    CONSTRAINT fk_resource_supplied_by_type
    FOREIGN KEY (supplied_by_type_id)
    REFERENCES resource_supplied_by_type (id)
);

CREATE UNIQUE INDEX idx_resource_supplied_by
ON resource_supplied_by (name);

CREATE TABLE application_resource (
    resource_uuid TEXT NOT NULL PRIMARY KEY,
    supplied_by_uuid TEXT,
    storage_path TEXT,
    CONSTRAINT fk_resource_uuid
    FOREIGN KEY (resource_uuid)
    REFERENCES resource (uuid),
    CONSTRAINT fk_resource_supplied_by_uuid
    FOREIGN KEY (supplied_by_uuid)
    REFERENCES resource_supplied_by (uuid)
);

-- Polled resource values from the repository.
CREATE TABLE repository_resource (
    resource_uuid TEXT NOT NULL PRIMARY KEY,
    last_polled TIMESTAMP NOT NULL,
    CONSTRAINT fk_resource_uuid
    FOREIGN KEY (resource_uuid)
    REFERENCES resource (uuid)
);

CREATE TABLE unit_resource (
    resource_uuid TEXT NOT NULL,
    unit_uuid TEXT NOT NULL,
    -- Download progress between the controller and the unit.
    download_progress INT,
    CONSTRAINT fk_resource_uuid
    FOREIGN KEY (resource_uuid)
    REFERENCES resource (uuid),
    CONSTRAINT fk_resource_unit_uuid
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    PRIMARY KEY (resource_uuid, unit_uuid)
);

CREATE TABLE resource_oci_image_metadata_store (
    resource_uuid TEXT NOT NULL,
    registry_path TEXT NOT NULL,
    username TEXT,
    password TEXT,
    CONSTRAINT fk_resource_uuid
    FOREIGN KEY (resource_uuid)
    REFERENCES resource (uuid)
);
