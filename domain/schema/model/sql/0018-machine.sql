CREATE TABLE machine (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL,
    net_node_uuid TEXT NOT NULL,
    life_id INT NOT NULL,
    nonce TEXT,
    password_hash_algorithm_id TEXT,
    password_hash TEXT,
    force_destroyed BOOLEAN DEFAULT FALSE,
    agent_started_at DATETIME,
    hostname TEXT,
    keep_instance BOOLEAN,
    CONSTRAINT fk_machine_net_node
    FOREIGN KEY (net_node_uuid)
    REFERENCES net_node (uuid),
    CONSTRAINT fk_machine_life
    FOREIGN KEY (life_id)
    REFERENCES life (id)
);

CREATE UNIQUE INDEX idx_machine_name
ON machine (name);

CREATE UNIQUE INDEX idx_machine_net_node
ON machine (net_node_uuid);

CREATE TABLE machine_platform (
    machine_uuid TEXT NOT NULL,
    os_id TEXT NOT NULL,
    channel TEXT,
    architecture_id INT NOT NULL,
    CONSTRAINT fk_machine_platform_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid),
    CONSTRAINT fk_machine_platform_os
    FOREIGN KEY (os_id)
    REFERENCES os (id),
    CONSTRAINT fk_machine_platform_architecture
    FOREIGN KEY (architecture_id)
    REFERENCES architecture (id)
);

-- This table is only used to track whether a machine is a controller or not.
-- It should be sparse and only contain a single row for the controller
-- machines.
CREATE TABLE machine_controller (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    CONSTRAINT fk_machine_controller_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid)
);

-- machine_placement_scope is a table which represents the valid scopes
-- that can exist for a machine placement. The provider scope is the only
-- placement that is deferred until the instance is started by the provider.
-- Other scopes can be added i.e. scriptlets.
CREATE TABLE machine_placement_scope (
    id INT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT INTO machine_placement_scope VALUES
(0, 'provider');

CREATE TABLE machine_placement (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    scope_id INT NOT NULL,
    directive TEXT NOT NULL,
    CONSTRAINT fk_machine_placement_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid),
    CONSTRAINT fk_machine_placement_scope
    FOREIGN KEY (scope_id)
    REFERENCES machine_placement_scope (id)
);

-- machine_parent table is a table which represents parents-children
-- relationships of machines. Each machine can have a single parent or be a
-- parent to multiple children.
CREATE TABLE machine_parent (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    parent_uuid TEXT NOT NULL,
    CONSTRAINT fk_machine_parent_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid),
    CONSTRAINT fk_machine_parent_parent
    FOREIGN KEY (parent_uuid)
    REFERENCES machine (uuid)
);

-- machine_agent_version tracks the reported agent version running for each
-- machine.
CREATE TABLE machine_agent_version (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    version TEXT NOT NULL,
    -- We don't want to link architecture here with that of the architecture
    -- that is on the machine. While correlation can be applied one deals with
    -- what should be the case and this field deals with what is running.
    architecture_id INT NOT NULL,
    CONSTRAINT fk_machine_agent_version_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid),
    CONSTRAINT fk_machine_agent_version_architecture
    FOREIGN KEY (architecture_id)
    REFERENCES architecture (id)
);

-- v_machine_agent_version provides a convenience view on the
-- machine_agent_version reporting the architecture name as well as the id.
-- This currently exists as a view because SQLAir doesn't support AS redefines
-- on select columns. SQLAir issue #179 was created to track this.
CREATE VIEW v_machine_agent_version AS
SELECT
    m.name,
    mav.machine_uuid,
    mav.architecture_id,
    mav.version,
    a.name AS architecture_name
FROM machine_agent_version AS mav
JOIN machine AS m ON mav.machine_uuid = m.uuid
JOIN architecture AS a ON mav.architecture_id = a.id;

-- v_machine_target_agent_version provides a convenience view for establishing
-- what the current target agent version  for a machine. A machine will only
-- have a record in this view if a target agent version has been set for the
-- model and the machine has had its running machine agent version set.
CREATE VIEW v_machine_target_agent_version AS
SELECT
    m.name,
    mav.machine_uuid,
    mav.architecture_id,
    a.name AS architecture_name,
    mav.version,
    av.target_version
FROM machine_agent_version AS mav
JOIN machine AS m ON mav.machine_uuid = m.uuid
JOIN architecture AS a ON mav.architecture_id = a.id
JOIN agent_version AS av;

CREATE TABLE machine_constraint (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    constraint_uuid TEXT NOT NULL,
    CONSTRAINT fk_machine_constraint_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid),
    CONSTRAINT fk_machine_constraint_constraint
    FOREIGN KEY (constraint_uuid)
    REFERENCES "constraint" (uuid)
);

CREATE TABLE machine_volume (
    machine_uuid TEXT NOT NULL,
    volume_uuid TEXT NOT NULL,
    CONSTRAINT fk_machine_volume_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid),
    CONSTRAINT fk_machine_volume_volume
    FOREIGN KEY (volume_uuid)
    REFERENCES storage_volume (uuid),
    PRIMARY KEY (machine_uuid, volume_uuid)
);

CREATE TABLE machine_filesystem (
    machine_uuid TEXT NOT NULL,
    filesystem_uuid TEXT NOT NULL,
    CONSTRAINT fk_machine_filesystem_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid),
    CONSTRAINT fk_machine_filesystem_filesystem
    FOREIGN KEY (filesystem_uuid)
    REFERENCES storage_filesystem (uuid),
    PRIMARY KEY (machine_uuid, filesystem_uuid)
);

CREATE TABLE machine_requires_reboot (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    created_at DATETIME NOT NULL DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc')),
    CONSTRAINT fk_machine_requires_reboot_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid)
);

CREATE TABLE machine_status_value (
    id INT PRIMARY KEY,
    status TEXT NOT NULL
);

INSERT INTO machine_status_value VALUES
(0, 'error'),
(1, 'started'),
(2, 'pending'),
(3, 'stopped'),
(4, 'down');

CREATE TABLE machine_status (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    status_id INT NOT NULL,
    message TEXT,
    data TEXT,
    updated_at DATETIME,
    CONSTRAINT fk_machine_constraint_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid),
    CONSTRAINT fk_machine_constraint_status
    FOREIGN KEY (status_id)
    REFERENCES machine_status_value (id)
);

CREATE VIEW v_machine_status AS
SELECT
    ms.machine_uuid,
    ms.message,
    ms.data,
    ms.updated_at,
    msv.status
FROM machine_status AS ms
JOIN machine_status_value AS msv ON ms.status_id = msv.id;

-- machine_removals table is a table which represents machines that are marked
-- for removal.
-- Being added to this table means that the machine is marked for removal,
CREATE TABLE machine_removals (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    CONSTRAINT fk_machine_removals_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid)
);

-- machine_lxd_profile table keeps track of the lxd profiles (previously
-- charm-profiles) for a machine.
CREATE TABLE machine_lxd_profile (
    machine_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    array_index INT NOT NULL,
    PRIMARY KEY (machine_uuid, name),
    CONSTRAINT fk_lxd_profile_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid)
);

-- container_type represents the valid container types that can exist for an
-- instance.
CREATE TABLE container_type (
    id INT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT INTO container_type VALUES
(0, 'none'),
(1, 'lxd');

CREATE TABLE machine_agent_presence (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    last_seen DATETIME,
    CONSTRAINT fk_machine_agent_presence_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid)
);
