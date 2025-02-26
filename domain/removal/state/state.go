// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/internal/errors"
)

// State provides persistence and retrieval associated with entity removal.
type State struct {
	*domain.StateBase
	logger logger.Logger
}

// NewState returns a new state reference.
func NewState(factory database.TxnRunnerFactory, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// RelationExists returns true if a relation exists with the input UUID.
func (st *State) RelationExists(ctx context.Context, rUUID string) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Capture(err)
	}

	relationUUID := entityUUID{UUID: rUUID}
	existsStmt, err := st.Prepare(`
SELECT uuid AS &entityUUID.uuid
FROM   relation
WHERE  uuid = $entityUUID.uuid`, relationUUID)
	if err != nil {
		return false, err
	}

	var relationExists bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, existsStmt, relationUUID).Get(&relationUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("checking if relation %q exists: %w", rUUID, err)
		}

		relationExists = true
		return nil
	})

	return relationExists, err
}
