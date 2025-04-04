CREATE TABLE charm_run_as_kind (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_charm_run_as_kind_name
ON charm_run_as_kind (name);

INSERT INTO charm_run_as_kind VALUES
(0, 'default'),
(1, 'root'),
(2, 'sudoer'),
(3, 'non-root');

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

    source_id INT NOT NULL DEFAULT 1,
    revision INT NOT NULL DEFAULT -1,

    -- architecture_id may be null for local charms.
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

    -- Ensure we have an architecture if the source is charmhub.
    CONSTRAINT chk_charm_architecture
    CHECK (source_id = 0 OR source_id = 1 AND architecture_id >= 0),

    -- Ensure we don't have an empty reference
    CONSTRAINT chk_charm_reference_name
    CHECK (reference_name <> '')
);

-- This ensures that the reference name and revision are unique. This is to
-- ensure that we don't have two charms with the same reference name and
-- revision. If this happens, we can just link the application to the existing
-- charm.
CREATE UNIQUE INDEX idx_charm_reference_name_revision
ON charm (source_id, reference_name, revision);

CREATE TABLE charm_provenance (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_charm_provenance_name
ON charm_provenance (name);

-- The provenance of the charm. This is used to determine where the charm
-- came from, which can then determine if the download information is still
-- relevant.
INSERT INTO charm_provenance VALUES
(0, 'download'),
(1, 'migration'),
(2, 'upload'),
(3, 'bootstrap');

CREATE TABLE charm_download_info (
    charm_uuid TEXT NOT NULL PRIMARY KEY,

    -- The provenance_id is the origin from which the download information
    -- was obtained. Ideally, we would have used origin, but that's already
    -- taken and I don't want to confuse the two.
    provenance_id INT NOT NULL,

    -- charmhub_identifier is the identifier that charmhub uses to identify the
    -- charm. This is used to refresh the charm from charmhub. The
    -- reference_name can change but the charmhub_identifier will not.
    charmhub_identifier TEXT NOT NULL,

    download_url TEXT NOT NULL,
    download_size INT NOT NULL,

    CONSTRAINT fk_charm_download_info_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid)
);

CREATE VIEW v_application_charm_download_info AS
SELECT
    a.uuid AS application_uuid,
    c.uuid AS charm_uuid,
    c.reference_name AS name,
    c.available,
    cs.id AS source_id,
    cp.name AS provenance,
    cdi.charmhub_identifier,
    cdi.download_url,
    cdi.download_size,
    ch.hash
FROM application AS a
LEFT JOIN charm AS c ON a.charm_uuid = c.uuid
LEFT JOIN charm_download_info AS cdi ON c.uuid = cdi.charm_uuid
LEFT JOIN charm_provenance AS cp ON cdi.provenance_id = cp.id
LEFT JOIN charm_source AS cs ON c.source_id = cs.id
LEFT JOIN charm_hash AS ch ON c.uuid = ch.charm_uuid;

CREATE TABLE charm_metadata (
    charm_uuid TEXT NOT NULL PRIMARY KEY,
    -- name represents the original name of the charm. This is what is stored
    -- in the charm metadata.yaml file.
    name TEXT NOT NULL,
    description TEXT,
    summary TEXT,
    subordinate BOOLEAN DEFAULT FALSE,
    min_juju_version TEXT,
    run_as_id INT DEFAULT 0,
    -- Assumes is a blob of YAML that will be parsed by the charm to compute
    -- the result of the SAT expression.
    -- As the expression tree is generic, you can't use RI or index into the
    -- blob without constraining the expression to a specific set of rules.
    assumes TEXT,
    CONSTRAINT fk_charm_run_as_kind_charm
    FOREIGN KEY (run_as_id)
    REFERENCES charm_run_as_kind (id),
    CONSTRAINT fk_charm_metadata_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid)
);


CREATE INDEX idx_charm_metadata_subordinate
ON charm_metadata (subordinate);

CREATE VIEW v_charm_metadata AS
SELECT
    c.uuid,
    cm.name,
    cm.description,
    cm.summary,
    cm.subordinate,
    cm.min_juju_version,
    crak.name AS run_as,
    cm.assumes,
    c.available
FROM charm AS c
LEFT JOIN charm_metadata AS cm ON c.uuid = cm.charm_uuid
LEFT JOIN charm_run_as_kind AS crak ON cm.run_as_id = crak.id;

CREATE TABLE charm_source (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_charm_source_name
ON charm_source (name);

INSERT INTO charm_source VALUES
(0, 'local'),
(1, 'charmhub');

CREATE VIEW v_charm_annotation_index AS
SELECT
    c.uuid,
    c.revision,
    cm.name
FROM charm AS c
LEFT JOIN charm_metadata AS cm ON c.uuid = cm.charm_uuid;

CREATE TABLE hash_kind (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_hash_kind_name
ON hash_kind (name);

-- We only support sha256 hashes for now.
INSERT INTO hash_kind VALUES
(0, 'sha256');

CREATE TABLE charm_hash (
    charm_uuid TEXT NOT NULL,
    hash_kind_id INT NOT NULL DEFAULT 0,
    hash TEXT NOT NULL,
    CONSTRAINT fk_charm_hash_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    CONSTRAINT fk_charm_hash_kind
    FOREIGN KEY (hash_kind_id)
    REFERENCES hash_kind (id)
);

CREATE TABLE charm_relation_kind (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_charm_relation_kind_name
ON charm_relation_kind (name);

INSERT INTO charm_relation_kind VALUES
(0, 'provides'),
(1, 'requires'),
(2, 'peers');

CREATE TABLE charm_relation_role (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_charm_relation_role_name
ON charm_relation_role (name);

INSERT INTO charm_relation_role VALUES
(0, 'provider'),
(1, 'requirer'),
(2, 'peer');

CREATE TABLE charm_relation_scope (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_charm_relation_scope_name
ON charm_relation_scope (name);

INSERT INTO charm_relation_scope VALUES
(0, 'global'),
(1, 'container');

CREATE TABLE charm_relation (
    uuid TEXT NOT NULL PRIMARY KEY,
    charm_uuid TEXT NOT NULL,
    kind_id TEXT NOT NULL,
    name TEXT NOT NULL,
    role_id INT NOT NULL,
    scope_id INT NOT NULL,
    interface TEXT,
    optional BOOLEAN,
    capacity INT,
    CONSTRAINT fk_charm_relation_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    CONSTRAINT fk_charm_relation_kind
    FOREIGN KEY (kind_id)
    REFERENCES charm_relation_kind (id),
    CONSTRAINT fk_charm_relation_role
    FOREIGN KEY (role_id)
    REFERENCES charm_relation_role (id),
    CONSTRAINT fk_charm_relation_scope
    FOREIGN KEY (scope_id)
    REFERENCES charm_relation_scope (id)
);

CREATE UNIQUE INDEX idx_charm_relation_charm_key
ON charm_relation (charm_uuid, name);

CREATE VIEW v_charm_relation AS
SELECT
    cr.charm_uuid,
    crk.name AS kind,
    cr.name,
    crr.name AS role,
    cr.interface,
    cr.optional,
    cr.capacity,
    crs.name AS scope
FROM charm_relation AS cr
JOIN charm_relation_kind AS crk ON cr.kind_id = crk.id
JOIN charm_relation_role AS crr ON cr.role_id = crr.id
JOIN charm_relation_scope AS crs ON cr.scope_id = crs.id;

CREATE INDEX idx_charm_relation_charm
ON charm_relation (charm_uuid);

CREATE TABLE charm_extra_binding (
    uuid TEXT NOT NULL PRIMARY KEY,
    charm_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    CONSTRAINT fk_charm_extra_binding_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid)
);

CREATE INDEX idx_charm_extra_binding_charm
ON charm_extra_binding (charm_uuid, name);

-- charm_category is a limited set of categories that a charm can be tagged
-- for the charmhub store. This is free form and driven by the charmhub store.
-- We're not enforcing any constraints on the values as that can be changed
-- by 3rd party stores.
CREATE TABLE charm_category (
    charm_uuid TEXT NOT NULL,
    array_index INT NOT NULL,
    value TEXT NOT NULL,
    CONSTRAINT fk_charm_category_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    PRIMARY KEY (charm_uuid, array_index, value)
);

CREATE INDEX idx_charm_category_charm
ON charm_category (charm_uuid);

-- charm_tag is a free form tag that can be applied to a charm.
CREATE TABLE charm_tag (
    charm_uuid TEXT NOT NULL,
    array_index INT NOT NULL,
    value TEXT NOT NULL,
    CONSTRAINT fk_charm_tag_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    PRIMARY KEY (charm_uuid, array_index, value)
);

CREATE INDEX idx_charm_tag_charm
ON charm_tag (charm_uuid);

CREATE TABLE charm_storage_kind (
    id INT PRIMARY KEY,
    kind TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_charm_storage_kind
ON charm_storage_kind (kind);

INSERT INTO charm_storage_kind VALUES
(0, 'block'),
(1, 'filesystem');

CREATE TABLE charm_storage (
    charm_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    storage_kind_id INT NOT NULL,
    shared BOOLEAN,
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

CREATE VIEW v_charm_storage AS
SELECT
    cs.charm_uuid,
    cs.name,
    cs.description,
    csk.kind,
    cs.shared,
    cs.read_only,
    cs.count_min,
    cs.count_max,
    cs.minimum_size_mib,
    cs.location,
    csp.array_index AS property_index,
    csp.value AS property
FROM charm_storage AS cs
LEFT JOIN charm_storage_kind AS csk ON cs.storage_kind_id = csk.id
LEFT JOIN charm_storage_property AS csp ON cs.charm_uuid = csp.charm_uuid AND cs.name = csp.charm_storage_name;

CREATE INDEX idx_charm_storage_charm
ON charm_storage (charm_uuid);

CREATE TABLE charm_storage_property (
    charm_uuid TEXT NOT NULL,
    charm_storage_name TEXT NOT NULL,
    array_index INT NOT NULL,
    value TEXT NOT NULL,
    CONSTRAINT fk_charm_storage_property_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    CONSTRAINT fk_charm_storage_property_charm_storage
    FOREIGN KEY (charm_uuid, charm_storage_name)
    REFERENCES charm_storage (charm_uuid, name),
    PRIMARY KEY (charm_uuid, charm_storage_name, array_index, value)
);

CREATE INDEX idx_charm_storage_property_charm
ON charm_storage_property (charm_uuid);

CREATE TABLE charm_device (
    charm_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    name TEXT,
    description TEXT,
    device_type TEXT,
    count_min INT NOT NULL,
    count_max INT NOT NULL,
    CONSTRAINT fk_charm_device_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    PRIMARY KEY (charm_uuid, "key")
);

CREATE INDEX idx_charm_device_charm
ON charm_device (charm_uuid);

CREATE TABLE charm_resource_kind (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_charm_resource_kind_name
ON charm_resource_kind (name);

INSERT INTO charm_resource_kind VALUES
(0, 'file'),
(1, 'oci-image');

CREATE TABLE charm_resource (
    charm_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    kind_id INT NOT NULL,
    path TEXT,
    description TEXT,
    CONSTRAINT fk_charm_resource_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    CONSTRAINT fk_charm_resource_charm_resource_kind
    FOREIGN KEY (kind_id)
    REFERENCES charm_resource_kind (id),
    PRIMARY KEY (charm_uuid, name)
);

CREATE VIEW v_charm_resource AS
SELECT
    cr.charm_uuid,
    cr.name,
    crk.name AS kind,
    cr.path,
    cr.description
FROM charm_resource AS cr
LEFT JOIN charm_resource_kind AS crk ON cr.kind_id = crk.id;

CREATE INDEX idx_charm_resource_charm
ON charm_resource (charm_uuid);

CREATE TABLE charm_term (
    charm_uuid TEXT NOT NULL,
    array_index INT NOT NULL,
    value TEXT NOT NULL,
    CONSTRAINT fk_charm_term_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    PRIMARY KEY (charm_uuid, array_index, value)
);

CREATE INDEX idx_charm_term_charm
ON charm_term (charm_uuid);

CREATE TABLE charm_container (
    charm_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    resource TEXT,
    -- Enforce the optional uid and gid to -1 if not set, otherwise the it might
    -- become 0, which happens to be root.
    uid INT DEFAULT -1,
    gid INT DEFAULT -1,
    CONSTRAINT fk_charm_container_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    PRIMARY KEY (charm_uuid, "key")
);

CREATE VIEW v_charm_container AS
SELECT
    cc.charm_uuid,
    cc."key",
    cc.resource,
    cc.uid,
    cc.gid,
    ccm.array_index,
    ccm.storage,
    ccm.location
FROM charm_container AS cc
LEFT JOIN charm_container_mount AS ccm ON cc.charm_uuid = ccm.charm_uuid AND cc."key" = ccm.charm_container_key;

CREATE INDEX idx_charm_container_charm
ON charm_container (charm_uuid);

CREATE TABLE charm_container_mount (
    array_index INT NOT NULL,
    charm_uuid TEXT NOT NULL,
    charm_container_key TEXT,
    storage TEXT,
    location TEXT,
    CONSTRAINT fk_charm_container_mount_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    CONSTRAINT fk_charm_container_mount_charm_container
    FOREIGN KEY (charm_uuid, charm_container_key)
    REFERENCES charm_container (charm_uuid, "key"),
    PRIMARY KEY (charm_uuid, charm_container_key, array_index)
);

CREATE INDEX idx_charm_container_mount_charm
ON charm_container_mount (charm_uuid);

CREATE TABLE charm_action (
    charm_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    description TEXT,
    parallel BOOLEAN,
    execution_group TEXT,
    params TEXT,
    CONSTRAINT fk_charm_actions_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    PRIMARY KEY (charm_uuid, "key")
);

CREATE TABLE charm_manifest_base (
    charm_uuid TEXT NOT NULL,
    array_index INT NOT NULL,
    nested_array_index INT NOT NULL,
    os_id INT DEFAULT 0,
    track TEXT,
    risk TEXT NOT NULL,
    branch TEXT,
    architecture_id INT DEFAULT 0,
    CONSTRAINT fk_charm_manifest_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    CONSTRAINT fk_charm_manifest_base_os
    FOREIGN KEY (os_id)
    REFERENCES os (id),
    CONSTRAINT fk_charm_manifest_base_architecture
    FOREIGN KEY (architecture_id)
    REFERENCES architecture (id),
    PRIMARY KEY (charm_uuid, array_index, nested_array_index, os_id, track, risk, branch, architecture_id)
);

CREATE VIEW v_charm_manifest AS
SELECT
    cmb.charm_uuid,
    cmb.array_index,
    cmb.nested_array_index,
    cmb.track,
    cmb.risk,
    cmb.branch,
    os.name AS os,
    architecture.name AS architecture
FROM charm_manifest_base AS cmb
LEFT JOIN os ON cmb.os_id = os.id
LEFT JOIN architecture ON cmb.architecture_id = architecture.id;

CREATE TABLE charm_config_type (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_charm_config_type_name
ON charm_config_type (name);

INSERT INTO charm_config_type VALUES
(0, 'string'),
(1, 'int'),
(2, 'float'),
(3, 'boolean'),
(4, 'secret');

CREATE TABLE charm_config (
    charm_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    type_id TEXT,
    default_value TEXT,
    description TEXT,
    CONSTRAINT fk_charm_config_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    CONSTRAINT fk_charm_config_charm_config_type
    FOREIGN KEY (type_id)
    REFERENCES charm_config_type (id),
    PRIMARY KEY (charm_uuid, "key")
);

CREATE VIEW v_charm_config AS
SELECT
    cc.charm_uuid,
    cc."key",
    cct.name AS type,
    cc.default_value,
    cc.description
FROM charm_config AS cc
LEFT JOIN charm_config_type AS cct ON cc.type_id = cct.id;

CREATE VIEW v_charm_locator AS
SELECT
    c.uuid,
    c.reference_name,
    c.revision,
    c.source_id,
    c.architecture_id,
    cm.name
FROM charm AS c
JOIN charm_metadata AS cm ON c.uuid = cm.charm_uuid;
