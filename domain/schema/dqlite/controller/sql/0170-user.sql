-- view_cloud_credential is a convenience view for accessing a
-- credential UUID based on the natural key used to display the
-- credential to users.
CREATE VIEW v_cloud_credential
AS
SELECT
    cc.uuid,
    cc.cloud_uuid,
    c.name AS cloud_name,
    cc.auth_type_id,
    at.type AS auth_type,
    cc.owner_uuid,
    cc.name,
    cc.revoked,
    cc.invalid,
    cc.invalid_reason,
    u.name AS owner_name
FROM cloud_credential AS cc
JOIN cloud AS c ON cc.cloud_uuid = c.uuid
JOIN user AS u ON cc.owner_uuid = u.uuid
JOIN auth_type AS at ON cc.auth_type_id = at.id;

-- v_cloud_credential_attribute returns a view of all cloud credentials
-- and their attributes repeated for every attribute.
CREATE VIEW v_cloud_credential_attribute
AS
SELECT
    cc.uuid,
    cc.cloud_uuid,
    cc.auth_type_id,
    cc.auth_type,
    cc.owner_uuid,
    cc.name,
    cc.revoked,
    cc.invalid,
    cc.invalid_reason,
    cc.cloud_name,
    cc.owner_name,
    cca."key" AS attribute_key,
    cca.value AS attribute_value
FROM v_cloud_credential AS cc
JOIN cloud_credential_attribute AS cca ON cc.uuid = cca.cloud_credential_uuid;

-- v_model_all is a view that provides a simple way to access models
-- that have not been activated. This is useful for the model creation process
-- where we need to access the model to update it but we do not want to show it
-- to the user until it is ready.
CREATE VIEW v_model_all AS
SELECT
    m.uuid,
    m.life_id,
    m.cloud_uuid,
    c.name AS cloud_name,
    ct.type AS cloud_type,
    c.endpoint AS cloud_endpoint,
    c.skip_tls_verify AS cloud_skip_tls_verify,
    cr.uuid AS cloud_region_uuid,
    cr.name AS cloud_region_name,
    cc.uuid AS cloud_credential_uuid,
    cc.name AS cloud_credential_name,
    cc.invalid AS cloud_credential_invalid,
    ccc.name AS cloud_credential_cloud_name,
    cco.uuid AS cloud_credential_owner_uuid,
    cco.name AS cloud_credential_owner_name,
    m.model_type_id,
    mt.type AS model_type,
    m.qualifier,
    m.name,
    l.value AS life,
    m.activated,
    -- Don't rely on controller_uuid always being set to a value.
    ctrli.uuid AS controller_uuid,
    IIF(ctrlm.model_uuid IS NOT NULL, TRUE, FALSE) AS is_controller_model
FROM model AS m
JOIN cloud AS c ON m.cloud_uuid = c.uuid
JOIN cloud_type AS ct ON c.cloud_type_id = ct.id
JOIN model_type AS mt ON m.model_type_id = mt.id
JOIN life AS l ON m.life_id = l.id
LEFT JOIN controller AS ctrli
LEFT JOIN controller AS ctrlm ON m.uuid = ctrlm.model_uuid
LEFT JOIN cloud_region AS cr ON m.cloud_region_uuid = cr.uuid
LEFT JOIN cloud_credential AS cc ON m.cloud_credential_uuid = cc.uuid
LEFT JOIN cloud AS ccc ON cc.cloud_uuid = ccc.uuid
LEFT JOIN user AS cco ON cc.owner_uuid = cco.uuid;

--- v_model purpose is to provide an easy access mechanism for models in the
--- system. It will only show models that have been activated so the caller does
--- not have to worry about retrieving half complete models.
CREATE VIEW v_model AS
SELECT
    uuid,
    cloud_uuid,
    cloud_name,
    cloud_type,
    cloud_endpoint,
    cloud_skip_tls_verify,
    cloud_region_uuid,
    cloud_region_name,
    cloud_credential_uuid,
    cloud_credential_name,
    cloud_credential_invalid,
    cloud_credential_cloud_name,
    cloud_credential_owner_uuid,
    cloud_credential_owner_name,
    model_type_id,
    model_type,
    name,
    qualifier,
    life_id,
    life,
    activated,
    controller_uuid,
    is_controller_model
FROM v_model_all
WHERE activated = TRUE;

CREATE UNIQUE INDEX idx_singleton_active_user ON user (name) WHERE removed IS FALSE;

CREATE INDEX idx_user_name ON user (name);

CREATE INDEX idx_user_removed
ON user (removed);

CREATE TABLE user_authentication (
    user_uuid TEXT NOT NULL PRIMARY KEY,
    disabled BOOLEAN NOT NULL,
    CONSTRAINT fk_user_authentication_user
    FOREIGN KEY (user_uuid)
    REFERENCES user (uuid)
);

CREATE INDEX idx_user_authentication_disabled_user_uuid
ON user_authentication (disabled, user_uuid);

CREATE TABLE user_password (
    user_uuid TEXT NOT NULL PRIMARY KEY,
    password_hash TEXT NOT NULL,
    password_salt TEXT NOT NULL,
    CONSTRAINT fk_user_password_user
    FOREIGN KEY (user_uuid)
    REFERENCES user_authentication (user_uuid)
);

CREATE TABLE user_activation_key (
    user_uuid TEXT NOT NULL PRIMARY KEY,
    activation_key BLOB NOT NULL,
    CONSTRAINT fk_user_activation_key_user
    FOREIGN KEY (user_uuid)
    REFERENCES user_authentication (user_uuid)
);

CREATE VIEW v_user_auth AS
SELECT
    u.uuid,
    u.name,
    u.display_name,
    u.external,
    u.removed,
    u.created_by_uuid,
    u.created_at,
    a.disabled
FROM user AS u LEFT JOIN user_authentication AS a ON u.uuid = a.user_uuid;
