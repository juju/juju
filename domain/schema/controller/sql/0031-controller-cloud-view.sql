-- The controller model is not guaranteed to be qualified by the admin user
-- (see juju bootstrap --controller-qualifier). Redefine v_cloud to identify
-- the controller model's cloud through the controller table's model_uuid,
-- rather than by the controller model's name and qualifier.
--
-- v_cloud_auth selects from v_cloud, so it is dropped first and recreated
-- unchanged afterwards.
DROP VIEW v_cloud_auth;
DROP VIEW v_cloud;

-- v_cloud is used to fetch well-constructed information about a cloud.
-- This view also includes information on whether the cloud is the
-- controller model's cloud.
CREATE VIEW v_cloud
AS
-- This selects the controller model's cloud uuid. We use this when loading
-- clouds to know if the cloud is the controller's cloud.
WITH controllers AS (
    SELECT m.cloud_uuid
    FROM controller AS ctrl
    JOIN model AS m ON ctrl.model_uuid = m.uuid
    WHERE m.activated = TRUE
)

SELECT
    c.uuid,
    c.name,
    c.cloud_type_id,
    ct.type AS cloud_type,
    c.endpoint,
    c.identity_endpoint,
    c.storage_endpoint,
    c.skip_tls_verify,
    IIF(controllers.cloud_uuid IS NULL, FALSE, TRUE) AS is_controller_cloud
FROM cloud AS c
JOIN cloud_type AS ct ON c.cloud_type_id = ct.id
LEFT JOIN controllers ON c.uuid = controllers.cloud_uuid;

-- v_cloud_auth is a view similar to v_cloud but includes a row for
-- each cloud and auth type pair.
CREATE VIEW v_cloud_auth
AS
SELECT
    c.uuid,
    c.name,
    c.cloud_type_id,
    c.cloud_type,
    c.endpoint,
    c.identity_endpoint,
    c.storage_endpoint,
    c.skip_tls_verify,
    c.is_controller_cloud,
    at.id AS auth_type_id,
    at.type AS auth_type
FROM v_cloud AS c
LEFT JOIN cloud_auth_type AS cat ON c.uuid = cat.cloud_uuid
JOIN auth_type AS at ON cat.auth_type_id = at.id;
