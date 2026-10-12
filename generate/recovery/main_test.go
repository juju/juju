// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"database/sql"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/schema"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/logger"
)

type generatorSuite struct{}

func TestGeneratorSuite(t *testing.T) { tc.Run(t, &generatorSuite{}) }

func (s *generatorSuite) TestInspectionUsesAppliedSchema(c *tc.C) {
	db, err := sql.Open("sqlite3", ":memory:")
	c.Assert(err, tc.ErrorIsNil)
	c.Cleanup(func() { c.Check(db.Close(), tc.ErrorIsNil) })
	_, err = db.Exec(`CREATE TABLE sample (
uuid TEXT NOT NULL,
enabled BOOLEAN DEFAULT FALSE,
created TIMESTAMP,
data BLOB, id INTEGER PRIMARY KEY AUTOINCREMENT)`)
	c.Assert(err, tc.ErrorIsNil)
	tables, err := inspect(c.Context(), db, "Model")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(tables, tc.HasLen, 1)
	c.Check(tables[0].Name, tc.Equals, "sample")
	c.Check(tables[0].Struct, tc.Equals, "ModelSample")
	c.Check(tables[0].Columns, tc.DeepEquals, []column{
		{Name: "uuid", Field: "UUID", Type: "string"},
		{Name: "enabled", Field: "Enabled", Type: "bool", HasDefault: true, Default: "FALSE"},
		{Name: "created", Field: "Created", Type: "time.Time"},
		{Name: "data", Field: "Data", Type: "[]byte"},
		{Name: "id", Field: "ID", Type: "int64"},
	})
}

func (s *generatorSuite) TestLegacyBinaryBindings(c *tc.C) {
	c.Check(bindingType("bakery_config", "local_users_public_key", "TEXT"), tc.Equals, "[]byte")
	c.Check(bindingType("macaroon_root_key", "id", "TEXT"), tc.Equals, "[]byte")
	c.Check(bindingType("macaroon_root_key", "root_key", "TEXT"), tc.Equals, "[]byte")
	c.Check(bindingType("controller", "uuid", "TEXT"), tc.Equals, "string")
	c.Check(bindingType("macaroon_root_key", "created_at", "TIMESTAMP"), tc.Equals, "time.Time")
}

func (s *generatorSuite) TestBinaryPoliciesMatchNativeSchemas(c *tc.C) {
	known := make(map[string]map[string]string)
	for _, ddl := range []database.Schema{
		schema.ControllerDDLForVersion(version.Current),
		schema.ModelDDLForVersion(version.Current),
	} {
		db, err := sql.Open("sqlite3", ":memory:")
		c.Assert(err, tc.ErrorIsNil)
		c.Cleanup(func() { c.Check(db.Close(), tc.ErrorIsNil) })
		c.Assert(database.NewDBMigration(&txnRunner{db: db}, logger.Noop(), ddl).Apply(c.Context()), tc.ErrorIsNil)
		tables, err := inspect(c.Context(), db, "Native")
		c.Assert(err, tc.ErrorIsNil)
		for _, table := range tables {
			columns := make(map[string]string)
			for _, column := range table.Columns {
				columns[column.Name] = column.Type
			}
			known[table.Name] = columns
		}
	}
	for table, columns := range binaryTextColumns {
		c.Assert(known[table], tc.NotNil, tc.Commentf("binary table %s", table))
		for _, column := range columns {
			c.Check(known[table][column], tc.Equals, "[]byte", tc.Commentf("binary column %s.%s", table, column))
		}
	}
}
