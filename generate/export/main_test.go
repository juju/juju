// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"testing"

	"github.com/juju/tc"
)

type exportGeneratorSuite struct{}

func TestExportGeneratorSuite(t *testing.T) {
	tc.Run(t, &exportGeneratorSuite{})
}

// TestNewExportTableAddsNullMarkers checks that affected nullable columns get
// companion projections that retain their original SQL nullness.
func (s *exportGeneratorSuite) TestNewExportTableAddsNullMarkers(c *tc.C) {
	table := newExportTable("some_table", "SomeTable", []column{
		{Name: "uuid", Type: "TEXT", NotNull: true},
		{Name: "enabled", Type: "BOOLEAN"},
		{Name: "updated_at", Type: "DATETIME"},
		{Name: "scheduled_on", Type: "DATE"},
		{Name: "completed_at", Type: "TIMESTAMP"},
	})

	c.Check(table, tc.DeepEquals, exportTable{
		Name:         "some_table",
		StructName:   "SomeTable",
		NullTypeName: "nullableSomeTable",
		Query: `SELECT &SomeTable.*,
       t."enabled" IS NULL AS &nullableSomeTable.enabled_is_null,
       t."updated_at" IS NULL AS &nullableSomeTable.updated_at_is_null,
       t."scheduled_on" IS NULL AS &nullableSomeTable.scheduled_on_is_null,
       t."completed_at" IS NULL AS &nullableSomeTable.completed_at_is_null
FROM   "some_table" AS t`,
		NullableColumns: []nullableColumn{
			{Name: "enabled", FieldName: "Enabled"},
			{Name: "updated_at", FieldName: "UpdatedAt"},
			{Name: "scheduled_on", FieldName: "ScheduledOn"},
			{Name: "completed_at", FieldName: "CompletedAt"},
		},
	})
}

// TestNewExportTableWithoutAffectedTypes checks that unaffected tables retain
// the compact wildcard query and require no null marker rows.
func (s *exportGeneratorSuite) TestNewExportTableWithoutAffectedTypes(c *tc.C) {
	table := newExportTable("some_table", "SomeTable", []column{
		{Name: "uuid", Type: "TEXT", NotNull: true},
	})

	c.Check(table.Query, tc.Equals, `SELECT &SomeTable.* FROM "some_table"`)
	c.Check(table.NullableColumns, tc.HasLen, 0)
}

// TestAffectedNullableTypes checks that null markers cover every declared
// type remapped by dqlite.
func (s *exportGeneratorSuite) TestAffectedNullableTypes(c *tc.C) {
	for _, columnType := range []string{"BOOLEAN", "DATETIME", "DATE", "TIMESTAMP"} {
		c.Check(isAffectedNullableType(column{Type: columnType}), tc.IsTrue)
	}
	c.Check(isAffectedNullableType(column{Type: "TEXT"}), tc.IsFalse)
	c.Check(isAffectedNullableType(column{
		Type:    "BOOLEAN",
		NotNull: true,
	}), tc.IsFalse)
}
