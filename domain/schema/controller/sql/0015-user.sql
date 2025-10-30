CREATE TABLE user (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL,
    display_name TEXT,
    external INTEGER NOT NULL,
    removed INTEGER NOT NULL DEFAULT FALSE,
    created_by_uuid TEXT NOT NULL,
    created_at ANY NOT NULL,
    CONSTRAINT fk_user_created_by_user
    FOREIGN KEY (created_by_uuid)
    REFERENCES user (uuid)
) STRICT;

CREATE UNIQUE INDEX idx_singleton_active_user ON user (name) WHERE removed IS FALSE;

CREATE TABLE user_authentication (
    user_uuid TEXT NOT NULL PRIMARY KEY,
    disabled INTEGER NOT NULL,
    CONSTRAINT fk_user_authentication_user
    FOREIGN KEY (user_uuid)
    REFERENCES user (uuid)
) STRICT;

CREATE TABLE user_password (
    user_uuid TEXT NOT NULL PRIMARY KEY,
    password_hash TEXT NOT NULL,
    password_salt BLOB NOT NULL,
    CONSTRAINT fk_user_password_user
    FOREIGN KEY (user_uuid)
    REFERENCES user_authentication (user_uuid)
) STRICT;

CREATE TABLE user_activation_key (
    user_uuid TEXT NOT NULL PRIMARY KEY,
    activation_key BLOB NOT NULL,
    CONSTRAINT fk_user_activation_key_user
    FOREIGN KEY (user_uuid)
    REFERENCES user_authentication (user_uuid)
) STRICT;

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
