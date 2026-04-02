DROP VIEW v_application_resource;

CREATE VIEW v_application_resource AS
SELECT
    r.uuid,
    r.name,
    r.created_at,
    r.revision,
    r.origin_type,
    r.state,
    r.retrieved_by,
    r.retrieved_by_type,
    r.path,
    r.description,
    r.kind_name,
    r.size,
    r.sha384,
    ar.application_uuid,
    a.name AS application_name
FROM v_resource AS r
JOIN application_resource AS ar ON r.uuid = ar.resource_uuid
JOIN application AS a ON ar.application_uuid = a.uuid;
