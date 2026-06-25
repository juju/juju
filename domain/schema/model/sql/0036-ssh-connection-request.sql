CREATE TABLE ssh_connection_request (
    tunnel_id TEXT NOT NULL PRIMARY KEY,
    machine_uuid TEXT NOT NULL,
    expires_at DATETIME NOT NULL,
    username TEXT NOT NULL,
    password TEXT NOT NULL,
    unit_port INT NOT NULL,
    ephemeral_public_key BLOB NOT NULL,
    CONSTRAINT fk_ssh_connection_request_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid)
);

CREATE TABLE ssh_connection_request_address (
    tunnel_id TEXT NOT NULL,
    index_id INT NOT NULL,
    address_value TEXT NOT NULL,
    PRIMARY KEY (tunnel_id, index_id),
    CONSTRAINT fk_ssh_connection_request_address_request
    FOREIGN KEY (tunnel_id)
    REFERENCES ssh_connection_request (tunnel_id)
);

CREATE INDEX idx_ssh_connection_request_machine_uuid
ON ssh_connection_request (machine_uuid);

CREATE INDEX idx_ssh_connection_request_expires_at
ON ssh_connection_request (expires_at);
