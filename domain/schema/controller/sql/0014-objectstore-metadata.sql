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
    CONSTRAINT fk_object_store_drain_info_object_store_drain_phase_type
    FOREIGN KEY (phase_type_id)
    REFERENCES object_store_drain_phase_type (id)
);

-- A unique constraint over a constant index ensures only 1 entry matching the 
-- condition can exist. This states, that multiple draining can exist if they're
-- not active, but only one active drain can exist.
CREATE UNIQUE INDEX idx_singleton_active_drain ON object_store_drain_info ((1)) WHERE phase_type_id < 2;
