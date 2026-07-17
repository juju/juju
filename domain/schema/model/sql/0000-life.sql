CREATE TABLE life (
    id INT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_life_value_id
ON life (value, id);

INSERT INTO life VALUES
(0, 'alive'),
(1, 'dying'),
(2, 'dead');
