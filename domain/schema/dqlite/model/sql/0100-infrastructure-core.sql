CREATE TABLE net_node (
    uuid TEXT NOT NULL PRIMARY KEY
);

CREATE TABLE machine (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL,
    net_node_uuid TEXT NOT NULL,
    life_id INT NOT NULL,
    nonce TEXT,
    password_hash_algorithm_id INT,
    password_hash TEXT,
    force_destroyed BOOLEAN DEFAULT FALSE,
    agent_started_at DATETIME,
    hostname TEXT,
    keep_instance BOOLEAN,
    CONSTRAINT fk_machine_net_node
    FOREIGN KEY (net_node_uuid)
    REFERENCES net_node (uuid),
    CONSTRAINT fk_machine_life
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_machine_password_hash_algorithm
    FOREIGN KEY (password_hash_algorithm_id)
    REFERENCES password_hash_algorithm (id)
);
