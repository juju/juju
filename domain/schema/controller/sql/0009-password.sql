CREATE TABLE password_hash_algorithm (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_password_hash_algorithm
ON password_hash_algorithm (name);

INSERT INTO password_hash_algorithm VALUES
(0, 'sha512');
