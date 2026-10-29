// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"os"
	"path/filepath"
	"regexp"
	stdtesting "testing"

	"github.com/juju/tc"
)

type tablesSuite struct{}

func TestTablesSuite(t *stdtesting.T) {
	tc.Run(t, &tablesSuite{})
}

var createTableRE = regexp.MustCompile(`(?i)CREATE TABLE (?:IF NOT EXISTS )?"?([a-zA-Z_0-9]+)"?`)

// schemaTables reads the schema DDL shipped in the repository and returns
// every table name it creates. Tests run with the package directory as the
// working directory, so the domain schema is three levels up.
func schemaTables(c *tc.C, patterns ...string) []string {
	var tables []string
	seen := make(map[string]struct{})
	for _, pattern := range patterns {
		files, err := filepath.Glob(filepath.Join("..", "..", "..", pattern))
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(files, tc.Not(tc.HasLen), 0, tc.Commentf("no files for %s", pattern))
		for _, file := range files {
			data, err := os.ReadFile(file)
			c.Assert(err, tc.ErrorIsNil)
			for _, m := range createTableRE.FindAllStringSubmatch(string(data), -1) {
				name := m[1]
				if _, dup := seen[name]; dup {
					continue
				}
				seen[name] = struct{}{}
				tables = append(tables, name)
			}
		}
	}
	return tables
}

func (s *tablesSuite) TestTablePoliciesCoverSchema(c *tc.C) {
	for _, tt := range []struct {
		name          string
		patterns      []string
		policies      map[string]tablePolicy
		specialTables []string
	}{
		{
			name:          "controller",
			patterns:      []string{"domain/schema/controller/sql/*.sql"},
			policies:      controllerTablePolicies,
			specialTables: []string{"object_store_placement"},
		},
		{
			name:          "model",
			patterns:      []string{"domain/schema/model/sql/*.sql", "domain/schema/model/*.ddl"},
			policies:      modelTablePolicies,
			specialTables: []string{"object_store_placement"},
		},
	} {
		tables := schemaTables(c, tt.patterns...)

		// The migration framework creates the schema bookkeeping table
		// itself; it is not in the DDL files.
		tables = append(tables, "schema")

		// Every policy or special-table entry must name a table the
		// schema actually creates: a stale entry silently changes
		// behaviour after a schema rename.
		known := make(map[string]struct{}, len(tables))
		for _, table := range tables {
			known[table] = struct{}{}
		}
		var stale []string
		for table := range tt.policies {
			if _, ok := known[table]; !ok {
				stale = append(stale, table)
			}
		}
		for _, table := range tt.specialTables {
			if _, ok := known[table]; !ok {
				stale = append(stale, table)
			}
		}
		c.Check(stale, tc.HasLen, 0, tc.Commentf(
			"policy entries for tables the schema does not create: %v", stale))
	}
}

// TestOrdinaryTablesAreReplaced pins the default for known schema tables.
// Unknown archive tables are rejected by generated decoding before policy runs.
func (s *tablesSuite) TestOrdinaryTablesAreReplaced(c *tc.C) {
	c.Check(tablePolicy(0), tc.Equals, policyReplace)
	c.Check(controllerTablePolicies["cloud"], tc.Equals, policyReplace)
	c.Check(modelTablePolicies["machine"], tc.Equals, policyReplace)
}
