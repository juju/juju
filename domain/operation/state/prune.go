// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/internal/errors"
)

// PruneOperations deletes operations older than maxAge and larger than maxSizeMB.
func (st *State) PruneOperations(ctx context.Context, maxAge time.Duration, maxSizeMB int) error {

	// Prune by age, completed only
	err := st.pruneCompletedOperationsOlderThan(ctx, maxAge)
	if err != nil {
		return errors.Errorf("pruning completed operation by age: %w", err)
	}

	// Prune by size
	// TODO(gfouillet): implement it as followup:
	//   - first pass should delete only completed operations up to the desired size
	//   - second pass should delete from all operations, oldest first

	// TODO(gfouillet): In a followup PR, we should return the storeUUIDs
	//   freed by the state prune operation.

	return nil
}

// pruneCompletedOperationsOlderThan deletes operations which have completed at
// a time older than age.
func (st *State) pruneCompletedOperationsOlderThan(ctx context.Context, age time.Duration) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		toDelete, err := st.getCompletedOperationUUIDsOlderThan(ctx, tx, age)
		if err != nil {
			return errors.Errorf("getting operation UUIDs older than %s: %w", age, err)
		}

		err = st.deleteOperationByUUIDs(ctx, tx, toDelete)
		if err != nil {
			return errors.Errorf("deleting operations with UUIDs %v: %w", toDelete, err)
		}
		return nil
	})
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// getCompletedOperationUUIDsOlderThan returns the UUIDs of operations older than age.
func (st *State) getCompletedOperationUUIDsOlderThan(ctx context.Context, tx *sqlair.TX, age time.Duration) ([]string, error) {
	if age <= 0 {
		// age shouldn't be negative, but zero age is valid. In any case, we ignore
		// the prune by age as done in 3.6
		st.logger.Warningf(ctx, "Ignoring pruning by age ignored: zero age (age: %s)", age)
		return nil, nil
	}

	type expires struct {
		At time.Time `db:"at"`
	}

	type operation uuid

	expiresAt := expires{At: time.Now().Add(-age)}

	stmt, err := st.Prepare(`
	SELECT &operation.uuid 
	FROM   operation
	WHERE  completed_at IS NOT NULL
	AND    completed_at < $expires.at
    `, operation{}, expiresAt)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var operations []operation
	err = tx.Query(ctx, stmt, expiresAt).GetAll(&operations)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Capture(err)
	}

	return transform.Slice(operations, func(o operation) string { return o.UUID }), nil
}
