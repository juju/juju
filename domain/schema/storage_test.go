// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"strings"
	"testing"

	"github.com/juju/tc"

	coreschema "github.com/juju/juju/core/database/schema"
)

type storageSuite struct{ schemaBaseSuite }

func TestStorageSuite(t *testing.T) { tc.Run(t, &storageSuite{}) }

func (s *storageSuite) TestBinaryTextColumnsMatchSchemas(c *tc.C) {
	known := make(map[string]map[string]string)
	for _, ddl := range []*coreschema.Schema{ControllerDDL(), ModelDDL()} {
		runner, db := s.OpenDB(c)
		_, err := ddl.Ensure(c.Context(), runner)
		c.Assert(err, tc.ErrorIsNil)
		for table := range binaryTextColumns {
			rows, err := db.QueryContext(c.Context(), `SELECT pti.name, pti.type
FROM pragma_table_info(?) AS pti`, table)
			c.Assert(err, tc.ErrorIsNil)
			for rows.Next() {
				var column, typ string
				c.Assert(rows.Scan(&column, &typ), tc.ErrorIsNil)
				if known[table] == nil {
					known[table] = make(map[string]string)
				}
				known[table][column] = typ
			}
			c.Assert(rows.Err(), tc.ErrorIsNil)
			c.Assert(rows.Close(), tc.ErrorIsNil)
		}
	}
	for table, columns := range binaryTextColumns {
		for _, column := range columns {
			c.Check(known[table][column], tc.Equals, "TEXT",
				tc.Commentf("legacy binary column %s.%s", table, column))
			c.Check(ColumnStorageType(table, column, "TEXT"), tc.Equals, "BLOB")
		}
	}
}

func (s *storageSuite) TestOtherStorageTypesRetainTheirDeclaration(c *tc.C) {
	for _, typ := range []string{"int", "integer", "text", "blob", "boolean", "timestamp"} {
		// An ordinary text column in a table with binary exceptions must keep
		// its declared type, as must columns in other tables.
		c.Check(ColumnStorageType("user_password", "password_hash", typ), tc.Equals,
			strings.ToUpper(typ))
		c.Check(ColumnStorageType("sequence", "value", typ), tc.Equals,
			strings.ToUpper(typ))
	}
}
