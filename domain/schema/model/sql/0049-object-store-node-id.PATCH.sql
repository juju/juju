CREATE TABLE object_store_placement (
    uuid TEXT NOT NULL PRIMARY KEY,
    node_id TEXT NOT NULL,
    CONSTRAINT fk_object_store_placement_uuid
    FOREIGN KEY (uuid)
    REFERENCES object_store_metadata (uuid)
);
