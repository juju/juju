CREATE TABLE ssh_key_algorithm_type (
    id INT PRIMARY KEY,
    type TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_ssh_key_algorithm_type_type
ON ssh_key_algorithm_type (type);

INSERT INTO ssh_key_algorithm_type VALUES
(0, 'ssh-rsa'),
(1, 'ecdsa-sha2-nistp256'),
(2, 'ssh-ed25519');

CREATE TABLE controller_ssh_host_key (
    id TEXT NOT NULL PRIMARY KEY,
    algorithm_type_id INT NOT NULL,
    ssh_key TEXT NOT NULL,
    public_key BLOB NOT NULL DEFAULT '',
    CONSTRAINT fk_controller_ssh_host_key_algorithm_type_id
    FOREIGN KEY (algorithm_type_id)
    REFERENCES ssh_key_algorithm_type (id)
);

CREATE UNIQUE INDEX idx_singleton_controller_ssh_host_key
ON controller_ssh_host_key ((1));

-- controller_ssh_server_port holds the port the controller's embedded SSH
-- jump server listens on. It is a singleton (only one row) and is owned by the
-- controller charm, which pushes changes via the control socket. The SSH
-- server worker watches this table and restarts on the configured port.
CREATE TABLE controller_ssh_server_port (
    port INT NOT NULL
);

CREATE UNIQUE INDEX idx_singleton_controller_ssh_server_port
ON controller_ssh_server_port ((1));
