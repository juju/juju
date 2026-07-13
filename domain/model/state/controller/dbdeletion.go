// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/internal/errors"
)

// GetPendingModelDatabaseDeletions returns the dqlite namespaces staged for
// deletion in model_database_deletion.
//
// Namespaces that are (again) registered in namespace_list are excluded: they
// belong to a model that has since returned to this controller, so their
// databases are live and must not be deleted. This guards the migrate-away,
// migrate-back, migrate-away-again sequence; a residual race remains only if a
// model with the same UUID is being imported at the exact moment the worker
// reads this, which is not realistic on human timescales.
func (st *State) GetPendingModelDatabaseDeletions(ctx context.Context) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT &dbModelDatabaseDeletion.namespace
FROM   model_database_deletion
WHERE  namespace NOT IN (SELECT namespace FROM namespace_list)
`, dbModelDatabaseDeletion{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var rows []dbModelDatabaseDeletion
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		rows = nil
		err := tx.Query(ctx, stmt).GetAll(&rows)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Errorf("getting pending model database deletions: %w", err)
	}

	namespaces := make([]string, len(rows))
	for i, r := range rows {
		namespaces[i] = r.Namespace
	}
	return namespaces, nil
}

// RemoveModelDatabaseDeletion removes the staged deletion for the given
// namespace. Removing an absent namespace is a no-op, so concurrent controller
// nodes racing to complete the same deletion do so benignly.
func (st *State) RemoveModelDatabaseDeletion(ctx context.Context, namespace string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	arg := dbModelDatabaseDeletion{Namespace: namespace}
	stmt, err := st.Prepare(`
DELETE FROM model_database_deletion
WHERE  namespace = $dbModelDatabaseDeletion.namespace
`, arg)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, arg).Run()
	})
	if err != nil {
		return errors.Errorf("removing model database deletion for %q: %w", namespace, err)
	}
	return nil
}
