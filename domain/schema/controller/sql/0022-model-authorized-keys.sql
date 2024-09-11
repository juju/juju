CREATE TABLE model_authorized_keys (
    model_id TEXT NOT NULL,
    user_public_ssh_key_id INTEGER NOT NULL,
    FOREIGN KEY (user_public_ssh_key_id)
    REFERENCES user_public_ssh_key (id),
    FOREIGN KEY (model_id)
    REFERENCES model (uuid)
);

CREATE UNIQUE INDEX idx_model_authorized_keys_composite
ON model_authorized_keys (model_id, user_public_ssh_key_id);

CREATE INDEX idx_model_authorized_keys_model_id
ON model_authorized_keys (model_id);

CREATE INDEX idx_model_authorized_keys_model_id_user_public_ssh_key_id
ON model_authorized_keys (user_public_ssh_key_id);

-- v_model_authorized_keys provides a nice view of what public ssh keys are
-- currently allowed on a model making sure that users that are removed and or
-- disabled have thier authorized keys removed from the model.
CREATE VIEW v_model_authorized_keys AS
SELECT mak.model_id,
       upsk.public_key
FROM model_authorized_keys AS mak
INNER JOIN user_public_ssh_key AS upsk ON mak.user_public_ssh_key_id = upsk.id
INNER JOIN user AS u ON upsk.user_id = u.uuid
INNER JOIN user_authentication AS ua ON ua.user_uuid = u.uuid
WHERE u.removed = FALSE
AND ua.disabled = FALSE;