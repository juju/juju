CREATE TABLE application (
    uuid TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    life_id INT NOT NULL,
    CONSTRAINT fk_application_life
    FOREIGN KEY (life_id)
    REFERENCES life (id)
) STRICT;

CREATE UNIQUE INDEX idx_application_name
ON application (name);
