CREATE TABLE offer (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL
);

-- The offer_endpoint table is a join table to indicate which application
-- endpoints are included in the offer.
--
-- Note: trg_ensure_single_app_per_offer ensures that for every offer,
-- each endpoint_uuid is for the same application.
CREATE TABLE offer_endpoint (
    offer_uuid TEXT NOT NULL,
    endpoint_uuid TEXT NOT NULL,
    PRIMARY KEY (offer_uuid, endpoint_uuid),
    CONSTRAINT fk_endpoint_uuid
    FOREIGN KEY (endpoint_uuid)
    REFERENCES application_endpoint (uuid),
    CONSTRAINT fk_offer_uuid
    FOREIGN KEY (offer_uuid)
    REFERENCES offer (uuid)
);


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
    JOIN relation_status AS rs ON oc.remote_relation_uuid = rs.relation_uuid AND rs.relation_status_type_id = 1
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
LEFT JOIN total_conn AS tc ON o.uuid = tc.offer_uuid
LEFT JOIN active_conn AS ac ON o.uuid = ac.offer_uuid;
