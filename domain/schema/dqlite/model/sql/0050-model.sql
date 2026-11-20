-- The model_config table is a new table that is used to store configuration
-- data for the model.
--
-- The provider tracker relies on the model_config table. Do not modify the
-- model_config table in a patch/build release. Only make changes to this table
-- during a major/minor release.
CREATE TABLE model_config (
    "key" TEXT NOT NULL PRIMARY KEY,
    value TEXT NOT NULL
);

-- This table is best effort to track the life of a model. The real location
-- of the model life is in the controller database. This is just a facsimile
-- of the model life.
CREATE TABLE model_life (
    model_uuid TEXT NOT NULL PRIMARY KEY,
    life_id INT NOT NULL,
    CONSTRAINT fk_model_constraint_model
    FOREIGN KEY (model_uuid)
    REFERENCES model (uuid),
    CONSTRAINT fk_model_life
    FOREIGN KEY (life_id)
    REFERENCES life (id)
);

CREATE TABLE model_migrating (
    uuid TEXT NOT NULL PRIMARY KEY,
    model_uuid TEXT NOT NULL,
    CONSTRAINT fk_model_migrating_model
    FOREIGN KEY (model_uuid)
    REFERENCES model (uuid)
);

CREATE UNIQUE INDEX idx_model_migrating_model_uuid ON model_migrating (model_uuid);

-- container_type represents the valid container types that can exist for an
-- instance.
CREATE TABLE container_type (
    id INT PRIMARY KEY,
    value TEXT NOT NULL
);

-- The allocate_public_ip column shold have been a (nullable) boolean, but
-- there is currently a bug somewhere between dqlite and go-dqlite that returns
-- a false boolean instead a null. Since the driver will correctly map a INT
-- to a boolean, then we can safely use it here as a workaround.
CREATE TABLE "constraint" (
    uuid TEXT NOT NULL PRIMARY KEY,
    arch TEXT,
    cpu_cores INT,
    cpu_power INT,
    mem INT,
    root_disk INT,
    root_disk_source TEXT,
    instance_role TEXT,
    instance_type TEXT,
    container_type_id INT,
    virt_type TEXT,
    -- allocate_public_ip is a bool value. We only use int to get around DQlite
    -- limitations with NULL bools.
    allocate_public_ip INT,
    image_id TEXT,
    ip_family TEXT,
    CONSTRAINT fk_constraint_container_type
    FOREIGN KEY (container_type_id)
    REFERENCES container_type (id)
) STRICT;

-- The model_constraint table is a new table that is used to store the
-- constraints that are associated with a model.
CREATE TABLE model_constraint (
    model_uuid TEXT NOT NULL PRIMARY KEY,
    constraint_uuid TEXT NOT NULL,
    CONSTRAINT fk_model_constraint_model
    FOREIGN KEY (model_uuid)
    REFERENCES model (uuid),
    CONSTRAINT fk_model_constraint_constraint
    FOREIGN KEY (constraint_uuid)
    REFERENCES "constraint" (uuid)
);
