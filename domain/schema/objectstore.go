// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import "github.com/juju/juju/core/database/schema"

// objectStoreMetadataSchema provides a helper function for generating a change_log ddl
// for a schema.
func objectStoreMetadataSchema() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE object_store_metadata_hash_type (
    id          INT PRIMARY KEY,
    hash_type   TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_object_store_metadata_hash_type_name
ON object_store_metadata_hash_type (hash_type);

INSERT INTO object_store_metadata_hash_type VALUES
    (0, 'none'),
    (1, 'sha-256');

CREATE TABLE object_store_metadata (
    uuid            TEXT PRIMARY KEY,
    key             TEXT NOT NULL,
    hash_type_id    INT NOT NULL,
    hash            TEXT,
    CONSTRAINT      fk_object_store_metadata_hash_type
        FOREIGN KEY (hash_type_id)
        REFERENCES  object_store_metadata_hash_type(id)
);

-- Until we proliferate juju with uuids everywhere, we need to allow for
-- accessing the metadata by key.
CREATE UNIQUE INDEX idx_object_store_metadata_key ON object_store_metadata (key);
CREATE UNIQUE INDEX idx_object_store_metadata_hash ON object_store_metadata (hash);

CREATE TABLE object_store_metadata_path (
    uuid            TEXT PRIMARY KEY,
    metadata_uuid   TEXT NOT NULL,
    path            TEXT NOT NULL,
    size            INT NOT NULL,
    CONSTRAINT      fk_object_store_metadata_metadata_uuid
        FOREIGN KEY (metadata_uuid)
        REFERENCES  object_store_metadata(uuid)
);

CREATE UNIQUE INDEX idx_object_store_metadata_path ON object_store_metadata_path (path);
`)
}
