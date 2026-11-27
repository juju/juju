-- generation identifies an in-flight generation. The uuid is the stable
-- identity used by lifecycle operations, generation_id is a human-facing
-- identifier allocated from the sequence table (namespace "generation",
-- generation.GenerationSequenceNamespace). generation_id is unique
-- across every generation the model ever had.
CREATE TABLE generation (
    uuid TEXT NOT NULL PRIMARY KEY,
    generation_id INT NOT NULL,
    name TEXT NOT NULL,
    created_by_uuid TEXT NOT NULL,
    created_at DATETIME NOT NULL
);

CREATE UNIQUE INDEX idx_generation_generation_id
ON generation (generation_id);

CREATE UNIQUE INDEX idx_generation_name
ON generation (name);

-- generation_tracking_mode is the tracking granularity of an associated
-- application. "unit" tracks only the units with rows in
-- generation_unit, "application" tracks the whole application, including
-- units added later, and keeps no per-unit rows.
CREATE TABLE generation_tracking_mode (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

INSERT INTO generation_tracking_mode VALUES
(0, 'unit'),
(1, 'application');

CREATE UNIQUE INDEX idx_generation_tracking_mode_name
ON generation_tracking_mode (name);

-- generation_application records the exclusive association between an
-- in-flight generation and an application.
CREATE TABLE generation_application (
    generation_uuid TEXT NOT NULL,
    application_uuid TEXT NOT NULL,
    tracking_mode_id INT NOT NULL,
    CONSTRAINT fk_generation_application_generation
    FOREIGN KEY (generation_uuid)
    REFERENCES generation (uuid),
    CONSTRAINT fk_generation_application_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_generation_application_tracking_mode
    FOREIGN KEY (tracking_mode_id)
    REFERENCES generation_tracking_mode (id),
    PRIMARY KEY (generation_uuid, application_uuid)
);

CREATE UNIQUE INDEX idx_generation_application_application_uuid
ON generation_application (application_uuid);

-- generation_unit records the units that individually track a
-- generation.
CREATE TABLE generation_unit (
    generation_uuid TEXT NOT NULL,
    application_uuid TEXT NOT NULL,
    unit_uuid TEXT NOT NULL,
    CONSTRAINT fk_generation_unit_application
    FOREIGN KEY (generation_uuid, application_uuid)
    REFERENCES generation_application (generation_uuid, application_uuid),
    CONSTRAINT fk_generation_unit_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    PRIMARY KEY (generation_uuid, application_uuid, unit_uuid)
);

CREATE UNIQUE INDEX idx_generation_unit_unit_uuid
ON generation_unit (unit_uuid);

-- committed_generation is the immutable history of committed
-- generations. Commit removes the in-flight generation row in
-- the same transaction that writes this one.
CREATE TABLE committed_generation (
    generation_uuid TEXT NOT NULL PRIMARY KEY,
    generation_id INT NOT NULL,
    name TEXT NOT NULL,
    created_by_uuid TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    committed_by_uuid TEXT NOT NULL,
    committed_at DATETIME NOT NULL
);

CREATE UNIQUE INDEX idx_committed_generation_generation_id
ON committed_generation (generation_id);

-- committed_generation_application is the application of a
-- committed generation. It freezes the application UUID and name at
-- commit time, so history stays readable after the application is
-- removed or renamed.
CREATE TABLE committed_generation_application (
    generation_uuid TEXT NOT NULL,
    application_uuid TEXT NOT NULL,
    application_name TEXT NOT NULL,
    CONSTRAINT fk_committed_generation_application_generation
    FOREIGN KEY (generation_uuid)
    REFERENCES committed_generation (generation_uuid),
    PRIMARY KEY (generation_uuid, application_uuid)
);

-- committed_generation_config stores the config deltas of a committed
-- generation, frozen at commit time.
CREATE TABLE committed_generation_config (
    generation_uuid TEXT NOT NULL,
    application_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    previous_type_id INT,
    previous_value TEXT,
    type_id INT NOT NULL,
    value TEXT,
    CONSTRAINT fk_committed_generation_config_application
    FOREIGN KEY (generation_uuid, application_uuid)
    REFERENCES committed_generation_application (generation_uuid, application_uuid),
    CONSTRAINT fk_committed_generation_config_type
    FOREIGN KEY (type_id)
    REFERENCES charm_config_type (id),
    CONSTRAINT fk_committed_generation_config_previous_type
    FOREIGN KEY (previous_type_id)
    REFERENCES charm_config_type (id),
    PRIMARY KEY (generation_uuid, application_uuid, "key")
);

-- committed_generation_charm stores the charm deltas of a committed
-- generation, frozen at commit time.
CREATE TABLE committed_generation_charm (
    generation_uuid TEXT NOT NULL,
    application_uuid TEXT NOT NULL,
    previous_source_id INT NOT NULL,
    previous_reference_name TEXT NOT NULL,
    previous_revision INT NOT NULL,
    source_id INT NOT NULL,
    reference_name TEXT NOT NULL,
    revision INT NOT NULL,
    CONSTRAINT fk_committed_generation_charm_application
    FOREIGN KEY (generation_uuid, application_uuid)
    REFERENCES committed_generation_application (generation_uuid, application_uuid),
    CONSTRAINT fk_committed_generation_charm_source
    FOREIGN KEY (source_id)
    REFERENCES charm_source (id),
    CONSTRAINT fk_committed_generation_charm_previous_source
    FOREIGN KEY (previous_source_id)
    REFERENCES charm_source (id),
    PRIMARY KEY (generation_uuid, application_uuid)
);

-- committed_generation_resource stores the resource deltas of a
-- committed generation, frozen at commit time.
CREATE TABLE committed_generation_resource (
    generation_uuid TEXT NOT NULL,
    application_uuid TEXT NOT NULL,
    charm_resource_name TEXT NOT NULL,
    previous_origin_type_id INT,
    previous_revision INT,
    previous_sha384 TEXT,
    previous_size INT,
    origin_type_id INT NOT NULL,
    revision INT,
    sha384 TEXT,
    size INT,
    CONSTRAINT fk_committed_generation_resource_application
    FOREIGN KEY (generation_uuid, application_uuid)
    REFERENCES committed_generation_application (generation_uuid, application_uuid),
    CONSTRAINT fk_committed_generation_resource_origin_type
    FOREIGN KEY (origin_type_id)
    REFERENCES resource_origin_type (id),
    CONSTRAINT fk_committed_generation_resource_previous_origin_type
    FOREIGN KEY (previous_origin_type_id)
    REFERENCES resource_origin_type (id),
    PRIMARY KEY (generation_uuid, application_uuid, charm_resource_name)
);
