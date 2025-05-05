-- model_namespace is a mapping table from models to the corresponding dqlite
-- namespace database.
CREATE TABLE model_namespace (
    namespace TEXT NOT NULL,
    model_uuid TEXT UNIQUE NOT NULL,
    CONSTRAINT fk_model_uuid
    FOREIGN KEY (model_uuid)
    REFERENCES model (uuid)
);

CREATE UNIQUE INDEX idx_namespace_model_uuid ON model_namespace (namespace, model_uuid);

CREATE TABLE model_type (
    id INT PRIMARY KEY,
    type TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_model_type_type
ON model_type (type);

INSERT INTO model_type VALUES
(0, 'iaas'),
(1, 'caas');

CREATE TABLE model (
    uuid TEXT NOT NULL PRIMARY KEY,
    -- activated tells us if the model creation process has been completed and
    -- we can use this model. The reason for this is model creation still happens
    -- over several transactions with any one of them possibly failing. We write true
    -- to this field when we are happy that the model can safely be used after all
    -- operations have been completed.
    activated BOOLEAN DEFAULT FALSE NOT NULL,
    cloud_uuid TEXT NOT NULL,
    cloud_region_uuid TEXT,
    cloud_credential_uuid TEXT,
    model_type_id INT NOT NULL,
    life_id INT NOT NULL,
    name TEXT NOT NULL,
    owner_uuid TEXT NOT NULL,
    CONSTRAINT fk_model_cloud
    FOREIGN KEY (cloud_uuid)
    REFERENCES cloud (uuid),
    CONSTRAINT fk_model_cloud_region
    FOREIGN KEY (cloud_region_uuid)
    REFERENCES cloud_region (uuid),
    CONSTRAINT fk_model_cloud_credential
    FOREIGN KEY (cloud_credential_uuid)
    REFERENCES cloud_credential (uuid),
    CONSTRAINT fk_model_model_type_id
    FOREIGN KEY (model_type_id)
    REFERENCES model_type (id),
    CONSTRAINT fk_model_owner_uuid
    FOREIGN KEY (owner_uuid)
    REFERENCES user (uuid),
    CONSTRAINT fk_model_life_id
    FOREIGN KEY (life_id)
    REFERENCES life (id)
);

-- idx_model_name_owner established an index that stops models being created
-- with the same name for a given owner.
CREATE UNIQUE INDEX idx_model_name_owner ON model (name, owner_uuid);
CREATE INDEX idx_model_activated ON model (activated);

-- v_model_all is a view that provides a simple way to access models
-- that have not been activated. This is useful for the model creation process
-- where we need to access the model to update it but we do not want to show it
-- to the user until it is ready.
CREATE VIEW v_model_all AS
SELECT
    m.uuid,
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
    m.name,
    m.owner_uuid,
    o.name AS owner_name,
    l.value AS life,
    m.activated,
    -- Don't rely on controller_uuid always being set to a value.
    ctrli.uuid AS controller_uuid,
    IIF(ctrlm.model_uuid IS NOT NULL, TRUE, FALSE) AS is_controller_model
FROM model AS m
JOIN cloud AS c ON m.cloud_uuid = c.uuid
JOIN cloud_type AS ct ON c.cloud_type_id = ct.id
JOIN model_type AS mt ON m.model_type_id = mt.id
JOIN user AS o ON m.owner_uuid = o.uuid
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
    owner_uuid,
    owner_name,
    life,
    activated,
    controller_uuid,
    is_controller_model
FROM v_model_all
WHERE activated = TRUE;

-- v_model_state exists to provide a simple view over the states that are
-- needed to calculate a model's status.
CREATE VIEW v_model_state AS
SELECT
    -- TODO (tlm, JUJU-7230) Wire up the value of migrating when model migration
    -- information is contained in the database.
    FALSE AS migrating,
    m.uuid,
    cc.invalid AS cloud_credential_invalid,
    cc.invalid_reason AS cloud_credential_invalid_reason,
    IIF(l.id = 1, TRUE, FALSE) AS destroying
FROM model AS m
JOIN life AS l ON m.life_id = l.id
LEFT JOIN cloud_credential AS cc ON m.cloud_credential_uuid = cc.uuid
WHERE m.activated = TRUE;
