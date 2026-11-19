CREATE TABLE ssh_fingerprint_hash_algorithm (
    id INT PRIMARY KEY,
    algorithm TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_ssh_fingerprint_hash_algorithm_algorithm
ON ssh_fingerprint_hash_algorithm (algorithm);

INSERT INTO ssh_fingerprint_hash_algorithm VALUES
(0, 'md5'),
(1, 'sha256');

CREATE TABLE user_public_ssh_key (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    -- comment is the comment string set in the public key. We also retain this
    -- value in public_key. This column exists to make an index for deletion.
    comment TEXT NOT NULL,
    fingerprint_hash_algorithm_id INT NOT NULL,
    fingerprint TEXT NOT NULL,
    public_key TEXT NOT NULL,
    user_uuid TEXT NOT NULL,
    FOREIGN KEY (fingerprint_hash_algorithm_id)
    REFERENCES ssh_fingerprint_hash_algorithm (id),
    FOREIGN KEY (user_uuid)
    REFERENCES user (uuid)
);

CREATE UNIQUE INDEX idx_user_public_ssh_key_user_fingerprint
ON user_public_ssh_key (user_uuid, fingerprint);

CREATE UNIQUE INDEX idx_user_public_ssh_key_user_public_key
ON user_public_ssh_key (user_uuid, public_key);

CREATE INDEX idx_user_public_ssh_key_user_comment
ON user_public_ssh_key (user_uuid, comment);
