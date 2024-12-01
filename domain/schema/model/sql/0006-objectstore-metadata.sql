CREATE TABLE object_store_metadata (
    uuid TEXT NOT NULL PRIMARY KEY,
    hash_256 TEXT NOT NULL,
    hash_512_384 TEXT NOT NULL,
    size INT NOT NULL
);

-- Add a unique index for each hash and a composite unique index for both hashes
-- to ensure that the same hash is not stored multiple times.
CREATE UNIQUE INDEX idx_object_store_metadata_hash_256 ON object_store_metadata (hash_256);
CREATE UNIQUE INDEX idx_object_store_metadata_hash_512_384 ON object_store_metadata (hash_512_384);
CREATE UNIQUE INDEX idx_object_store_metadata_hash ON object_store_metadata (hash_256, hash_512_384);

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
    osm.hash_256,
    osm.hash_512_384,
    osm.size,
    osmp.path
FROM object_store_metadata AS osm
LEFT JOIN object_store_metadata_path AS osmp
    ON osm.uuid = osmp.metadata_uuid;
