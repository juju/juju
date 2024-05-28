CREATE TABLE object_store_metadata_hash_type (
    id          INT PRIMARY KEY,
    hash_type   TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_object_store_metadata_hash_type_name
ON object_store_metadata_hash_type (hash_type);

INSERT INTO object_store_metadata_hash_type VALUES
    (0, 'none'),
    (1, 'sha512-384');

CREATE TABLE object_store_metadata (
    uuid            TEXT PRIMARY KEY,
    hash_type_id    INT NOT NULL,
    hash            TEXT NOT NULL,
    size            INT NOT NULL,
    CONSTRAINT      fk_object_store_metadata_hash_type
        FOREIGN KEY (hash_type_id)
        REFERENCES  object_store_metadata_hash_type(id)
);

CREATE UNIQUE INDEX idx_object_store_metadata_hash ON object_store_metadata (hash);

CREATE TABLE object_store_metadata_path (
    path            TEXT PRIMARY KEY,
    metadata_uuid   TEXT NOT NULL,
    CONSTRAINT      fk_object_store_metadata_metadata_uuid
        FOREIGN KEY (metadata_uuid)
        REFERENCES  object_store_metadata(uuid)
);
