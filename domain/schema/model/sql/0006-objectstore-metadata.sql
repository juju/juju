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
