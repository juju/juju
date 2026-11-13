// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/database/schema"
)

type primaryKeySchemaSuite struct {
	schemaBaseSuite
}

func TestPrimaryKeySchemaSuite(t *testing.T) {
	tc.Run(t, &primaryKeySchemaSuite{})
}

// TestModelPrimaryKeysAreNotNullable verifies that no model table can store a
// NULL in its primary key.
func (s *primaryKeySchemaSuite) TestModelPrimaryKeysAreNotNullable(c *tc.C) {
	s.assertPrimaryKeysAreNotNullable(c, ModelDDL())
}

// TestControllerPrimaryKeysAreNotNullable verifies that no controller table
// can store a NULL in its primary key.
func (s *primaryKeySchemaSuite) TestControllerPrimaryKeysAreNotNullable(c *tc.C) {
	s.assertPrimaryKeysAreNotNullable(c, ControllerDDL())
}

// assertPrimaryKeysAreNotNullable applies the DDL and checks that every
// primary key column is declared NOT NULL. SQLite accepts NULL in most
// primary key columns, whereas the SQL standard and PostgreSQL do not, so a
// nullable key column lets SQLite store rows that have no portable identity.
//
// A single-column integer key is exempt. INTEGER PRIMARY KEY aliases the rowid
// and cannot hold NULL, and the INT PRIMARY KEY columns of the static lookup
// tables are only written by the schema itself.
func (s *primaryKeySchemaSuite) assertPrimaryKeysAreNotNullable(c *tc.C, ddl *schema.Schema) {
	s.applyDDL(c, ddl)

	rows, err := s.DB().QueryContext(c.Context(), `
SELECT m.name, p.name
FROM   sqlite_master AS m
JOIN   pragma_table_info(m.name) AS p
WHERE  m.type = 'table'
AND    m.name NOT LIKE 'sqlite_%'
AND    p.pk > 0
AND    p."notnull" = 0
AND    NOT (
           UPPER(p.type) IN ('INT', 'INTEGER')
           AND (SELECT COUNT(*) FROM pragma_table_info(m.name) WHERE pk > 0) = 1
       )
ORDER BY m.name, p.pk`)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	var nullable []string
	for rows.Next() {
		var table, column string
		c.Assert(rows.Scan(&table, &column), tc.ErrorIsNil)
		nullable = append(nullable, table+"."+column)
	}
	c.Assert(rows.Err(), tc.ErrorIsNil)
	c.Check(nullable, tc.HasLen, 0)
}
