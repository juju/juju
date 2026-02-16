DROP VIEW v_offer_detail;

CREATE VIEW v_offer_detail AS
WITH conn AS (
    SELECT offer_uuid FROM offer_connection
),

active_conn AS (
    SELECT oc.offer_uuid FROM offer_connection AS oc
    JOIN relation_status AS rs ON oc.remote_relation_uuid = rs.relation_uuid
    WHERE rs.relation_status_type_id = 1
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
    (SELECT COUNT(*) FROM conn AS c WHERE o.uuid = c.offer_uuid) AS total_connections,
    (SELECT COUNT(*) FROM active_conn AS ac WHERE o.uuid = ac.offer_uuid) AS total_active_connections
FROM offer AS o
JOIN offer_endpoint AS oe ON o.uuid = oe.offer_uuid
JOIN application_endpoint AS ae ON oe.endpoint_uuid = ae.uuid
JOIN application AS a ON ae.application_uuid = a.uuid
JOIN charm_relation AS cr ON ae.charm_relation_uuid = cr.uuid
JOIN charm_relation_role AS crr ON cr.role_id = crr.id
JOIN charm AS c ON a.charm_uuid = c.uuid
JOIN charm_source AS cs ON c.source_id = cs.id
JOIN charm_metadata AS cm ON c.uuid = cm.charm_uuid;
