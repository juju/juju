CREATE TABLE charm_source (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

-- The charm table exists as the nexus to all charm data.
--
-- The fact that the charm is in the database indicates that it's a placeholder.
-- Updating the available flag to true indicates that the charm is now available
-- for deployment.
CREATE TABLE charm (
    uuid TEXT NOT NULL PRIMARY KEY,
    -- Archive path is the path to the charm archive on disk. This is used to
    -- determine the source of the charm.
    archive_path TEXT,
    object_store_uuid TEXT,

    available BOOLEAN DEFAULT FALSE,

    version TEXT,
    lxd_profile TEXT,

    -- The following fields are purely here to reconstruct the charm URL.
    -- Once we have the ability to only talk about charms in terms of a UUID,
    -- these fields can be removed.
    -- These values are not intended to be used for any other purpose, they
    -- should not be used as a way to "derive" the charm origin. That concept
    -- is for applications.
    -- Note: revision is used to create lxd profile names, if removed, the
    -- unique profile naming must be resolved.

    source_id INT NOT NULL DEFAULT 1,
    revision INT NOT NULL DEFAULT -1,

    -- architecture_id may be null for local charms, but must be NULL for
    -- CMR charms as they are architecture agnostic.
    architecture_id INT,

    -- reference_name is the name of the charm that was originally supplied.
    -- The charm name can be different from the actual charm name in the
    -- metadata. If it's downloaded from charmhub the reference_name will be
    -- the name of the charm in the charmhub store. This is the transient
    -- name of the charm.
    --
    -- This can happen if the charm was uploaded to charmhub with a different
    -- name than the charm name in the metadata.yaml file.
    reference_name TEXT NOT NULL,

    -- create_time is purely used for ordering a charm by time, as we can't
    -- use the revision number to determine the order of the charm.
    create_time DATETIME NOT NULL DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc')),

    CONSTRAINT fk_charm_source_source
    FOREIGN KEY (source_id)
    REFERENCES charm_source (id),
    CONSTRAINT fk_charm_architecture
    FOREIGN KEY (architecture_id)
    REFERENCES architecture (id),
    CONSTRAINT fk_charm_object_store_metadata
    FOREIGN KEY (object_store_uuid)
    REFERENCES object_store_metadata (uuid),

    -- Ensure we have an architecture if the source is local or charmhub.
    CONSTRAINT chk_charm_architecture
    CHECK (((source_id = 0 OR source_id = 1) AND architecture_id >= 0) OR (source_id = 2 AND architecture_id IS NULL)),

    -- Ensure we don't have an empty reference
    CONSTRAINT chk_charm_reference_name
    CHECK (reference_name <> '')
);

CREATE TABLE charm_storage_kind (
    id INT PRIMARY KEY,
    kind TEXT NOT NULL
);

CREATE TABLE charm_storage (
    charm_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    storage_kind_id INT NOT NULL,
    shared BOOLEAN NOT NULL,
    read_only BOOLEAN,
    count_min INT NOT NULL,
    count_max INT NOT NULL,
    minimum_size_mib INT,
    location TEXT,
    CONSTRAINT fk_charm_storage_charm_storage_kind
    FOREIGN KEY (storage_kind_id)
    REFERENCES charm_storage_kind (id),
    CONSTRAINT fk_charm_storage_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    PRIMARY KEY (charm_uuid, name)
);

CREATE TABLE application (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL,
    life_id INT NOT NULL,
    charm_uuid TEXT NOT NULL,
    charm_modified_version INT NOT NULL DEFAULT 0,
    charm_upgrade_on_error BOOLEAN DEFAULT FALSE,
    -- space_uuid is the default binding for this application.
    space_uuid TEXT NOT NULL,
    CONSTRAINT fk_application_life
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_application_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    CONSTRAINT fk_space_uuid
    FOREIGN KEY (space_uuid)
    REFERENCES space (uuid)
);

CREATE TABLE unit (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL,
    life_id INT NOT NULL,
    application_uuid TEXT NOT NULL,
    net_node_uuid TEXT NOT NULL,
    charm_uuid TEXT NOT NULL,
    password_hash_algorithm_id INT,
    password_hash TEXT,
    CONSTRAINT fk_unit_life
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_unit_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_unit_net_node
    FOREIGN KEY (net_node_uuid)
    REFERENCES net_node (uuid),
    CONSTRAINT fk_unit_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    CONSTRAINT fk_unit_password_hash_algorithm
    FOREIGN KEY (password_hash_algorithm_id)
    REFERENCES password_hash_algorithm (id)
);
