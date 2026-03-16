CREATE TABLE object_store_metadata (
    uuid TEXT NOT NULL PRIMARY KEY,
    sha_256 TEXT NOT NULL,
    sha_384 TEXT NOT NULL,
    size INT NOT NULL
);

-- Add a unique index for each hash and a composite unique index for both hashes
-- to ensure that the same hash is not stored multiple times.
CREATE UNIQUE INDEX idx_object_store_metadata_sha_256 ON object_store_metadata (sha_256);
CREATE UNIQUE INDEX idx_object_store_metadata_sha_384 ON object_store_metadata (sha_384);

CREATE TABLE object_store_metadata_path (
    path TEXT NOT NULL PRIMARY KEY,
    metadata_uuid TEXT NOT NULL,
    CONSTRAINT fk_object_store_metadata_metadata_uuid
    FOREIGN KEY (metadata_uuid)
    REFERENCES object_store_metadata (uuid)
);

CREATE VIEW v_object_store_metadata AS
SELECT
    osm.uuid,
    osm.sha_256,
    osm.sha_384,
    osm.size,
    osmp.path
FROM object_store_metadata AS osm
LEFT JOIN object_store_metadata_path AS osmp
    ON osm.uuid = osmp.metadata_uuid;

CREATE TABLE object_store_placement (
    uuid TEXT NOT NULL,
    node_id TEXT NOT NULL,
    CONSTRAINT fk_object_store_placement_uuid
    FOREIGN KEY (uuid)
    REFERENCES object_store_metadata (uuid),
    PRIMARY KEY (uuid, node_id)
);

CREATE TABLE object_store_backend_type (
    id INT NOT NULL PRIMARY KEY,
    type TEXT NOT NULL UNIQUE
);

INSERT INTO object_store_backend_type (id, type) VALUES
(0, 'file'),
(1, 's3');

CREATE TABLE object_store_backend (
    uuid TEXT NOT NULL PRIMARY KEY,
    life_id INT NOT NULL,
    type_id INT NOT NULL,
    CONSTRAINT fk_object_store_backend_life_id
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_object_store_backend_type_id
    FOREIGN KEY (type_id)
    REFERENCES object_store_backend_type (id)
);

INSERT INTO object_store_backend (uuid, life_id, type_id) VALUES
('f44ea516-22ad-4161-b2bd-cbae9d7a9412', 0, 0);

-- A unique constraint over a constant index ensures only 1 entry matching the
-- condition can exist. In this case only 1 object store backend of type file
-- can exist, but multiple s3 backends can exist.
CREATE UNIQUE INDEX idx_singleton_object_store_backend ON object_store_backend ((1)) WHERE type_id = 0;

-- This index ensures only 1 object store backend can exist with a life_id of 0,
-- which is the life_id used for the file backend. This ensures only 1 file
-- backend can exist at a time.
CREATE UNIQUE INDEX idx_object_store_backend_life_id ON object_store_backend (life_id) WHERE life_id = 0;

CREATE TABLE object_store_backend_s3_credential (
    object_store_backend_uuid TEXT NOT NULL PRIMARY KEY,
    endpoint TEXT NOT NULL,
    static_key TEXT NOT NULL,
    static_secret TEXT NOT NULL,
    session_token TEXT,
    CONSTRAINT fk_object_store_backend_uuid_s3_credential_object_store_uuid
    FOREIGN KEY (object_store_backend_uuid)
    REFERENCES object_store_backend (uuid)
);

CREATE TABLE object_store_drain_phase_type (
    id INT PRIMARY KEY,
    type TEXT
);

CREATE UNIQUE INDEX idx_object_store_drain_phase_type_type
ON object_store_drain_phase_type (type);

INSERT INTO object_store_drain_phase_type VALUES
(0, 'unknown'),
(1, 'draining'),
(2, 'error'),
(3, 'completed');

CREATE TABLE object_store_drain_info (
    uuid TEXT NOT NULL PRIMARY KEY,
    phase_type_id INT NOT NULL,
    from_backend_uuid TEXT,
    to_backend_uuid TEXT NOT NULL,
    CONSTRAINT fk_object_store_drain_info_object_store_drain_phase_type
    FOREIGN KEY (phase_type_id)
    REFERENCES object_store_drain_phase_type (id),
    CONSTRAINT fk_object_store_drain_info_from_object_store_backend_uuid
    FOREIGN KEY (from_backend_uuid)
    REFERENCES object_store_backend (uuid),
    CONSTRAINT fk_object_store_drain_info_to_object_store_backend_uuid
    FOREIGN KEY (to_backend_uuid)
    REFERENCES object_store_backend (uuid)
);

-- A unique constraint over a constant index ensures only 1 entry matching the 
-- condition can exist. This states, that multiple draining can exist if they're
-- not active, but only one active drain can exist.
CREATE UNIQUE INDEX idx_singleton_active_drain ON object_store_drain_info ((1)) WHERE phase_type_id < 2;
