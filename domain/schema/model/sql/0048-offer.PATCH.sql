-- Add indexes for efficient view queries.
CREATE INDEX IF NOT EXISTS idx_offer_connection_offer_uuid
ON offer_connection (offer_uuid);

CREATE INDEX IF NOT EXISTS idx_offer_connection_remote_relation_offer
ON offer_connection (remote_relation_uuid, offer_uuid);

CREATE INDEX IF NOT EXISTS idx_relation_status_type_relation
ON relation_status (relation_status_type_id, relation_uuid);

DROP VIEW v_offer_detail;

CREATE VIEW v_offer_detail AS
WITH total_conn AS (
    SELECT
        offer_uuid,
        COUNT(*) AS total_connections
    FROM offer_connection
    GROUP BY offer_uuid
),

active_conn AS (
    SELECT
        oc.offer_uuid,
        COUNT(*) AS total_active_connections
    FROM offer_connection AS oc
    JOIN relation_status AS rs
        ON oc.remote_relation_uuid = rs.relation_uuid
        AND rs.relation_status_type_id = 1
    GROUP BY oc.offer_uuid
)

SELECT
    o.uuid AS offer_uuid,
    o.name AS offer_name,
    a.name AS application_name,
    cm.description AS application_description,
    c.reference_name AS charm_name,
    c.revision AS charm_revision,
    cs.name AS charm_source,
    c.architecture_id AS charm_architecture,
    cr.name AS endpoint_name,
    crr.name AS endpoint_role,
    cr.interface AS endpoint_interface,
    cr.capacity AS endpoint_limit,
    COALESCE(tc.total_connections, 0) AS total_connections,
    COALESCE(ac.total_active_connections, 0) AS total_active_connections
FROM offer AS o
JOIN offer_endpoint AS oe ON o.uuid = oe.offer_uuid
JOIN application_endpoint AS ae ON oe.endpoint_uuid = ae.uuid
JOIN application AS a ON ae.application_uuid = a.uuid
JOIN charm AS c ON a.charm_uuid = c.uuid
JOIN charm_relation AS cr ON ae.charm_relation_uuid = cr.uuid
JOIN charm_relation_role AS crr ON cr.role_id = crr.id
JOIN charm_source AS cs ON c.source_id = cs.id
JOIN charm_metadata AS cm ON c.uuid = cm.charm_uuid
LEFT JOIN total_conn AS tc ON tc.offer_uuid = o.uuid
LEFT JOIN active_conn AS ac ON ac.offer_uuid = o.uuid;
