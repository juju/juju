// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import "strings"

// ControllerDDL is used to create the controller database schema at bootstrap.
func ControllerDDL() []string {
	delta := `
CREATE TABLE lease_type (
	id		INT PRIMARY KEY,
	type	TEXT
);

INSERT INTO lease_type VALUES
	(0, 'controller'), 
	(1, 'model' ), 
	(2, 'application');

CREATE TABLE IF NOT EXISTS lease (
	uuid			TEXT PRIMARY KEY,
	lease_type_id	INT NOT NULL,
	name			TEXT,
	holder			TEXT,
	start			TIMESTAMP,
	duration		INT,
	pinned			BOOLEAN,
	CONSTRAINT		fk_lease_lease_type
		FOREIGN KEY	(lease_type_id)
		REFERENCES	lease_type(id)
);

CREATE UNIQUE INDEX idx_lease_type_name
ON lease (lease_type_id, name);`[1:]

	return strings.Split(delta, ";\n\n")
}
