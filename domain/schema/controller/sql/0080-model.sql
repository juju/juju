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
    qualifier TEXT NOT NULL,
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
    CONSTRAINT fk_model_life_id
    FOREIGN KEY (life_id)
    REFERENCES life (id)
);

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

-- idx_model_qualified_name established an index that stops models being created
-- with the same qualified name.
CREATE UNIQUE INDEX idx_model_qualified_name ON model (name, qualifier);
CREATE INDEX idx_model_activated ON model (activated);
CREATE INDEX idx_model_life ON model (life_id);

-- v_cloud is used to fetch well-constructed information about a cloud.
-- This view also includes information on whether the cloud is the
-- controller model's cloud.
CREATE VIEW v_cloud
AS
-- This selects the controller model's cloud uuid. We use this when loading
-- clouds to know if the cloud is the controller's cloud.
WITH controllers AS (
    SELECT m.cloud_uuid
    FROM model AS m
    WHERE
        m.name = 'controller'
        AND m.qualifier = 'admin'
        AND m.activated = TRUE
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
