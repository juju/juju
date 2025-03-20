-- The agent_binary_metadata table in the controller database records information about
-- the agent binaries stored in the controller's object store, including their version, SHA,
-- architecture, and the object store reference.
-- This table tracks agent binaries available across the whole Juju controller
-- for use by any model.
CREATE TABLE agent_binary_store (
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
