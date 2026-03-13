DROP VIEW v_model_secret_backend;

CREATE VIEW v_model_secret_backend AS
SELECT
    m.uuid,
    m.name,
    mt.type AS model_type,
    msb.secret_backend_uuid,
    sb.name AS secret_backend_name,
    sbo.origin AS secret_backend_origin,
    sbt.type AS secret_backend_type,
    (SELECT uuid FROM controller) AS controller_uuid
FROM model_secret_backend AS msb
JOIN secret_backend AS sb ON msb.secret_backend_uuid = sb.uuid
JOIN model AS m ON msb.model_uuid = m.uuid
JOIN model_type AS mt ON m.model_type_id = mt.id
JOIN secret_backend_type AS sbt ON sb.backend_type_id = sbt.id
JOIN secret_backend_origin AS sbo ON sb.origin_id = sbo.id;
