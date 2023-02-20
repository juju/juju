// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

// ControllerDDL is used to create the controller database schema at bootstrap.
func ControllerDDL() []string {
	schemas := []func() string{
		leaseSchema,
		changeLogSchema,
		nodeSchema,
	}

	var deltas []string
	for _, fn := range schemas {
		deltas = append(deltas, fn())
	}

	return deltas
}

func leaseSchema() string {
	return `
CREATE TABLE lease_type (
    id   INT PRIMARY KEY,
    type TEXT
);

CREATE UNIQUE INDEX idx_lease_type_type
ON lease_type (type);

INSERT INTO lease_type VALUES
    (0, 'singular-controller'),    -- The controller running singular controller/model workers.
    (1, 'application-leadership'); -- The unit that holds leadership for an application.

CREATE TABLE lease (
    uuid            TEXT PRIMARY KEY,
    lease_type_id   INT NOT NULL,
    model_uuid      TEXT,
    name            TEXT,
    holder          TEXT,
    start           TIMESTAMP,
    expiry          TIMESTAMP,
    CONSTRAINT      fk_lease_lease_type
        FOREIGN KEY (lease_type_id)
        REFERENCES  lease_type(id)
);

CREATE UNIQUE INDEX idx_lease_model_type_name
ON lease (model_uuid, lease_type_id, name);

CREATE INDEX idx_lease_expiry
ON lease (expiry);

CREATE TABLE lease_pin (
    -- The presence of entries in this table for a particular lease_uuid
    -- implies that the lease in question is pinned and cannot expire.
    uuid       TEXT PRIMARY KEY,
    lease_uuid TEXT,
    entity_id  TEXT,
    CONSTRAINT      fk_lease_pin_lease
        FOREIGN KEY (lease_uuid)
        REFERENCES  lease(uuid)
);

CREATE UNIQUE INDEX idx_lease_pin_lease_entity
ON lease_pin (lease_uuid, entity_id);

CREATE INDEX idx_lease_pin_lease
ON lease_pin (lease_uuid);
`[1:]
}

func changeLogSchema() string {
	return `
CREATE TABLE change_log_edit_type (
    id        INT PRIMARY KEY,
    edit_type TEXT
);

CREATE UNIQUE INDEX idx_change_log_edit_type_edit_type
ON change_log_edit_type (edit_type);

-- The change log type values are bitmasks, so that multiple types can be
-- expressed when looking for changes.
INSERT INTO change_log_edit_type VALUES
    (1, 'create'),
    (2, 'update'),
    (4, 'delete');

CREATE TABLE change_log_namespace (
    id        INT PRIMARY KEY,
    namespace TEXT
);

CREATE UNIQUE INDEX idx_change_log_namespace_namespace
ON change_log_namespace (namespace);

INSERT INTO change_log_namespace VALUES
    (1, 'node');

CREATE TABLE change_log (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    edit_type_id        INT NOT NULL,
    namespace_id        INT NOT NULL,
    changed_uuid        TEXT NOT NULL,
    created_at          DATETIME NOT NULL DEFAULT(STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc')),
    CONSTRAINT          fk_change_log_edit_type
            FOREIGN KEY (edit_type_id)
            REFERENCES  change_log_edit_type(id),
    CONSTRAINT          fk_change_log_namespace
            FOREIGN KEY (namespace_id)
            REFERENCES  change_log_namespace(id)
);`[1:]
}

func nodeSchema() string {
	return `
CREATE TABLE node (
    controller_id  TEXT PRIMARY KEY, 
    dqlite_node_id INT,               -- This is the uint64 from Dqlite NodeInfo.
    bind_address   TEXT               -- IP address (no port) that Dqlite is bound to. 
);

CREATE UNIQUE INDEX idx_node_dqlite_node
ON node (dqlite_node_id);

CREATE UNIQUE INDEX idx_node_bind_address
ON node (bind_address);

CREATE TRIGGER trg_changelog_node_insert
AFTER INSERT ON node FOR EACH ROW
BEGIN
	INSERT INTO change_log (edit_type_id, namespace_id, changed_uuid) VALUES (1, 1, NEW.controller_id);
END;

CREATE TRIGGER trg_changelog_node_update
AFTER UPDATE ON node FOR EACH ROW
BEGIN
	INSERT INTO change_log (edit_type_id, namespace_id, changed_uuid) VALUES (2, 1, OLD.controller_id);
END;

CREATE TRIGGER trg_changelog_node_delete
AFTER DELETE ON node FOR EACH ROW
BEGIN
	INSERT INTO change_log (edit_type_id, namespace_id, changed_uuid) VALUES (4, 1, OLD.controller_id);
END;`[1:]
}
