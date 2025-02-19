CREATE TABLE model_last_login (
    model_uuid TEXT NOT NULL,
    user_uuid TEXT NOT NULL,
    time TIMESTAMP NOT NULL,
    PRIMARY KEY (model_uuid, user_uuid),
    CONSTRAINT fk_model_last_login_model
    FOREIGN KEY (model_uuid)
    REFERENCES model (uuid),
    CONSTRAINT fk_model_last_login_user
    FOREIGN KEY (user_uuid)
    REFERENCES user (uuid)
);

CREATE VIEW v_user_last_login AS
-- We cannot select last_login as MAX directly here because it returns a sqlite
-- string value, not a timestamp and this stops us scanning into time.Time.
SELECT
    time AS last_login,
    user_uuid,
    MAX(time) AS t
FROM model_last_login
GROUP BY user_uuid;
