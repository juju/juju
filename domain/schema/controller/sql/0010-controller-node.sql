CREATE TABLE controller_node (
    controller_id TEXT PRIMARY KEY,
    -- This is the uint64 from Dqlite NodeInfo, stored as text.
    dqlite_node_id TEXT,
    -- IP address (no port) that Dqlite is bound to.  
    bind_address TEXT
) STRICT;

CREATE UNIQUE INDEX idx_controller_node_dqlite_node
ON controller_node (dqlite_node_id);

CREATE UNIQUE INDEX idx_controller_node_bind_address
ON controller_node (bind_address);
