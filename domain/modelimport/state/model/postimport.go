// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/domain/export/types/v4_1_0"
)

// MergeModelAgentPassword merges the migrated model's agent password hash
// into the target's bootstrap model_agent row. That row is target-owned and
// created at bootstrap, so Import never touches it directly; only the
// password travels from the source.
func (st *State) MergeModelAgentPassword(ctx context.Context, modelAgent v4_1_0.ModelAgent) error {
	db, err := st.DB(ctx)
	if err != nil {
		return fmt.Errorf("getting db: %w", err)
	}

	stmt, err := sqlair.Prepare(`
UPDATE model_agent
SET    password_hash = $ModelAgent.password_hash,
       password_hash_algorithm_id = $ModelAgent.password_hash_algorithm_id
WHERE  model_uuid = $ModelAgent.model_uuid
`, v4_1_0.ModelAgent{})
	if err != nil {
		return fmt.Errorf("preparing model_agent password merge statement: %w", err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var outcome sqlair.Outcome
		if err := tx.Query(ctx, stmt, modelAgent).Get(&outcome); err != nil {
			return fmt.Errorf("merging model_agent password: %w", err)
		}
		affected, err := outcome.Result().RowsAffected()
		if err != nil {
			return fmt.Errorf("checking model_agent password merge result: %w", err)
		}
		if affected != 1 {
			return fmt.Errorf("merging model_agent password: affected %d rows, expected 1", affected)
		}
		return nil
	})
}
