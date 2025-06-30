CREATE TABLE model_authorized_keys (
    model_uuid TEXT NOT NULL,
    user_public_ssh_key_id INTEGER NOT NULL,
    PRIMARY KEY (model_uuid, user_public_ssh_key_id),
    FOREIGN KEY (user_public_ssh_key_id)
    REFERENCES user_public_ssh_key (id),
    FOREIGN KEY (model_uuid)
    REFERENCES model (uuid)
);

--CREATE UNIQUE INDEX idx_model_authorized_keys_composite
--ON model_authorized_keys (model_uuid, user_public_ssh_key_id);

CREATE INDEX idx_model_authorized_keys_model_uuid
ON model_authorized_keys (model_uuid);

CREATE INDEX idx_model_authorized_keys_model_uuid_user_public_ssh_key_id
ON model_authorized_keys (user_public_ssh_key_id);

-- v_model_authorized_keys provides a nice view of what public ssh keys are
-- currently allowed on a model making sure that users that are removed and or
-- disabled have thier authorized keys removed from the model.
CREATE VIEW v_model_authorized_keys AS
SELECT
    mak.model_uuid,
    upsk.public_key,
    upsk.user_uuid
FROM model_authorized_keys AS mak
JOIN user_public_ssh_key AS upsk ON mak.user_public_ssh_key_id = upsk.id
JOIN user AS u ON upsk.user_uuid = u.uuid
JOIN user_authentication AS ua ON u.uuid = ua.user_uuid
WHERE
    u.removed = FALSE
    AND ua.disabled = FALSE;
