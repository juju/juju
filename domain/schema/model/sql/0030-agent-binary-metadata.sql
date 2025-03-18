-- The agent_binary_metadata table stores information about cached agent binaries,
-- including their version, SHA, and object store location, sourced from the
-- simple stream in the model database.
CREATE TABLE agent_binary_metadata (
    uuid TEXT NOT NULL PRIMARY KEY,
    version TEXT NOT NULL,
    object_store_uuid TEXT NOT NULL,
    CONSTRAINT fk_agent_binary_metadata_object_store_metadata
    FOREIGN KEY (object_store_uuid)
    REFERENCES object_store_metadata (uuid)
);

CREATE UNIQUE INDEX idx_agent_binary_metadata_version
ON agent_binary_metadata (version);
