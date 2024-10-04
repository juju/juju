CREATE TABLE block_command_type (
    id INT PRIMARY KEY,
    name_type TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_block_command_type_name
ON block_command_type (name_type);

INSERT INTO block_command_type VALUES
(0, 'destroy'),
(1, 'remove'),
(2, 'change');

CREATE TABLE block_command (
    uuid TEXT PRIMARY KEY,
    block_command_type_id INT NOT NULL,
    message TEXT,
    CONSTRAINT fk_block_command_type
    FOREIGN KEY (block_command_type_id)
    REFERENCES block_command_type (id)
);

CREATE UNIQUE INDEX idx_block_command_type
ON block_command (block_command_type_id);
