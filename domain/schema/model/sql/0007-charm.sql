CREATE TABLE charm_run_as_kind (
    id       INT PRIMARY KEY,
    name     TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_charm_run_as_kind_name
    ON charm_run_as_kind (name);

INSERT INTO charm_run_as_kind VALUES
    (0, 'default'), 
    (1, 'root'),
    (2, 'sudoer'),
    (3, 'non-root');

CREATE TABLE charm (
    uuid                TEXT PRIMARY KEY,
    name                TEXT,
    description         TEXT,
    summary             TEXT,
    min_juju_version    TEXT,
    run_as_id           INT,
    -- Assumes is a blob of YAML that will be parsed by the charm to compute
    -- the result of the SAT expression.
    -- As the expression tree is generic, you can't use RI or index into the
    -- blob without constraining the expression to a specific set of rules.
    assumes_blob        TEXT,
    -- Available is a flag that indicates whether the charm is available for
    -- deployment.
    available           BOOLEAN,
    CONSTRAINT          fk_charm_run_as_kind_charm
        FOREIGN KEY     (run_as_id)
        REFERENCES      charm_run_as_kind(id)
);

CREATE UNIQUE INDEX idx_charm_name
    ON charm (name);

CREATE TABLE charm_source (
    id       INT PRIMARY KEY,
    name     TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_charm_source_name
    ON charm_source (name);

INSERT INTO charm_source VALUES
    (0, 'local'),
    (1, 'ch');

CREATE TABLE charm_origin (
    charm_uuid          TEXT NOT NULL,
    source_id           INT,
    id                  string,
    hash                string,
    revision            INT,
    CONSTRAINT          fk_charm_source_source
        FOREIGN KEY     (source_id)
        REFERENCES      source(id),
    CONSTRAINT          fk_charm_origin_charm
        FOREIGN KEY     (charm_uuid)
        REFERENCES      charm(uuid)
);

CREATE TABLE charm_channel (
    charm_uuid          TEXT NOT NULL,
    track               TEXT,
    risk                TEXT,
    branch              TEXT,
    CONSTRAINT          fk_charm_channel_charm
        FOREIGN KEY     (charm_uuid)
        REFERENCES      charm(uuid)
);

CREATE TABLE architecture (
    id       INT PRIMARY KEY,
    name     TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_architecture_name
    ON architecture (name);

INSERT INTO architecture VALUES
    (0, 'amd64'),
    (1, 'arm64'),
    (2, 'ppc64el'),
    (3, 's390x'),
    (4, 'riscv64');

CREATE TABLE charm_platform (
    charm_uuid          TEXT NOT NULL,
    os                  TEXT,
    channel             TEXT,
    architecture_id     TEXT,
    CONSTRAINT          fk_charm_channel_charm
        FOREIGN KEY     (charm_uuid)
        REFERENCES      charm(uuid),
    CONSTRAINT          fk_charm_origin_architecture
        FOREIGN KEY     (architecture_id)
        REFERENCES      architecture(id)
);

CREATE TABLE hash_kind (
    id       INT PRIMARY KEY,
    name     TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_hash_kind_name
    ON hash_kind (name);

INSERT INTO hash_kind VALUES
    (0, 'sha256');

CREATE TABLE charm_hash (
    charm_uuid          TEXT NOT NULL,
    hash_kind_id        TEXT NOT NULL,
    hash                TEXT NOT NULL,
    CONSTRAINT          fk_charm_hash_charm
        FOREIGN KEY     (charm_uuid)
        REFERENCES      charm(uuid),
    CONSTRAINT          fk_charm_hash_kind
        FOREIGN KEY     (hash_kind_id)
        REFERENCES      hash_kind(id)
);

CREATE TABLE charm_relation_kind (
    id       INT PRIMARY KEY,
    name     TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_charm_relation_kind_name
    ON charm_relation_kind (name);

INSERT INTO charm_relation_kind VALUES
    (0, 'provides'), 
    (1, 'requires'),
    (2, 'peers');

CREATE TABLE charm_relation_role (
    id       INT PRIMARY KEY,
    name     TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_charm_relation_role_name
    ON charm_relation_role (name);

INSERT INTO charm_relation_role VALUES
    (0, 'provider'),
    (1, 'requirer'),
    (2, 'peer');

CREATE TABLE charm_relation_scope (
    id       INT PRIMARY KEY,
    name     TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_charm_relation_scope_name
    ON charm_relation_scope (name);

INSERT INTO charm_relation_scope VALUES
    (0, 'global'),
    (1, 'container');

CREATE TABLE charm_relation (
    charm_uuid          TEXT NOT NULL,
    kind_id             TEXT NOT NULL,
    name                TEXT,
    role_id             TEXT,
    interface           TEXT,
    optional            BOOLEAN,
    "limit"             INT,
    scope_id            TEXT,
    CONSTRAINT          fk_charm_relation_charm
        FOREIGN KEY     (charm_uuid)
        REFERENCES      charm(uuid),
    CONSTRAINT          fk_charm_relation_kind
        FOREIGN KEY     (kind_id)
        REFERENCES      charm_relation_kind(id),
    CONSTRAINT          fk_charm_relation_role
        FOREIGN KEY     (role_id)
        REFERENCES      charm_relation_role(id),
    CONSTRAINT          fk_charm_relation_scope
        FOREIGN KEY     (scope_id)
        REFERENCES      charm_relation_scope(id),
    PRIMARY KEY (charm_uuid, kind_id, name)
);

CREATE INDEX idx_charm_relation_charm
ON charm_relation (charm_uuid);

CREATE TABLE charm_extra_binding (
    charm_uuid          TEXT NOT NULL,
    name                TEXT NOT NULL,
    CONSTRAINT          fk_charm_extra_binding_charm
        FOREIGN KEY     (charm_uuid)
        REFERENCES      charm(uuid),
    PRIMARY KEY (charm_uuid, name)
);

CREATE INDEX idx_charm_extra_binding_charm
ON charm_extra_binding (charm_uuid);

CREATE TABLE charm_category (
    charm_uuid          TEXT NOT NULL,
    value               TEXT NOT NULL,
    CONSTRAINT          fk_charm_category_charm
        FOREIGN KEY     (charm_uuid)
        REFERENCES      charm(uuid),
    PRIMARY KEY (charm_uuid, value)
);

CREATE INDEX idx_charm_category_charm
ON charm_category (charm_uuid);

CREATE TABLE charm_tag (
    charm_uuid          TEXT NOT NULL,
    value               TEXT NOT NULL,
    CONSTRAINT          fk_charm_tag_charm
        FOREIGN KEY     (charm_uuid)
        REFERENCES      charm(uuid),
    PRIMARY KEY (charm_uuid, value)
);

CREATE INDEX idx_charm_tag_charm
ON charm_tag (charm_uuid);

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
    charm_uuid          TEXT NOT NULL,
    charm_storage_name  TEXT NOT NULL,
    key                 TEXT,
    value               TEXT,
    CONSTRAINT          fk_charm_storage_property_charm
        FOREIGN KEY     (charm_uuid)
        REFERENCES      charm(uuid),
    CONSTRAINT          fk_charm_storage_property_charm_storage
        FOREIGN KEY     (charm_storage_name)
        REFERENCES      charm_storage(name),
    PRIMARY KEY (charm_uuid, charm_storage_name, key)
);

CREATE INDEX idx_charm_storage_property_charm
ON charm_storage_property (charm_uuid);

CREATE TABLE charm_device (
    charm_uuid          TEXT NOT NULL,
    name                TEXT,
    description         TEXT,
    device_type         TEXT,
    count_min           INT NOT NULL,
    count_max           INT NOT NULL,
    CONSTRAINT          fk_charm_device_charm
        FOREIGN KEY     (charm_uuid)
        REFERENCES      charm(uuid),
    PRIMARY KEY (charm_uuid, name)
);

CREATE INDEX idx_charm_device_charm
ON charm_device (charm_uuid);

CREATE TABLE charm_payload (
    charm_uuid          TEXT NOT NULL,
    name                TEXT,
    type                TEXT,
    CONSTRAINT          fk_charm_payload_charm
        FOREIGN KEY     (charm_uuid)
        REFERENCES      charm(uuid),
    PRIMARY KEY (charm_uuid, name)
);

CREATE INDEX idx_charm_payload_charm
ON charm_payload (charm_uuid);

CREATE TABLE charm_resource_kind (
    id       INT PRIMARY KEY,
    name     TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_charm_resource_kind_name
    ON charm_resource_kind (name);

INSERT INTO charm_resource_kind VALUES
    (0, 'file'),
    (1, 'oci-image');

CREATE TABLE charm_resource (
    charm_uuid          TEXT NOT NULL,
    name                TEXT,
    kind_id             INT NOT NULL,
    path                TEXT,
    description         TEXT,
    CONSTRAINT          fk_charm_resource_charm
        FOREIGN KEY     (charm_uuid)
        REFERENCES      charm(uuid),
    CONSTRAINT          fk_charm_resource_kind
        FOREIGN KEY     (kind_id)
        REFERENCES      charm_resource_kind(id),
    PRIMARY KEY (charm_uuid, name)
);

CREATE INDEX idx_charm_resource_charm
ON charm_resource (charm_uuid);

CREATE TABLE charm_term (
    charm_uuid          TEXT NOT NULL,
    value               TEXT NOT NULL,
    CONSTRAINT          fk_charm_term_charm
        FOREIGN KEY     (charm_uuid)
        REFERENCES      charm(uuid),
    PRIMARY KEY (charm_uuid, value)
);

CREATE INDEX idx_charm_term_charm
ON charm_term (charm_uuid);

CREATE TABLE charm_container (
    charm_uuid          TEXT NOT NULL,
    name                TEXT,
    resource            TEXT,
    uid                 INT,
    gid                 INT,
    CONSTRAINT          fk_charm_container_charm
        FOREIGN KEY     (charm_uuid)
        REFERENCES      charm(uuid),
    PRIMARY KEY (charm_uuid, resource)
);

CREATE INDEX idx_charm_container_charm
ON charm_container (charm_uuid);

CREATE TABLE charm_container_mount (
    charm_uuid            TEXT NOT NULL,
    charm_container_name  TEXT,
    resource              TEXT,
    storage               TEXT,
    location              TEXT,
    CONSTRAINT            fk_charm_container_mount_charm
        FOREIGN KEY       (charm_uuid)
        REFERENCES        charm(uuid),
    CONSTRAINT            fk_charm_container_mount_charm_container
        FOREIGN KEY       (charm_container_name)
        REFERENCES        charm_container(name),
    PRIMARY KEY (charm_uuid, resource)
);

CREATE INDEX idx_charm_container_mount_charm
ON charm_container_mount (charm_uuid);
