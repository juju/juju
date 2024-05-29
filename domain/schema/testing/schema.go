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

// DumpChangeLogState dumps the change log to the test log.
func DumpChangeLogState(c *gc.C, runner database.TxnRunner) {
	var (
		logs    []changeLogRow
		witness changeLogWitnessRow
	)
	err := runner.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT id, edit_type_id, namespace_id, changed, created_at FROM change_log")
		c.Assert(err, jc.ErrorIsNil)

		defer rows.Close()

		for rows.Next() {
			var row changeLogRow
			err := rows.Scan(&row.ID, &row.EditTypeID, &row.NamespaceID, &row.Changed, &row.CreatedAt)
			c.Assert(err, jc.ErrorIsNil)
			logs = append(logs, row)
		}

		row := tx.QueryRowContext(ctx, "SELECT controller_id, lower_bound, upper_bound, updated_at FROM change_log_witness")
		err = row.Scan(&witness.ControllerID, &witness.LowerBound, &witness.UpperBound, &witness.UpdatedAt)
		c.Assert(err, jc.ErrorIsNil)

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Logf("Change log witness: %v", witness)
	c.Logf("Change log entries:")
	for _, log := range logs {
		c.Logf("  %d: %v", log.ID, log)
	}
}

type changeLogRow struct {
	ID          int
	EditTypeID  int
	NamespaceID int
	Changed     string
	CreatedAt   string
}

type changeLogWitnessRow struct {
	ControllerID string
	LowerBound   int
	UpperBound   int
	UpdatedAt    string
}
