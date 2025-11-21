DROP VIEW v_machine_status;

CREATE VIEW v_machine_status AS
SELECT
    ms.machine_uuid,
    ms.message,
    ms.data,
    ms.updated_at,
    msv.status,
    map.machine_uuid IS NOT NULL AS present
FROM machine_status AS ms
JOIN machine_status_value AS msv ON ms.status_id = msv.id
LEFT JOIN machine_agent_presence AS map ON ms.machine_uuid = map.machine_uuid;
