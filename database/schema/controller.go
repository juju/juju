// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import "strings"

// ControllerDDL is used to create the controller database schema at bootstrap.
func ControllerDDL() []string {
	delta := `
CREATE TABLE lease_type (
    id   INT PRIMARY KEY,
    type TEXT
);

CREATE UNIQUE INDEX idx_lease_type_type
ON lease_type (type);

INSERT INTO lease_type VALUES
    (0, 'controller'),  -- The controller running singular controller workers.
    (1, 'model' ),      -- The controller running singular workers for a model.
    (2, 'application'); -- The unit that holds leadership for an application.

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

CREATE INDEX idx_lease_pin_lease
ON lease_pin (lease_uuid);

`[1:]

	return strings.Split(delta, ";\n\n")
}
