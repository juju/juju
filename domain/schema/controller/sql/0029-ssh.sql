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
    CONSTRAINT fk_controller_ssh_host_key_algorithm_type_id
    FOREIGN KEY (algorithm_type_id)
    REFERENCES ssh_key_algorithm_type (id)
);

CREATE UNIQUE INDEX idx_singleton_controller_ssh_host_key
ON controller_ssh_host_key ((1));
