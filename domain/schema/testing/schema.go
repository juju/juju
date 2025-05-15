// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"database/sql"

	"github.com/juju/tc"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/database/schema"
)

// SchemaApplier is a helper that applies a schema to a database.
type SchemaApplier struct {
	Schema  *schema.Schema
	Verbose bool
}

// Apply applies the schema to the database.
func (s *SchemaApplier) Apply(c *tc.C, ctx context.Context, runner database.TxnRunner) {
	if s.Verbose {
		s.Schema.Hook(func(i int, statement string) error {
			c.Logf("-- Applying schema change %d\n%s\n", i, statement)
			return nil
		})
	}

	changeSet, err := s.Schema.Ensure(ctx, runner)
	c.Assert(err, tc.IsNil)
	c.Check(changeSet.Post, tc.Equals, s.Schema.Len())
}

// DumpChangeLogState dumps the change log to the test log.
func DumpChangeLogState(c *tc.C, runner database.TxnRunner) {
	var (
		logs    []changeLogRow
		witness changeLogWitnessRow
	)
	err := runner.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT id, edit_type_id, namespace_id, changed, created_at FROM change_log")
		if err != nil {
			return err
		}

		defer rows.Close()

		for rows.Next() {
			var row changeLogRow
			err := rows.Scan(&row.ID, &row.EditTypeID, &row.NamespaceID, &row.Changed, &row.CreatedAt)
			if err != nil {
				return err
			}
			logs = append(logs, row)
		}

		row := tx.QueryRowContext(ctx, "SELECT controller_id, lower_bound, upper_bound, updated_at FROM change_log_witness")
		err = row.Scan(&witness.ControllerID, &witness.LowerBound, &witness.UpperBound, &witness.UpdatedAt)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

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
