-- Controller database tables for secret backends.

CREATE TABLE secret_backend_type (
    id INT PRIMARY KEY,
    type TEXT NOT NULL,
    description TEXT,
    CONSTRAINT chk_empty_type
    CHECK (type != '')
);

CREATE UNIQUE INDEX idx_secret_backend_type_type
ON secret_backend_type (type);

INSERT INTO secret_backend_type VALUES
(0, 'controller', 'the juju controller secret backend'),
(1, 'kubernetes', 'the kubernetes secret backend'),
(2, 'vault', 'the vault secret backend');

CREATE TABLE secret_backend (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL,
    backend_type_id INT NOT NULL,
    token_rotate_interval INT,
    CONSTRAINT chk_empty_name
    CHECK (name != ''),
    CONSTRAINT fk_secret_backend_type_id
    FOREIGN KEY (backend_type_id)
    REFERENCES secret_backend_type (id)
);

CREATE UNIQUE INDEX idx_secret_backend_name
ON secret_backend (name);

CREATE TABLE secret_backend_config (
    backend_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    content TEXT NOT NULL,
    CONSTRAINT chk_empty_name
    CHECK (name != ''),
    CONSTRAINT chk_empty_content
    CHECK (content != ''),
    CONSTRAINT pk_secret_backend_config
    PRIMARY KEY (backend_uuid, name),
    CONSTRAINT fk_secret_backend_config_backend_uuid
    FOREIGN KEY (backend_uuid)
    REFERENCES secret_backend (uuid)
);

CREATE TABLE secret_backend_rotation (
    backend_uuid TEXT NOT NULL PRIMARY KEY,
    next_rotation_time DATETIME NOT NULL,
    CONSTRAINT fk_secret_backend_rotation_secret_backend_uuid
    FOREIGN KEY (backend_uuid)
    REFERENCES secret_backend (uuid)
);

CREATE TABLE secret_backend_reference (
    secret_backend_uuid TEXT NOT NULL,
    model_uuid TEXT NOT NULL,
    secret_revision_uuid TEXT NOT NULL,
    CONSTRAINT pk_secret_backend_reference
    PRIMARY KEY (secret_backend_uuid, model_uuid, secret_revision_uuid),
    CONSTRAINT fk_secret_backend_reference_model_uuid
    FOREIGN KEY (model_uuid)
    REFERENCES model (uuid),
    CONSTRAINT fk_secret_backend_reference_secret_backend_uuid
    FOREIGN KEY (secret_backend_uuid)
    REFERENCES secret_backend (uuid)
);

CREATE TABLE model_secret_backend (
    model_uuid TEXT NOT NULL PRIMARY KEY,
    secret_backend_uuid TEXT NOT NULL,
    CONSTRAINT fk_model_secret_backend_model_uuid
    FOREIGN KEY (model_uuid)
    REFERENCES model (uuid),
    CONSTRAINT fk_model_secret_backend_secret_backend_uuid
    FOREIGN KEY (secret_backend_uuid)
    REFERENCES secret_backend (uuid)
);

CREATE VIEW v_model_secret_backend AS
SELECT
    m.uuid,
    m.name,
    mt.type AS model_type,
    msb.secret_backend_uuid,
    sb.name AS secret_backend_name,
    (SELECT uuid FROM controller) AS controller_uuid
FROM model_secret_backend AS msb
JOIN secret_backend AS sb ON msb.secret_backend_uuid = sb.uuid
JOIN model AS m ON msb.model_uuid = m.uuid
JOIN model_type AS mt ON m.model_type_id = mt.id;
