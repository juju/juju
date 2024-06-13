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

CREATE TABLE charm (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT,
    description TEXT,
    summary TEXT,
    subordinate BOOLEAN DEFAULT FALSE,
    min_juju_version TEXT,
    run_as_id INT,
    -- Assumes is a blob of YAML that will be parsed by the charm to compute
    -- the result of the SAT expression.
    -- As the expression tree is generic, you can't use RI or index into the
    -- blob without constraining the expression to a specific set of rules.
    assumes TEXT,
    lxd_profile TEXT,
    CONSTRAINT fk_charm_run_as_kind_charm
    FOREIGN KEY (run_as_id)
    REFERENCES charm_run_as_kind (id)
);

CREATE TABLE charm_channel (
    charm_uuid TEXT NOT NULL,
    track TEXT,
    risk TEXT NOT NULL,
    branch TEXT,
    CONSTRAINT fk_charm_channel_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    PRIMARY KEY (charm_uuid)
);

CREATE UNIQUE INDEX idx_charm_name
ON charm (name);

-- The charm_state table exists to store the availability of a charm. The
-- fact that the charm is in the database indicates that it's a placeholder.
-- Updating the available flag to true indicates that the charm is now
-- available for deployment.
-- This is exists as a separate table as the charm table models the charm
-- metadata and the goal state of the charm. The charm_state table models the
-- internal state of the charm.
CREATE TABLE charm_state (
    charm_uuid TEXT NOT NULL,
    -- Available is a flag that indicates whether the charm is available for
    -- deployment.
    available BOOLEAN,
    CONSTRAINT fk_charm_state_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid)
);

CREATE TABLE charm_source (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_charm_source_name
ON charm_source (name);

INSERT INTO charm_source VALUES
(0, 'local'),
(1, 'ch');

CREATE TABLE charm_origin (
    charm_uuid TEXT NOT NULL,
    source_id INT,
    id TEXT,
    revision INT,
    CONSTRAINT fk_charm_source_source
    FOREIGN KEY (source_id)
    REFERENCES charm_source (id),
    CONSTRAINT fk_charm_origin_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid)
);

CREATE TABLE charm_platform (
    charm_uuid TEXT NOT NULL,
    os_id TEXT,
    channel TEXT,
    architecture_id TEXT,
    CONSTRAINT fk_charm_platform_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    CONSTRAINT fk_charm_platform_os
    FOREIGN KEY (os_id)
    REFERENCES os (id),
    CONSTRAINT fk_charm_platform_architecture
    FOREIGN KEY (architecture_id)
    REFERENCES architecture (id)
);

CREATE TABLE hash_kind (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_hash_kind_name
ON hash_kind (name);

INSERT INTO hash_kind VALUES
(0, 'sha256');

CREATE TABLE charm_hash (
    charm_uuid TEXT NOT NULL,
    hash_kind_id TEXT NOT NULL,
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
    charm_uuid TEXT NOT NULL,
    kind_id TEXT NOT NULL,
    name TEXT,
    role_id TEXT,
    interface TEXT,
    optional BOOLEAN,
    capacity INT,
    scope_id TEXT,
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
    REFERENCES charm_relation_scope (id),
    PRIMARY KEY (charm_uuid, kind_id, name)
);

CREATE INDEX idx_charm_relation_charm
ON charm_relation (charm_uuid);

CREATE TABLE charm_extra_binding (
    charm_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    CONSTRAINT fk_charm_extra_binding_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    PRIMARY KEY (charm_uuid, name)
);

CREATE INDEX idx_charm_extra_binding_charm
ON charm_extra_binding (charm_uuid);

-- charm_category is a limited set of categories that a charm can be tagged
-- for the charmhub store. This is free form and driven by the charmhub store.
-- We're not enforcing any constraints on the values as that can be changed
-- by 3rd party stores.
CREATE TABLE charm_category (
    charm_uuid TEXT NOT NULL,
    value TEXT NOT NULL,
    CONSTRAINT fk_charm_category_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    PRIMARY KEY (charm_uuid, value)
);

CREATE INDEX idx_charm_category_charm
ON charm_category (charm_uuid);

-- charm_tag is a free form tag that can be applied to a charm.
CREATE TABLE charm_tag (
    charm_uuid TEXT NOT NULL,
    value TEXT NOT NULL,
    CONSTRAINT fk_charm_tag_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    PRIMARY KEY (charm_uuid, value)
);

CREATE INDEX idx_charm_tag_charm
ON charm_tag (charm_uuid);

CREATE TABLE charm_storage_kind (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

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
    CONSTRAINT fk_storage_instance_kind
    FOREIGN KEY (storage_kind_id)
    REFERENCES storage_kind (id),
    CONSTRAINT fk_charm_storage_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    PRIMARY KEY (charm_uuid, name)
);

CREATE INDEX idx_charm_storage_charm
ON charm_storage (charm_uuid);

CREATE TABLE charm_storage_property (
    charm_uuid TEXT NOT NULL,
    charm_storage_name TEXT NOT NULL,
    value TEXT,
    CONSTRAINT fk_charm_storage_property_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    CONSTRAINT fk_charm_storage_property_charm_storage
    FOREIGN KEY (charm_storage_name)
    REFERENCES charm_storage (name),
    PRIMARY KEY (charm_uuid, charm_storage_name, value)
);

CREATE INDEX idx_charm_storage_property_charm
ON charm_storage_property (charm_uuid);

CREATE TABLE charm_device (
    charm_uuid TEXT NOT NULL,
    name TEXT,
    description TEXT,
    device_type TEXT,
    count_min INT NOT NULL,
    count_max INT NOT NULL,
    CONSTRAINT fk_charm_device_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    PRIMARY KEY (charm_uuid, name)
);

CREATE INDEX idx_charm_device_charm
ON charm_device (charm_uuid);

CREATE TABLE charm_payload (
    charm_uuid TEXT NOT NULL,
    name TEXT,
    type TEXT,
    CONSTRAINT fk_charm_payload_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    PRIMARY KEY (charm_uuid, name)
);

CREATE INDEX idx_charm_payload_charm
ON charm_payload (charm_uuid);

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
    name TEXT,
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

CREATE INDEX idx_charm_resource_charm
ON charm_resource (charm_uuid);

CREATE TABLE charm_term (
    charm_uuid TEXT NOT NULL,
    value TEXT NOT NULL,
    CONSTRAINT fk_charm_term_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    PRIMARY KEY (charm_uuid, value)
);

CREATE INDEX idx_charm_term_charm
ON charm_term (charm_uuid);

CREATE TABLE charm_container (
    charm_uuid TEXT NOT NULL,
    name TEXT,
    resource TEXT,
    -- Enforce the optional uid and gid to -1 if not set, otherwise the it might
    -- become 0, which happens to be root.
    uid INT DEFAULT -1,
    gid INT DEFAULT -1,
    CONSTRAINT fk_charm_container_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    PRIMARY KEY (charm_uuid, resource)
);

CREATE INDEX idx_charm_container_charm
ON charm_container (charm_uuid);

CREATE TABLE charm_container_mount (
    charm_uuid TEXT NOT NULL,
    charm_container_name TEXT,
    name TEXT,
    storage TEXT,
    location TEXT,
    CONSTRAINT fk_charm_container_mount_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    CONSTRAINT fk_charm_container_mount_charm_container
    FOREIGN KEY (charm_container_name)
    REFERENCES charm_container (name),
    PRIMARY KEY (charm_uuid, name)
);

CREATE INDEX idx_charm_container_mount_charm
ON charm_container_mount (charm_uuid);

-- Create a charm url view for backwards compatibility.
CREATE VIEW v_charm_url AS
SELECT
    c.uuid,
    cs.name || ':' || c.name || '-' || COALESCE(co.revision, 0) AS url
FROM charm AS c
INNER JOIN charm_origin AS co ON c.uuid = co.charm_uuid
LEFT JOIN charm_source AS cs ON co.source_id = cs.id;

CREATE TABLE charm_action (
    charm_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    parallel BOOLEAN,
    execution_group TEXT,
    params TEXT,
    CONSTRAINT fk_charm_actions_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    PRIMARY KEY (charm_uuid, name)
);

CREATE TABLE charm_manifest_base (
    charm_uuid TEXT NOT NULL,
    os_id TEXT NOT NULL,
    track TEXT,
    risk TEXT NOT NULL,
    branch TEXT,
    architecture_id TEXT NOT NULL,
    CONSTRAINT fk_charm_manifest_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    CONSTRAINT fk_charm_manifest_base_os
    FOREIGN KEY (os_id)
    REFERENCES os (id),
    CONSTRAINT fk_charm_manifest_base_architecture
    FOREIGN KEY (architecture_id)
    REFERENCES architecture (id),
    PRIMARY KEY (charm_uuid, os_id, track, risk, branch, architecture_id)
);

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
    name TEXT NOT NULL,
    type_id TEXT,
    default_value TEXT,
    description TEXT,
    CONSTRAINT fk_charm_config_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    CONSTRAINT fk_charm_config_charm_config_type
    FOREIGN KEY (type_id)
    REFERENCES charm_config_type (id),
    PRIMARY KEY (charm_uuid, name)
);
