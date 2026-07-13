// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"database/sql"

	"github.com/juju/tc"
)

// seedModelDatabaseDeletion stages a deletion for the given namespace.
func (m *stateSuite) seedModelDatabaseDeletion(c *tc.C, namespace string) {
	err := m.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO model_database_deletion (namespace, created_at)
VALUES (?, DATETIME('now', 'utc'))`, namespace)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// registerNamespace inserts a namespace_list row, simulating a live model
// database namespace on this controller.
func (m *stateSuite) registerNamespace(c *tc.C, namespace string) {
	err := m.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO namespace_list (namespace) VALUES (?)`, namespace)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// TestGetPendingModelDatabaseDeletionsEmpty asserts no staged rows yields an
// empty slice and no error.
func (m *stateSuite) TestGetPendingModelDatabaseDeletionsEmpty(c *tc.C) {
	namespaces, err := m.modelState.GetPendingModelDatabaseDeletions(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(namespaces, tc.HasLen, 0)
}

// TestGetPendingModelDatabaseDeletions asserts staged namespaces are returned,
// excluding any that are (again) registered in namespace_list because their
// model returned to this controller.
func (m *stateSuite) TestGetPendingModelDatabaseDeletions(c *tc.C) {
	m.seedModelDatabaseDeletion(c, "gone-model")
	m.seedModelDatabaseDeletion(c, "returned-model")
	// The returned model's namespace is live again, so it must be excluded.
	m.registerNamespace(c, "returned-model")

	namespaces, err := m.modelState.GetPendingModelDatabaseDeletions(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(namespaces, tc.DeepEquals, []string{"gone-model"})
}

// TestRemoveModelDatabaseDeletion asserts removing a staged deletion clears it
// and is idempotent.
func (m *stateSuite) TestRemoveModelDatabaseDeletion(c *tc.C) {
	m.seedModelDatabaseDeletion(c, "gone-model")

	err := m.modelState.RemoveModelDatabaseDeletion(c.Context(), "gone-model")
	c.Assert(err, tc.ErrorIsNil)

	namespaces, err := m.modelState.GetPendingModelDatabaseDeletions(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(namespaces, tc.HasLen, 0)

	// Removing an absent namespace is a no-op.
	err = m.modelState.RemoveModelDatabaseDeletion(c.Context(), "gone-model")
	c.Assert(err, tc.ErrorIsNil)
}
