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
    REFERENCES life (id),
    CONSTRAINT fk_machine_password_hash_algorithm
    FOREIGN KEY (password_hash_algorithm_id)
    REFERENCES password_hash_algorithm (id)
);

CREATE UNIQUE INDEX idx_machine_name
ON machine (name);

CREATE UNIQUE INDEX idx_machine_net_node
ON machine (net_node_uuid);

CREATE TABLE machine_manual (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    CONSTRAINT fk_machine_manual_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid)
);

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

CREATE TABLE machine_container_type (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    container_type_id INT NOT NULL,
    CONSTRAINT fk_machine_container_type_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid),
    CONSTRAINT fk_machine_container_type_container_type
    FOREIGN KEY (container_type_id)
    REFERENCES container_type (id)
);

CREATE TABLE machine_agent_presence (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    last_seen DATETIME,
    CONSTRAINT fk_machine_agent_presence_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid)
);

CREATE VIEW v_machine_is_controller AS
SELECT m.uuid AS machine_uuid
FROM machine AS m
JOIN net_node AS n ON m.net_node_uuid = n.uuid
JOIN unit AS u ON n.uuid = u.net_node_uuid
JOIN application AS a ON u.application_uuid = a.uuid
JOIN application_controller AS ac ON a.uuid = ac.application_uuid;

CREATE VIEW v_machine_constraint AS
SELECT
    mc.machine_uuid,
    c.arch,
    c.cpu_cores,
    c.cpu_power,
    c.mem,
    c.root_disk,
    c.root_disk_source,
    c.instance_role,
    c.instance_type,
    ctype.value AS container_type,
    c.virt_type,
    c.allocate_public_ip,
    c.image_id,
    ctag.tag,
    cspace.space AS space_name,
    cspace."exclude" AS space_exclude,
    czone.zone
FROM machine_constraint AS mc
JOIN "constraint" AS c ON mc.constraint_uuid = c.uuid
LEFT JOIN container_type AS ctype ON c.container_type_id = ctype.id
LEFT JOIN constraint_tag AS ctag ON c.uuid = ctag.constraint_uuid
LEFT JOIN constraint_space AS cspace ON c.uuid = cspace.constraint_uuid
LEFT JOIN constraint_zone AS czone ON c.uuid = czone.constraint_uuid;

CREATE VIEW v_machine_platform AS
SELECT
    mp.machine_uuid,
    os.name AS os_name,
    mp.channel,
    a.name AS architecture
FROM machine_platform AS mp
JOIN os ON mp.os_id = os.id
JOIN architecture AS a ON mp.architecture_id = a.id;
