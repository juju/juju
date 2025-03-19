-- The agent_binary_metadata table stores information about agent binaries stored in the model's object store,
-- including their version, SHA, architecture, and object store information, sourced from the
-- simple stream in the model database.
CREATE TABLE agent_binary_metadata (
    uuid TEXT NOT NULL PRIMARY KEY,
    version TEXT NOT NULL,
    object_store_uuid TEXT NOT NULL,
    architecture_id INT NOT NULL,
    CONSTRAINT fk_agent_binary_metadata_object_store_metadata
    FOREIGN KEY (object_store_uuid)
    REFERENCES object_store_metadata (uuid),
    CONSTRAINT fk_agent_binary_metadata_architecture
    FOREIGN KEY (architecture_id)
    REFERENCES architecture (id)
);

CREATE UNIQUE INDEX idx_agent_binary_metadata_version_architecture
ON agent_binary_metadata (version, architecture_id);
