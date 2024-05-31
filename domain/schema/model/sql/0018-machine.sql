CREATE TABLE machine (
    uuid              TEXT PRIMARY KEY,
    machine_id        TEXT NOT NULL,
    net_node_uuid     TEXT NOT NULL,
    life_id           INT NOT NULL,
    base              TEXT,
    nonce             TEXT,
    container_type_id INT,
    password_hash     TEXT,
    clean             BOOLEAN,
    force_destroyed   BOOLEAN,
    placement         TEXT,
    agent_started_at  DATETIME,
    hostname          TEXT,
    CONSTRAINT        fk_machine_net_node
        FOREIGN KEY   (net_node_uuid)
        REFERENCES    net_node(uuid),
    CONSTRAINT        fk_machine_life
        FOREIGN KEY   (life_id)
        REFERENCES    life(id),
    CONSTRAINT        fk_machine_container_type
        FOREIGN KEY   (container_type_id)
        REFERENCES    container_type(id)
);

CREATE UNIQUE INDEX idx_machine_id
ON machine (machine_id);

CREATE UNIQUE INDEX idx_machine_net_node
ON machine (net_node_uuid);

CREATE TABLE container_type (
    id    INT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT INTO container_type VALUES
    (0, 'lxd');

CREATE TABLE machine_constraint (
    machine_uuid     TEXT PRIMARY KEY,
    constraint_uuid  TEXT NOT NULL,
    CONSTRAINT       fk_machine_constraint_machine
        FOREIGN KEY  (machine_uuid)
        REFERENCES   machine(uuid),
    CONSTRAINT       fk_machine_constraint_constraint
        FOREIGN KEY  (constraint_uuid)
        REFERENCES   "constraint"(uuid)
);

CREATE TABLE machine_principal (
    machine_uuid      TEXT NOT NULL,
    unit_uuid         TEXT NOT NULL,
    CONSTRAINT        fk_machine_principal_machine
        FOREIGN KEY   (machine_uuid)
        REFERENCES    machine(uuid),
    CONSTRAINT        fk_machine_principal_unit
        FOREIGN KEY   (unit_uuid)
        REFERENCES    unit(uuid),
    PRIMARY KEY (machine_uuid, unit_uuid)
);

CREATE TABLE machine_tool (
    machine_uuid     TEXT NOT NULL,
    tool_url         TEXT NOT NULL,
    tool_version     TEXT NOT NULL,
    tool_sha256      TEXT NOT NULL,
    tool_size        INT NOT NULL,
    CONSTRAINT       fk_machine_principal_machine
        FOREIGN KEY  (machine_uuid)
        REFERENCES   machine(uuid),
    PRIMARY KEY (machine_uuid, tool_url)
);

CREATE TABLE machine_job (
    machine_uuid     TEXT NOT NULL,
    machine_job      INT NOT NULL,
    CONSTRAINT       fk_machine_principal_machine
        FOREIGN KEY  (machine_uuid)
        REFERENCES   machine(uuid),
    PRIMARY KEY (machine_uuid, machine_job)
);

CREATE TABLE machine_volume (
    machine_uuid     TEXT NOT NULL,
    volume_uuid      TEXT NOT NULL,
    CONSTRAINT       fk_machine_volume_machine
        FOREIGN KEY  (machine_uuid)
        REFERENCES   machine(uuid),
    CONSTRAINT       fk_machine_volume_volume
        FOREIGN KEY  (volume_uuid)
        REFERENCES   storage_volume(uuid),
    PRIMARY KEY (machine_uuid, volume_uuid)
);

CREATE TABLE machine_filesystem (
    machine_uuid     TEXT NOT NULL,
    filesystem_uuid  TEXT NOT NULL,
    CONSTRAINT       fk_machine_filesystem_machine
        FOREIGN KEY  (machine_uuid)
        REFERENCES   machine(uuid),
    CONSTRAINT       fk_machine_filesystem_filesystem
        FOREIGN KEY  (filesystem_uuid)
        REFERENCES   storage_filesystem(uuid),
    PRIMARY KEY (machine_uuid, filesystem_uuid)
);

CREATE TABLE address_type (
    id      INT PRIMARY KEY,
    value   TEXT NOT NULL
);

INSERT INTO address_type VALUES
    (0, 'hostname'),
    (1, 'ipv4'),
    (2, 'ipv6');

CREATE TABLE address_scope (
    id      INT PRIMARY KEY,
    value   TEXT NOT NULL
);

INSERT INTO address_scope VALUES
    (0, 'unknown'),
    (1, 'public'),
    (2, 'local-cloud'),
    (3, 'local-machine'),
    (4, 'link-local');

CREATE TABLE address_origin (
    id      INT PRIMARY KEY,
    value   TEXT NOT NULL
);

INSERT INTO address_origin VALUES
    (0, 'unknown'),
    (1, 'provider'),
    (2, 'machine');

CREATE TABLE address (
    uuid            TEXT PRIMARY KEY,
    value           TEXT NOT NULL,
    address_type_id TEXT NOT NULL,
    scope_id        TEXT NOT NULL,
    origin_id       TEXT NOT NULL,
    space_id        TEXT NOT NULL,
    CONSTRAINT      fk_address_address_type
        FOREIGN KEY (address_type_id)
        REFERENCES  address_type(id),
    CONSTRAINT      fk_address_space
        FOREIGN KEY (scope_id)
        REFERENCES  address_scope(id),
    CONSTRAINT      fk_address_origin
        FOREIGN KEY (origin_id)
        REFERENCES  address_origin(id),
    CONSTRAINT      fk_address_space
        FOREIGN KEY (space_id)
        REFERENCES  space(uuid)
);

CREATE TABLE machine_instance_address (
    machine_uuid     TEXT NOT NULL,
    address_uuid     TEXT NOT NULL,
    CONSTRAINT       fk_machine_address_machine
        FOREIGN KEY  (machine_uuid)
        REFERENCES   machine(uuid),
    CONSTRAINT       fk_machine_address_address
        FOREIGN KEY  (address_uuid)
        REFERENCES   address(uuid),
    PRIMARY KEY (machine_uuid, address_uuid)
);

CREATE TABLE machine_machine_address (
    machine_uuid            TEXT NOT NULL,
    machine_address_uuid    TEXT NOT NULL,
    CONSTRAINT              fk_machine_address_machine
        FOREIGN KEY         (machine_uuid)
        REFERENCES          machine(uuid),
    CONSTRAINT              fk_machine_address_address
        FOREIGN KEY         (machine_address_uuid)
        REFERENCES          address(uuid),
    PRIMARY KEY (machine_uuid, machine_address_uuid)
);