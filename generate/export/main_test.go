// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/juju/tc"
)

type generatorSuite struct{}

func TestGeneratorSuite(t *testing.T) {
	tc.Run(t, &generatorSuite{})
}

func (s *generatorSuite) TestExportGeneratorDoesNotOwnImportTemplates(c *tc.C) {
	_, filename, _, ok := runtime.Caller(0)
	c.Assert(ok, tc.IsTrue)

	dir := filepath.Dir(filename)
	for _, name := range []string{"import.tmpl", "import_test.tmpl"} {
		_, err := os.Stat(filepath.Join(dir, name))
		c.Assert(err, tc.ErrorIs, os.ErrNotExist)
	}
}

func (s *generatorSuite) TestNewExportTableAddsNullMarkers(c *tc.C) {
	table := newExportTable("some_table", "SomeTable", []column{
		{Name: "uuid", Type: "TEXT", NotNull: true},
		{Name: "enabled", Type: "BOOLEAN"},
		{Name: "updated_at", Type: "DATETIME"},
	})

	c.Check(table.NullableColumns, tc.DeepEquals, []nullableColumn{
		{Name: "enabled", FieldName: "Enabled"},
		{Name: "updated_at", FieldName: "UpdatedAt"},
	})
}

func (s *generatorSuite) TestAffectedNullableTypes(c *tc.C) {
	for _, columnType := range []string{"BOOLEAN", "DATETIME", "DATE", "TIMESTAMP"} {
		c.Check(isAffectedNullableType(column{Type: columnType}), tc.IsTrue)
	}
	c.Check(isAffectedNullableType(column{Type: "TEXT"}), tc.IsFalse)
	c.Check(isAffectedNullableType(column{Type: "BOOLEAN", NotNull: true}), tc.IsFalse)
}
