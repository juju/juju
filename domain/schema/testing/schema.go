// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"database/sql"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/database/schema"
)

// SchemaApplier is a helper that applies a schema to a database.
type SchemaApplier struct {
	Schema  *schema.Schema
	Verbose bool
}

// Apply applies the schema to the database.
func (s *SchemaApplier) Apply(c *gc.C, ctx context.Context, runner database.TxnRunner) {
	if s.Verbose {
		s.Schema.Hook(func(i int, statement string) error {
			c.Logf("-- Applying schema change %d\n%s\n", i, statement)
			return nil
		})
	}

	changeSet, err := s.Schema.Ensure(ctx, runner)
	c.Assert(err, gc.IsNil)
	c.Check(changeSet.Post, gc.Equals, s.Schema.Len())
}

// DumpChangeLog dumps the change log to the test log.
func DumpChangeLog(ctx context.Context, c *gc.C, runner database.TxnRunner) {
	err := runner.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT id, edit_type_id, namespace_id, changed, created_at FROM change_log")
		c.Assert(err, jc.ErrorIsNil)

		defer rows.Close()

		for rows.Next() {
			var id, editTypeID, namespaceID int
			var changed, createdAt string
			err := rows.Scan(&id, &editTypeID, &namespaceID, &changed, &createdAt)
			c.Assert(err, jc.ErrorIsNil)
			c.Logf("change log entry %d: %d %d %s %s", id, editTypeID, namespaceID, changed, createdAt)
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}
