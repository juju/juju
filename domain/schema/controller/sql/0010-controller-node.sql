CREATE TABLE controller_node (
    controller_id TEXT NOT NULL PRIMARY KEY,
    life_id INT NOT NULL DEFAULT 0,
    dqlite_node_id TEXT, -- This is the uint64 from Dqlite NodeInfo, stored as text.
    CONSTRAINT fk_controller_node_life
    FOREIGN KEY (life_id)
    REFERENCES life (id)
);

CREATE UNIQUE INDEX idx_controller_node_dqlite_node
ON controller_node (dqlite_node_id);

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

CREATE INDEX idx_controller_node_agent_version_architecture
ON controller_node_agent_version (architecture_id);

-- api_address_agent contains general API endpoints for controller agents.
-- is_agent_only is false when an endpoint is also reachable by clients.
CREATE TABLE api_address_agent (
    address TEXT NOT NULL PRIMARY KEY,
    is_agent_only BOOLEAN NOT NULL,
    scope TEXT NOT NULL
);

-- api_address_client contains general API endpoints configured for clients.
CREATE TABLE api_address_client (
    address TEXT NOT NULL PRIMARY KEY,
    scope TEXT NOT NULL
);

-- api_address_agent_by_controller contains API endpoints for individual
-- controller nodes that are reachable by controller agents.
CREATE TABLE api_address_agent_by_controller (
    controller_id TEXT NOT NULL,
    -- The value of the configured IP address with the port appended.
    -- e.g. 192.168.1.2:17070 or [2001:db8:0000:0000:0000:0000:0000:00001]:17070.
    address TEXT NOT NULL,
    -- Represents the context an address may apply to. E.g. public, private.
    scope TXT NOT NULL,
    CONSTRAINT fk_api_address_agent_by_controller_controller
    FOREIGN KEY (controller_id)
    REFERENCES controller_node (controller_id),
    PRIMARY KEY (controller_id, address)
);

-- api_address_client_by_controller contains API endpoints for individual
-- controller nodes that are reachable by clients.
CREATE TABLE api_address_client_by_controller (
    controller_id TEXT NOT NULL,
    -- The value of the configured IP address with the port appended.
    -- e.g. 192.168.1.2:17070 or [2001:db8:0000:0000:0000:0000:0000:00001]:17070.
    address TEXT NOT NULL,
    -- Represents the context an address may apply to. E.g. public, private.
    scope TXT NOT NULL,
    CONSTRAINT fk_api_address_client_by_controller_controller
    FOREIGN KEY (controller_id)
    REFERENCES controller_node (controller_id),
    PRIMARY KEY (controller_id, address)
);

CREATE TABLE controller_node_password (
    controller_id TEXT NOT NULL PRIMARY KEY,
    password_hash_algorithm_id TEXT,
    password_hash TEXT,
    CONSTRAINT fk_controller_node_password_controller
    FOREIGN KEY (controller_id)
    REFERENCES controller_node (controller_id),
    CONSTRAINT fk_controller_node_password_hash_algorithm
    FOREIGN KEY (password_hash_algorithm_id)
    REFERENCES password_hash_algorithm (id)
);

-- controller_node_nonce stores a per-ordinal nonce used to authenticate a
-- controller pod during UnitIntroduction. Nonces are generated before the
-- StatefulSet is created and bound to a specific ordinal in the ConfigMap.
-- The correct nonce must be presented by the pod's init container to prove
-- it is the legitimate pod for that ordinal. The row is verified (not
-- consumed) on each introduction attempt. Idempotency is provided by the
-- password insert-if-absent guard, not by nonce consumption.
--
-- There is deliberately no foreign key to controller_node: this nonce must
-- exist before the controller pod can introduce itself and create that row.
CREATE TABLE controller_node_nonce (
    controller_id TEXT NOT NULL PRIMARY KEY,
    nonce TEXT NOT NULL
);
