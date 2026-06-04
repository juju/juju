DROP VIEW v_machine_status;

CREATE VIEW v_machine_status AS
WITH machine_presence AS (
    SELECT machine_uuid
    FROM machine_agent_presence
    UNION
    SELECT mp.parent_uuid AS machine_uuid
    FROM machine_parent AS mp
    JOIN machine_agent_presence AS map ON mp.machine_uuid = map.machine_uuid
)

SELECT
    ms.machine_uuid,
    ms.message,
    ms.data,
    ms.updated_at,
    msv.status,
    mp.machine_uuid IS NOT NULL AS present
FROM machine_status AS ms
JOIN machine_status_value AS msv ON ms.status_id = msv.id
LEFT JOIN machine_presence AS mp ON ms.machine_uuid = mp.machine_uuid;
