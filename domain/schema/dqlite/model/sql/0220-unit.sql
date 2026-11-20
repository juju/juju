CREATE VIEW v_model_metrics AS
SELECT
    (SELECT COUNT(DISTINCT uuid) FROM application) AS application_count,
    (SELECT COUNT(DISTINCT uuid) FROM machine) AS machine_count,
    (SELECT COUNT(DISTINCT uuid) FROM unit) AS unit_count;

-- v_secret_permission is used to query secrets which can
-- be accessed by a subject of application, unit, or model.
CREATE VIEW v_secret_permission AS
SELECT
    sp.secret_id,
    sp.role_id,
    sp.subject_type_id,
    sp.subject_uuid,
    sp.scope_type_id,
    sp.scope_uuid,
    -- subject_id is the natural id of the subject entity (uuid for model)
    (CASE
        WHEN sp.subject_type_id = 0 THEN suu.name
        WHEN sp.subject_type_id = 1 THEN sua.name
        WHEN sp.subject_type_id = 2 THEN m.uuid
    END) AS subject_id,
    -- scope_id is the natural id of the scope entity (uuid for model)
    -- relations are not processed here - their scope_id is the relation key
    -- which needs to be composed in code.
    (CASE
        WHEN sp.scope_type_id = 0 THEN scu.name
        WHEN sp.scope_type_id = 1 THEN sca.name
        WHEN sp.scope_type_id = 2 THEN m.uuid
    END) AS scope_id
FROM secret_permission AS sp
LEFT JOIN unit AS suu ON sp.subject_uuid = suu.uuid
LEFT JOIN application AS sua ON sp.subject_uuid = sua.uuid
LEFT JOIN unit AS scu ON sp.scope_uuid = scu.uuid
LEFT JOIN application AS sca ON sp.scope_uuid = sca.uuid
JOIN model AS m;

CREATE VIEW v_secret_owner AS
SELECT
    'model' AS owner_kind,
    m.uuid AS owner_uuid,
    m.name AS owner_name,
    so.label,
    so.secret_id
FROM secret_model_owner AS so
JOIN model AS m -- this is a singleton
UNION ALL
SELECT
    'application' AS owner_kind,
    so.application_uuid AS owner_uuid,
    a.name AS owner_name,
    so.label,
    so.secret_id
FROM secret_application_owner AS so
JOIN application AS a ON so.application_uuid = a.uuid
UNION ALL
SELECT
    'unit' AS owner_kind,
    so.unit_uuid AS owner_uuid,
    u.name AS owner_name,
    so.label,
    so.secret_id
FROM secret_unit_owner AS so
JOIN unit AS u ON so.unit_uuid = u.uuid;

CREATE VIEW v_secret_metadata AS
SELECT
    sm.secret_id,
    sm.version,
    sm.description,
    sm.auto_prune,
    sm.latest_revision_checksum,
    sm.create_time,
    sm.update_time,
    rp.policy,
    sro.next_rotation_time,
    sre.expire_time,
    sr.revision,
    so.owner_kind,
    so.owner_uuid,
    so.owner_name,
    so.label
FROM secret_metadata AS sm
JOIN secret_rotate_policy AS rp ON sm.rotate_policy_id = rp.id
JOIN secret_revision AS sr ON sm.secret_id = sr.secret_id
LEFT JOIN secret_revision_expire AS sre ON sr.uuid = sre.revision_uuid
LEFT JOIN secret_rotation AS sro ON sm.secret_id = sro.secret_id
LEFT JOIN v_secret_owner AS so ON sm.secret_id = so.secret_id;

CREATE UNIQUE INDEX idx_unit_name
ON unit (name);

-- unit passwords are unique across all units. This is to prevent
-- a unit from being able to impersonate another unit. NULL passwords are
-- allowed for multiple units, as NULLs are considered distinct.
CREATE UNIQUE INDEX idx_unit_password_hash
ON unit (password_hash);

CREATE INDEX idx_unit_application
ON unit (application_uuid);

CREATE INDEX idx_unit_net_node
ON unit (net_node_uuid);

-- unit_principal table is a table which is used to store the
-- principal units for subordinate units.
CREATE TABLE unit_principal (
    unit_uuid TEXT NOT NULL PRIMARY KEY,
    principal_uuid TEXT NOT NULL,
    CONSTRAINT fk_unit_principal_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    CONSTRAINT fk_unit_principal_principal
    FOREIGN KEY (principal_uuid)
    REFERENCES unit (uuid)
);

CREATE INDEX idx_unit_principal_principal
ON unit_principal (principal_uuid, unit_uuid);

CREATE TABLE unit_workload_version (
    unit_uuid TEXT NOT NULL PRIMARY KEY,
    version TEXT NOT NULL,
    CONSTRAINT fk_unit_workload_version_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid)
);

-- unit_agent_version tracks the reported agent version running for each
-- unit.
CREATE TABLE unit_agent_version (
    unit_uuid TEXT NOT NULL PRIMARY KEY,
    version TEXT NOT NULL,
    architecture_id INT NOT NULL,
    CONSTRAINT fk_unit_agent_version_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    CONSTRAINT fk_unit_agent_version_architecture
    FOREIGN KEY (architecture_id)
    REFERENCES architecture (id)
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
    unit_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (unit_uuid, "key"),
    CONSTRAINT fk_unit_state_charm_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid)
);

-- Local relation state stored upon hook commit with uniter state.
CREATE TABLE unit_state_relation (
    unit_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (unit_uuid, "key"),
    CONSTRAINT fk_unit_state_relation_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid)
);

-- cloud containers belong to a k8s unit.
CREATE TABLE k8s_pod (
    unit_uuid TEXT NOT NULL PRIMARY KEY,
    -- provider_id comes from the provider, no FK.
    -- it represents the k8s pod name.
    provider_id TEXT NOT NULL,
    CONSTRAINT fk_k8s_pod_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid)
);

CREATE UNIQUE INDEX idx_k8s_pod_provider
ON k8s_pod (provider_id);

CREATE TABLE k8s_pod_port (
    unit_uuid TEXT NOT NULL,
    port TEXT NOT NULL,
    CONSTRAINT fk_k8s_pod_port_k8s_pod
    FOREIGN KEY (unit_uuid)
    REFERENCES k8s_pod (unit_uuid),
    PRIMARY KEY (unit_uuid, port)
);

-- Status values for unit agents.
CREATE TABLE unit_agent_status_value (
    id INT PRIMARY KEY,
    status TEXT NOT NULL
);

INSERT INTO unit_agent_status_value VALUES
(0, 'allocating'),
(1, 'executing'),
(2, 'idle'),
(3, 'error'),
(4, 'failed'),
(5, 'lost'),
(6, 'rebooting');

-- Status values for cloud containers.
CREATE TABLE k8s_pod_status_value (
    id INT PRIMARY KEY,
    status TEXT NOT NULL
);

INSERT INTO k8s_pod_status_value VALUES
(0, 'unset'),
(1, 'waiting'),
(2, 'blocked'),
(3, 'running'),
(4, 'error');

CREATE TABLE unit_agent_status (
    unit_uuid TEXT NOT NULL PRIMARY KEY,
    status_id INT NOT NULL,
    message TEXT,
    data TEXT,
    updated_at DATETIME,
    CONSTRAINT fk_unit_agent_status_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    CONSTRAINT fk_unit_agent_status_status
    FOREIGN KEY (status_id)
    REFERENCES unit_agent_status_value (id)
);

CREATE INDEX idx_unit_agent_status_status_unit
ON unit_agent_status (status_id, unit_uuid);

CREATE TABLE unit_workload_status (
    unit_uuid TEXT NOT NULL PRIMARY KEY,
    status_id INT NOT NULL,
    message TEXT,
    data TEXT,
    updated_at DATETIME,
    CONSTRAINT fk_unit_workload_status_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    CONSTRAINT fk_workload_status_value_status
    FOREIGN KEY (status_id)
    REFERENCES workload_status_value (id)
);

CREATE TABLE k8s_pod_status (
    unit_uuid TEXT NOT NULL PRIMARY KEY,
    status_id INT NOT NULL,
    message TEXT,
    data TEXT,
    updated_at DATETIME,
    CONSTRAINT fk_k8s_pod_status_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    CONSTRAINT fk_k8s_pod_status_status
    FOREIGN KEY (status_id)
    REFERENCES k8s_pod_status_value (id)
);

CREATE TABLE unit_agent_presence (
    unit_uuid TEXT NOT NULL PRIMARY KEY,
    last_seen DATETIME,
    CONSTRAINT fk_unit_agent_presence_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid)
);

CREATE VIEW v_unit_agent_status AS
SELECT
    u.uuid AS unit_uuid,
    u.name AS unit_name,
    u.application_uuid,
    uas.status_id,
    uas.message,
    uas.data,
    uas.updated_at,
    EXISTS(
        SELECT 1 FROM unit_agent_presence AS uap
        WHERE u.uuid = uap.unit_uuid
    ) AS present
FROM unit AS u
JOIN unit_agent_status AS uas ON u.uuid = uas.unit_uuid;

CREATE VIEW v_unit_workload_status AS
SELECT
    u.uuid AS unit_uuid,
    u.name AS unit_name,
    u.application_uuid,
    uws.status_id,
    uws.message,
    uws.data,
    uws.updated_at,
    EXISTS(
        SELECT 1 FROM unit_agent_presence AS uap
        WHERE u.uuid = uap.unit_uuid
    ) AS present
FROM unit AS u
JOIN unit_workload_status AS uws ON u.uuid = uws.unit_uuid;

CREATE VIEW v_unit_workload_agent_status AS
SELECT
    u.uuid AS unit_uuid,
    u.name AS unit_name,
    u.application_uuid,
    uws.status_id AS workload_status_id,
    uws.message AS workload_message,
    uws.data AS workload_data,
    uws.updated_at AS workload_updated_at,
    uas.status_id AS agent_status_id,
    uas.message AS agent_message,
    uas.data AS agent_data,
    uas.updated_at AS agent_updated_at,
    EXISTS(
        SELECT 1 FROM unit_agent_presence AS uap
        WHERE u.uuid = uap.unit_uuid
    ) AS present
FROM unit AS u
LEFT JOIN unit_workload_status AS uws ON u.uuid = uws.unit_uuid
LEFT JOIN unit_agent_status AS uas ON u.uuid = uas.unit_uuid;

CREATE VIEW v_full_unit_status AS
SELECT
    u.uuid AS unit_uuid,
    u.name AS unit_name,
    u.application_uuid,
    uws.status_id AS workload_status_id,
    uws.message AS workload_message,
    uws.data AS workload_data,
    uws.updated_at AS workload_updated_at,
    uas.status_id AS agent_status_id,
    uas.message AS agent_message,
    uas.data AS agent_data,
    uas.updated_at AS agent_updated_at,
    kps.status_id AS k8s_pod_status_id,
    kps.message AS k8s_pod_message,
    kps.data AS k8s_pod_data,
    kps.updated_at AS k8s_pod_updated_at,
    EXISTS(
        SELECT 1 FROM unit_agent_presence AS uap
        WHERE u.uuid = uap.unit_uuid
    ) AS present
FROM unit AS u
LEFT JOIN unit_workload_status AS uws ON u.uuid = uws.unit_uuid
LEFT JOIN unit_agent_status AS uas ON u.uuid = uas.unit_uuid
LEFT JOIN k8s_pod_status AS kps ON u.uuid = kps.unit_uuid;

CREATE VIEW v_unit_password_hash AS
SELECT
    a.uuid AS application_uuid,
    a.name AS application_name,
    u.uuid AS unit_uuid,
    u.name AS unit_name,
    u.password_hash
FROM application AS a
LEFT JOIN unit AS u ON a.uuid = u.application_uuid;

CREATE TABLE resolve_mode (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE unit_resolved (
    unit_uuid TEXT NOT NULL PRIMARY KEY,
    mode_id INT NOT NULL,
    CONSTRAINT fk_unit_resolved_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    CONSTRAINT fk_unit_resolved_mode
    FOREIGN KEY (mode_id)
    REFERENCES resolve_mode (id)
);

INSERT INTO resolve_mode VALUES
(0, 'retry-hooks'),
(1, 'no-hooks');

CREATE VIEW v_unit_attribute AS
SELECT
    u.uuid,
    u.name,
    u.life_id,
    ur.mode_id AS resolve_mode_id,
    k.provider_id
FROM unit AS u
LEFT JOIN unit_resolved AS ur ON u.uuid = ur.unit_uuid
LEFT JOIN k8s_pod AS k ON u.uuid = k.unit_uuid;
