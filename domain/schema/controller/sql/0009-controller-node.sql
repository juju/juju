CREATE TABLE controller_node (
    controller_id TEXT NOT NULL PRIMARY KEY,
    dqlite_node_id TEXT,              -- This is the uint64 from Dqlite NodeInfo, stored as text.
    bind_address TEXT               -- IP address (no port) that Dqlite is bound to.
);

CREATE UNIQUE INDEX idx_controller_node_dqlite_node
ON controller_node (dqlite_node_id);

CREATE UNIQUE INDEX idx_controller_node_bind_address
ON controller_node (bind_address);

-- controller_node_agent_version tracks the reported agent version running for
-- each controller in the cluster.
CREATE TABLE controller_node_agent_version (
    controller_id TEXT NOT NULL PRIMARY KEY,
    version TEXT NOT NULL,
    architecture_id INT NOT NULL,
    CONSTRAINT fk_controller_node_agent_version_controller
    FOREIGN KEY (controller_id)
    REFERENCES controller_node (controller_id),
    CONSTRAINT fk_controller_node_agent_version_architecture
    FOREIGN KEY (architecture_id)
    REFERENCES architecture (id)
);
