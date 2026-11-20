-- agent_stream defines the recognised streams available in the model for
-- fetching agent binaries.
CREATE TABLE agent_stream (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_agent_stream_name
ON agent_stream (name);

INSERT INTO agent_stream VALUES
(0, 'released'),
(1, 'proposed'),
(2, 'testing'),
(3, 'devel');

CREATE TABLE agent_version (
    stream_id INT NOT NULL,
    target_version TEXT NOT NULL,
    latest_version TEXT NOT NULL,
    FOREIGN KEY (stream_id)
    REFERENCES agent_stream (id)
);

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

-- v_unit_target_agent_version provides a convenience view for establishing what
-- the current target agent version of a unit should be. A unit will only have
-- a record in this view if a target agent version has been set for the model
-- and the unit has had its running unit agent version set.
CREATE VIEW v_unit_target_agent_version AS
SELECT
    u.name,
    uav.unit_uuid,
    uav.architecture_id,
    uav.version,
    av.target_version,
    a.name AS architecture_name
FROM unit_agent_version AS uav
JOIN unit AS u ON uav.unit_uuid = u.uuid
JOIN architecture AS a ON uav.architecture_id = a.id
JOIN agent_version AS av;

-- A unique constraint over a constant index
-- ensures only 1 row can exist.
CREATE UNIQUE INDEX idx_singleton_agent_version ON agent_version ((1));

CREATE VIEW v_model_config AS
SELECT
    mc."key",
    mc.value
FROM model_config AS mc
-- agent_version is authoritative for these keys, so any competing
-- model_config rows are excluded. Keep these key literals in sync with
-- environs/config.AgentStreamKey and environs/config.AgentVersionKey.
WHERE mc."key" NOT IN ('agent-stream', 'agent-version')
UNION ALL
SELECT
    'agent-stream' AS "key",
    mas.name AS value
FROM agent_version AS mv
JOIN agent_stream AS mas ON mv.stream_id = mas.id
UNION ALL
SELECT
    'agent-version' AS "key",
    mv.target_version AS value
FROM agent_version AS mv;
