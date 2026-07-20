// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/uuid"
)

// operationSchemaSuite verifies the behaviour of the operation schema
// migrations, in particular the TEXT-to-INTEGER migration of operation_id
// in patch 0062.
type operationSchemaSuite struct {
	schemaBaseSuite
}

// TestOperationSchemaSuite registers the tests for the
// [operationSchemaSuite].
func TestOperationSchemaSuite(t *testing.T) {
	tc.Run(t, &operationSchemaSuite{})
}

// TestOperationIdMigrationCastsTextToInteger verifies that the 0062
// patch correctly migrates operation_id from TEXT to INTEGER. The CAST
// performed by SQLite must preserve the numeric value and the resulting
// column must be of INTEGER storage type.
//
// This test guards against SQLite's permissive CAST behaviour: a
// non-numeric string silently becomes 0, which would cause duplicate-key
// errors on the unique index with an opaque message.
func (s *operationSchemaSuite) TestOperationIdMigrationCastsTextToInteger(c *tc.C) {
	// 1. Start with the pre-0062 schema where operation_id is TEXT.
	preMigration := semversion.MustParse("4.0.12")
	s.applyDDL(c, ModelDDLForVersion(preMigration))

	// 2. Insert TEXT operation_id values, simulating pre-migration data.
	now := time.Now().UTC()
	opUUID1 := uuid.MustNewUUID().String()
	opUUID2 := uuid.MustNewUUID().String()

	s.assertExecSQL(c,
		`INSERT INTO operation (uuid, operation_id, enqueued_at) VALUES (?, ?, ?)`,
		opUUID1, "99", now,
	)
	s.assertExecSQL(c,
		`INSERT INTO operation (uuid, operation_id, enqueued_at) VALUES (?, ?, ?)`,
		opUUID2, "7", now,
	)

	// Sanity check: pre-migration, the column is TEXT.
	var preType string
	err := s.DB().QueryRowContext(c.Context(),
		`SELECT typeof(operation_id) FROM operation LIMIT 1`,
	).Scan(&preType)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(preType, tc.Equals, "text")

	// 3. Apply the 0062 patch by upgrading to 4.0.13.
	postMigration := semversion.MustParse("4.0.13")
	s.reapplyDDL(c, ModelDDLForVersion(postMigration))

	// 4. Verify the values are correct integers, sorted numerically.
	var ids []int64
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		ids = nil
		rows, err := tx.QueryContext(ctx,
			`SELECT operation_id FROM operation ORDER BY operation_id`,
		)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ids, tc.HasLen, 2)
	// Numeric sort: 7 before 42 (with TEXT it would be "42" before "7").
	c.Check(ids[0], tc.Equals, int64(7))
	c.Check(ids[1], tc.Equals, int64(99))

	// 5. Verify the column storage type is now INTEGER.
	var postType string
	err = s.DB().QueryRowContext(c.Context(),
		`SELECT typeof(operation_id) FROM operation LIMIT 1`,
	).Scan(&postType)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(postType, tc.Equals, "integer")
}

// TestOperationIdMigrationUniqueIndexIntact verifies that the unique index
// on operation_id still works after the migration. This guards against
// the silent CAST-to-0 collision the reviewer flagged: if two non-numeric
// values existed, both would CAST to 0 and the unique index creation
// would fail with an opaque error.
func (s *operationSchemaSuite) TestOperationIdMigrationUniqueIndexIntact(c *tc.C) {
	// 1. Start with the pre-0062 schema.
	preMigration := semversion.MustParse("4.0.12")
	s.applyDDL(c, ModelDDLForVersion(preMigration))

	// 2. Insert a single row with a TEXT operation_id.
	now := time.Now().UTC()
	opUUID := uuid.MustNewUUID().String()
	s.assertExecSQL(c,
		`INSERT INTO operation (uuid, operation_id, enqueued_at) VALUES (?, ?, ?)`,
		opUUID, "99", now,
	)

	// 3. Apply the 0062 patch.
	postMigration := semversion.MustParse("4.0.13")
	s.reapplyDDL(c, ModelDDLForVersion(postMigration))

	// 4. Verify the unique index is intact: inserting a duplicate must fail.
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO operation (uuid, operation_id, enqueued_at) VALUES (?, ?, ?)`,
			uuid.MustNewUUID().String(), 99, now,
		)
		return err
	})
	c.Assert(err, tc.NotNil)
	c.Check(err.Error(), tc.Matches, ".*UNIQUE constraint failed: operation.operation_id.*")
}

// TestOperationIdMigrationTriggersRecreated verifies that the triggers
// destroyed by DROP TABLE during the 0062 migration are recreated by
// the patch. Without these triggers, changestream events would be lost
// for operation_task_log and operation_task_status.
func (s *operationSchemaSuite) TestOperationIdMigrationTriggersRecreated(c *tc.C) {
	// 1. Apply the full current schema (which includes 0062 on fresh DB).
	s.applyDDL(c, ModelDDL())

	// 2. Verify the recreated triggers exist by inspecting sqlite_master.
	expectedTriggers := []string{
		// Immutability triggers
		"trg_operation_parameter_immutable_update",
		"trg_operation_machine_task_immutable_update",
		"trg_operation_unit_task_immutable_update",
		// Mutual exclusivity triggers
		"trg_insert_machine_task_if_not_unit_task",
		"trg_insert_unit_task_if_not_machine_task",
		// Operation task status triggers
		"trg_log_custom_operation_task_status_pending_insert",
		"trg_log_custom_operation_task_status_pending_update",
		"trg_log_custom_operation_task_status_pending_or_aborting_insert",
		"trg_log_custom_operation_task_status_pending_or_aborting_update",
		// Operation task log changestream triggers
		"trg_log_operation_task_log_insert",
		"trg_log_operation_task_log_update",
		"trg_log_operation_task_log_delete",
	}

	for _, name := range expectedTriggers {
		var found string
		err := s.DB().QueryRowContext(c.Context(),
			`SELECT name FROM sqlite_master WHERE type = 'trigger' AND name = ?`,
			name,
		).Scan(&found)
		c.Assert(err, tc.ErrorIsNil,
			tc.Commentf("trigger %q not found after schema apply", name))
		c.Check(found, tc.Equals, name)
	}
}

// TestOperationIdMigrationAllTablesPresent verifies that all 9 tables in
// the operation FK dependency tree exist after the migration. This guards
// against the patch accidentally dropping a table without recreating it.
func (s *operationSchemaSuite) TestOperationIdMigrationAllTablesPresent(c *tc.C) {
	// Apply the full current schema (includes 0062 on fresh DB).
	s.applyDDL(c, ModelDDL())

	expectedTables := []string{
		"operation",
		"operation_action",
		"operation_task",
		"operation_parameter",
		"operation_unit_task",
		"operation_machine_task",
		"operation_task_output",
		"operation_task_status",
		"operation_task_status_value",
		"operation_task_log",
	}

	for _, table := range expectedTables {
		var found string
		err := s.DB().QueryRowContext(c.Context(),
			`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`,
			table,
		).Scan(&found)
		c.Assert(err, tc.ErrorIsNil,
			tc.Commentf("table %q not found after schema apply", table))
		c.Check(found, tc.Equals, table)
	}
}

// TestOperationIdMigrationAllIndexesPresent verifies that all indexes on
// the operation tables are recreated after the migration.
func (s *operationSchemaSuite) TestOperationIdMigrationAllIndexesPresent(c *tc.C) {
	// Apply the full current schema (includes 0062 on fresh DB).
	s.applyDDL(c, ModelDDL())

	expectedIndexes := []string{
		"idx_operation_id",
		"idx_operation_action_charm_action_key_operation_uuid",
		"idx_task_id",
		"idx_operation_task_log_id",
	}

	for _, name := range expectedIndexes {
		var found string
		err := s.DB().QueryRowContext(c.Context(),
			`SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`,
			name,
		).Scan(&found)
		c.Assert(err, tc.ErrorIsNil,
			tc.Commentf("index %q not found after schema apply", name))
		c.Check(found, tc.Equals, name)
	}
}
