CREATE TABLE application (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL,
    life_id INT NOT NULL,
    -- charm_uuid should not be nullable, but we need to allow it for now
    -- whilst we're wiring up the model.
    charm_uuid TEXT,
    charm_modified_version INT,
    charm_upgrade_on_error BOOLEAN DEFAULT FALSE,
    exposed BOOLEAN DEFAULT FALSE,
    placement TEXT,
    password_hash_algorithm_id TEXT,
    password_hash TEXT,
    CONSTRAINT fk_application_life
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_application_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    CONSTRAINT fk_application_password_hash_algorithm
    FOREIGN KEY (password_hash_algorithm_id)
    REFERENCES password_hash_algorithm (id)
);

CREATE UNIQUE INDEX idx_application_name
ON application (name);

CREATE TABLE application_channel (
    application_uuid TEXT NOT NULL PRIMARY KEY,
    track TEXT NOT NULL,
    risk TEXT NOT NULL,
    branch TEXT,
    CONSTRAINT fk_application_channel_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid)
);

-- Application scale is currently only targeting k8s applications.
CREATE TABLE application_scale (
    application_uuid TEXT NOT NULL PRIMARY KEY,
    scale INT,
    scale_target TEXT,
    scaling BOOLEAN DEFAULT FALSE,
    CONSTRAINT fk_application_endpoint_space_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid)
);

CREATE TABLE application_platform (
    application_uuid TEXT NOT NULL PRIMARY KEY,
    os_id TEXT,
    channel TEXT,
    architecture_id TEXT,
    CONSTRAINT fk_application_platform_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_application_platform_os
    FOREIGN KEY (os_id)
    REFERENCES os (id),
    CONSTRAINT fk_application_platform_architecture
    FOREIGN KEY (architecture_id)
    REFERENCES architecture (id)
);

CREATE TABLE application_endpoint_space (
    application_uuid TEXT NOT NULL,
    space_uuid TEXT,
    CONSTRAINT fk_application_endpoint_space_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_application_endpoint_space_space
    FOREIGN KEY (space_uuid)
    REFERENCES space (uuid),
    PRIMARY KEY (application_uuid, space_uuid)
);

CREATE TABLE application_endpoint_cidr (
    application_uuid TEXT NOT NULL,
    cidr TEXT,
    CONSTRAINT fk_application_endpoint_space_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    PRIMARY KEY (application_uuid, cidr)
);

CREATE TABLE application_config (
    application_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    type_id TEXT,
    value TEXT,
    CONSTRAINT fk_application_config_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_application_config_charm_config_type
    FOREIGN KEY (type_id)
    REFERENCES charm_config_type (id),
    PRIMARY KEY (application_uuid, name)
);

CREATE TABLE application_constraint (
    application_uuid TEXT NOT NULL PRIMARY KEY,
    constraint_uuid TEXT,
    CONSTRAINT fk_application_constraint_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_application_constraint_constraint
    FOREIGN KEY (constraint_uuid)
    REFERENCES "constraint" (uuid)
);

CREATE TABLE application_setting (
    application_uuid TEXT NOT NULL PRIMARY KEY,
    trust BOOLEAN DEFAULT FALSE,
    CONSTRAINT fk_application_setting_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid)
);
