-- The agent_binary_store table stores information about agent binaries stored in the model's object store,
-- including their version, SHA, architecture, and object store information.
CREATE TABLE agent_binary_store (
    version TEXT NOT NULL,
    architecture_id INT NOT NULL,
    object_store_uuid TEXT NOT NULL,
    PRIMARY KEY (version, architecture_id),
    CONSTRAINT fk_agent_binary_metadata_object_store_metadata
    FOREIGN KEY (object_store_uuid)
    REFERENCES object_store_metadata (uuid),
    CONSTRAINT fk_agent_binary_metadata_architecture
    FOREIGN KEY (architecture_id)
    REFERENCES architecture (id)
);

CREATE VIEW v_agent_binary_store AS
SELECT
    abs.version,
    abs.object_store_uuid,
    abs.architecture_id,
    a.name AS architecture_name,
    osm.size,
    osm.sha_256,
    osm.sha_384,
    osmp.path
FROM agent_binary_store AS abs
JOIN architecture AS a ON abs.architecture_id = a.id
JOIN object_store_metadata AS osm ON abs.object_store_uuid = osm.uuid
JOIN object_store_metadata_path AS osmp ON osm.uuid = osmp.metadata_uuid;
