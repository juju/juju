// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"database/sql"
	"reflect"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/version"
	controllertypes "github.com/juju/juju/domain/export/types/controller/v4_1_0"
	modeltypes "github.com/juju/juju/domain/export/types/v4_1_0"
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
	tables, err := inspect(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(tables, tc.HasLen, 1)
	c.Check(tables[0].Name, tc.Equals, "sample")
	c.Check(tables[0].Struct, tc.Equals, "Sample")
	c.Check(tables[0].Columns, tc.DeepEquals, []column{
		{Name: "uuid", Field: "UUID"},
		{Name: "enabled", Field: "Enabled", Nullable: true},
		{Name: "created", Field: "Created", Nullable: true},
		{Name: "data", Field: "Data", Binary: true, Nullable: true},
		{Name: "id", Field: "ID", Nullable: true},
	})
}

func (s *generatorSuite) TestExportTypesAndPoliciesMatchNativeSchemas(c *tc.C) {
	known := make(map[string]map[string]column)
	for _, test := range []struct {
		ddl     database.Schema
		payload reflect.Type
	}{
		{schema.ControllerDDLForVersion(version.Current), reflect.TypeFor[controllertypes.ControllerExport]()},
		{schema.ModelDDLForVersion(version.Current), reflect.TypeFor[modeltypes.ModelExport]()},
	} {
		db, err := sql.Open("sqlite3", ":memory:")
		c.Assert(err, tc.ErrorIsNil)
		c.Cleanup(func() { c.Check(db.Close(), tc.ErrorIsNil) })
		c.Assert(database.NewDBMigration(&txnRunner{db: db}, logger.Noop(), test.ddl).Apply(c.Context()), tc.ErrorIsNil)
		tables, err := inspect(c.Context(), db)
		c.Assert(err, tc.ErrorIsNil)
		for _, table := range tables {
			columns := make(map[string]column)
			for _, column := range table.Columns {
				columns[column.Name] = column
			}
			known[table.Name] = columns
			payloadField, ok := test.payload.FieldByName(table.Struct)
			if !ok {
				c.Check(recoveryExclusions[table.Name], tc.Not(tc.Equals), "",
					tc.Commentf("unexported table %s must be excluded", table.Name))
				continue
			}
			rowType := payloadField.Type.Elem()
			c.Check(rowType.NumField(), tc.Equals, len(table.Columns))
			for _, column := range table.Columns {
				field, ok := rowType.FieldByName(camel(column.Name))
				c.Assert(ok, tc.IsTrue, tc.Commentf("column %s.%s", table.Name, column.Name))
				c.Check(field.Tag.Get("yaml"), tc.Equals, column.Name)
				c.Check(field.Tag.Get("db"), tc.Equals, column.Name)
				if column.Binary {
					typ := field.Type
					if typ.Kind() == reflect.Pointer {
						typ = typ.Elem()
					}
					want := reflect.TypeFor[[]byte]()
					if column.BinaryText {
						want = reflect.TypeFor[string]()
					}
					c.Check(typ, tc.Equals, want)
				}
			}
		}
	}
	for table := range recoveryExclusions {
		c.Check(known[table], tc.NotNil, tc.Commentf("excluded table %s", table))
	}
}
