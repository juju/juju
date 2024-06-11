CREATE TABLE change_log_edit_type (
    id INT PRIMARY KEY,
    edit_type TEXT
) STRICT;

CREATE UNIQUE INDEX idx_change_log_edit_type_edit_type
ON change_log_edit_type (edit_type);

-- The change log type values are bitmasks, so that multiple types can be
-- expressed when looking for changes.
INSERT INTO change_log_edit_type VALUES
(1, 'create'),
(2, 'update'),
(4, 'delete');

CREATE TABLE change_log_namespace (
    id INT PRIMARY KEY,
    namespace TEXT,
    description TEXT
) STRICT;

CREATE UNIQUE INDEX idx_change_log_namespace_namespace
ON change_log_namespace (namespace);

CREATE TABLE change_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    edit_type_id INT NOT NULL,
    namespace_id INT NOT NULL,
    changed TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc')),
    CONSTRAINT fk_change_log_edit_type
    FOREIGN KEY (edit_type_id)
    REFERENCES change_log_edit_type (id),
    CONSTRAINT fk_change_log_namespace
    FOREIGN KEY (namespace_id)
    REFERENCES change_log_namespace (id)
);

-- The change log witness table is used to track which nodes have seen
-- which change log entries. This is used to determine when a change log entry
-- can be deleted.
-- We'll delete all change log entries that are older than the lower_bound
-- change log entry that has been seen by all controllers.
CREATE TABLE change_log_witness (
    controller_id TEXT PRIMARY KEY,
    lower_bound INT NOT NULL DEFAULT (-1),
    upper_bound INT NOT NULL DEFAULT (-1),
    updated_at TEXT NOT NULL DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc'))
) STRICT;
