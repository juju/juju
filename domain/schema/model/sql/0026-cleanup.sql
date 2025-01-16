CREATE TABLE removal_type (
    id INT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_removal_type_name
ON removal_type (name);

INSERT INTO removal_type VALUES
(0, 'relation');

CREATE TABLE removal (
    uuid TEXT NOT NULL PRIMARY KEY,
    removal_type_id INT NOT NULL,
    entity_uuid TEXT NOT NULL,
    scheduled_for DATETIME NOT NULL DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc')),
    arg TEXT,
    CONSTRAINT fk_removal_type
    FOREIGN KEY (removal_type_id)
    REFERENCES removal_type (id)
);
