CREATE TABLE removal_type (
    id INT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_removal_type_name
ON removal_type (name);

INSERT INTO removal_type VALUES
(0, 'relation'),
(1, 'unit'),
(2, 'application'),
(3, 'machine');

CREATE TABLE removal (
    uuid TEXT NOT NULL PRIMARY KEY,
    removal_type_id INT NOT NULL,
    entity_uuid TEXT NOT NULL,
    force INT NOT NULL DEFAULT 0,
    -- Indicates when the job should be actioned by the worker,
    -- allowing us to schedule removals in the future.
    scheduled_for DATETIME NOT NULL DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc')),
    -- JSON for free-form job argumentation.
    arg TEXT,
    CONSTRAINT fk_removal_type
    FOREIGN KEY (removal_type_id)
    REFERENCES removal_type (id)
);
