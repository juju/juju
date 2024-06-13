CREATE TABLE space (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_spaces_uuid_name
ON space (name);

CREATE TABLE provider_space (
    provider_id TEXT NOT NULL PRIMARY KEY,
    space_uuid TEXT NOT NULL,
    CONSTRAINT fk_provider_space_space_uuid
    FOREIGN KEY (space_uuid)
    REFERENCES space (uuid)
);

CREATE UNIQUE INDEX idx_provider_space_space_uuid
ON provider_space (space_uuid);

INSERT INTO space VALUES
(0, 'alpha');
