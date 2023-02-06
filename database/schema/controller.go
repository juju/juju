// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

// ControllerDDL is used to create the controller database schema at bootstrap.
func ControllerDDL() []string {
	schemas := []func() string{
		leaseSchema,
		changeLogSchema,
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
CREATE TABLE change_log_type (
    id   INT PRIMARY KEY,
    type TEXT
);

CREATE UNIQUE INDEX idx_change_log_type_type
ON change_log_type (type);

INSERT INTO change_log_type VALUES
    (1, 'create'),
    (2, 'update'),
    (4, 'delete');

CREATE TABLE change_log_namespace (
    id   INT PRIMARY KEY,
    name TEXT
);

CREATE UNIQUE INDEX idx_change_log_namespace_name
ON change_log_namespace (name);

CREATE TABLE change_log (
    uuid                    TEXT PRIMARY KEY,
    change_log_type_id      INT NOT NULL,
    change_log_namespace_id INT NOT NULL,
    namespace_id            TEXT NOT NULL,
    created_at              DATETIME NOT NULL,
    CONSTRAINT              fk_change_log_type
        FOREIGN KEY (change_log_type_id)
        REFERENCES  change_log_type(id),
    CONSTRAINT              fk_change_log_namespace
        FOREIGN KEY (change_log_namespace_id)
        REFERENCES  change_log_namespace(id)
);`[1:]
}
