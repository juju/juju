CREATE TABLE unit_resolve_kind (
    id TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_unit_resolve_kind
ON unit_resolve_kind (name);

INSERT INTO unit_resolve_kind VALUES
(0, 'none'),
(1, 'retry-hooks'),
(2, 'no-hooks');

CREATE TABLE unit (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL,
    life_id INT NOT NULL,
    application_uuid TEXT NOT NULL,
    net_node_uuid TEXT NOT NULL,
    -- charm_uuid should not be nullable, but we need to allow it for now
    -- whilst we're wiring up the model.
    charm_uuid TEXT,
    resolve_kind_id TEXT NOT NULL,
    password_hash_algorithm_id TEXT,
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
    CONSTRAINT fk_unit_resolve_kind
    FOREIGN KEY (resolve_kind_id)
    REFERENCES unit_resolve_kind (id),
    CONSTRAINT fk_unit_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    CONSTRAINT fk_unit_password_hash_algorithm
    FOREIGN KEY (password_hash_algorithm_id)
    REFERENCES password_hash_algorithm (id)
);

CREATE UNIQUE INDEX idx_unit_name
ON unit (name);

CREATE INDEX idx_unit_application
ON unit (application_uuid);

CREATE INDEX idx_unit_net_node
ON unit (net_node_uuid);

CREATE TABLE unit_platform (
    unit_uuid TEXT NOT NULL PRIMARY KEY,
    os_id TEXT,
    channel TEXT,
    architecture_id TEXT,
    CONSTRAINT fk_unit_platform_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    CONSTRAINT fk_unit_platform_os
    FOREIGN KEY (os_id)
    REFERENCES os (id),
    CONSTRAINT fk_unit_platform_architecture
    FOREIGN KEY (architecture_id)
    REFERENCES architecture (id)
);

CREATE TABLE unit_tool (
    unit_uuid TEXT NOT NULL,
    url TEXT NOT NULL,
    version_major INT NOT NULL,
    version_minor INT NOT NULL,
    version_tag TEXT,
    version_patch INT NOT NULL,
    version_build INT,
    CONSTRAINT fk_unit_tool_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    CONSTRAINT fk_object_store_metadata_path_unit
    FOREIGN KEY (url)
    REFERENCES object_store_metadata_path (path),
    PRIMARY KEY (unit_uuid, url)
);

CREATE TABLE unit_state (
    unit_uuid TEXT NOT NULL PRIMARY KEY,
    uniter_state TEXT,
    storage_state TEXT,
    secret_state TEXT,
    CONSTRAINT fk_unit_state_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid)
);

-- Local charm state stored upon hook commit with uniter state.
CREATE TABLE unit_state_charm (
    unit_uuid TEXT,
    "key" TEXT,
    value TEXT NOT NULL,
    PRIMARY KEY (unit_uuid, "key"),
    CONSTRAINT fk_unit_state_charm_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid)
);

-- Local relation state stored upon hook commit with uniter state.
CREATE TABLE unit_state_relation (
    unit_uuid TEXT,
    "key" TEXT,
    value TEXT NOT NULL,
    PRIMARY KEY (unit_uuid, "key"),
    CONSTRAINT fk_unit_state_relation_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid)
);
