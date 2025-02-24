CREATE TABLE controller_node (
    controller_id TEXT NOT NULL PRIMARY KEY,
    dqlite_node_id TEXT,              -- This is the uint64 from Dqlite NodeInfo, stored as text.
    bind_address TEXT               -- IP address (no port) that Dqlite is bound to. 
);

CREATE UNIQUE INDEX idx_controller_node_dqlite_node
ON controller_node (dqlite_node_id);

CREATE UNIQUE INDEX idx_controller_node_bind_address
ON controller_node (bind_address);
