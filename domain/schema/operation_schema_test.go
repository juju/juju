// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"testing"

	"github.com/juju/tc"
)

// operationSchemaSuite verifies the operation schema, including that
// operation_id is stored as INTEGER and that all triggers and indexes
// are present.
type operationSchemaSuite struct {
	schemaBaseSuite
}

// TestOperationSchemaSuite registers the tests for the
// [operationSchemaSuite].
func TestOperationSchemaSuite(t *testing.T) {
	tc.Run(t, &operationSchemaSuite{})
}

// SetUpTest is responsible for setting up the model DDL so the operation
// schema can be tested.
func (s *operationSchemaSuite) SetUpTest(c *tc.C) {
	s.schemaBaseSuite.SetUpTest(c)
	s.applyDDL(c, ModelDDL())
}

// TestOperationIdIsInteger verifies that the operation_id column is stored
// as INTEGER, not TEXT. This is the core assertion of the TEXT-to-INTEGER
// migration — the sequence-generated uint64 value must be stored natively
// as an integer to ensure correct numeric ordering.
func (s *operationSchemaSuite) TestOperationIdIsInteger(c *tc.C) {
	var colType string
	err := s.DB().QueryRowContext(c.Context(),
		`SELECT typeof(operation_id) FROM operation LIMIT 1`,
	).Scan(&colType)
	// No rows yet — that's fine, check the column declaration instead.
	if err != nil {
		// Use PRAGMA table_info to get the declared type.
		err = s.DB().QueryRowContext(c.Context(),
			`SELECT type FROM pragma_table_info('operation') WHERE name = 'operation_id'`,
		).Scan(&colType)
		c.Assert(err, tc.ErrorIsNil)
	}
	c.Check(colType, tc.Equals, "INTEGER")
}

// TestOperationTablesPresent verifies that all operation tables exist.
func (s *operationSchemaSuite) TestOperationTablesPresent(c *tc.C) {
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
			tc.Commentf("table %q not found", table))
		c.Check(found, tc.Equals, table)
	}
}

// TestOperationTriggersPresent verifies that all triggers on operation
// tables exist.
func (s *operationSchemaSuite) TestOperationTriggersPresent(c *tc.C) {
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
			tc.Commentf("trigger %q not found", name))
		c.Check(found, tc.Equals, name)
	}
}

// TestOperationIndexesPresent verifies that all indexes on operation tables
// exist.
func (s *operationSchemaSuite) TestOperationIndexesPresent(c *tc.C) {
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
			tc.Commentf("index %q not found", name))
		c.Check(found, tc.Equals, name)
	}
}
