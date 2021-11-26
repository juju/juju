// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"database/sql"

	"github.com/juju/errors"
)

var (
	schemas = map[string]string{
		"logs": `
CREATE TABLE IF NOT EXISTS logs(
	ts DATETIME,
	entity TEXT,
	version TEXT,
	module TEXT,
	location TEXT,
	level INTEGER,
	message TEXT,
	labels TEXT
);
`,
		// default schema
		"_default": ``,
	}
)

func ensureDBSchema(dbName string, db *sql.DB) error {
	// Check for a schema override for the provided DB name or fall back to
	// the default schema.
	schema, found := schemas[dbName]
	if !found {
		schema = schemas["_default"]
	}

	_, err := db.Exec(schema)
	return errors.Annotate(err, "ensuring schema is up to date")
}
