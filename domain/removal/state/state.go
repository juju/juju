// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"time"

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
		return false, errors.Errorf("preparing relation exists query: %w", err)
	}

	var relationExists bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, existsStmt, relationUUID).Get(&relationUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("running relation exists query: %w", err)
		}

		relationExists = true
		return nil
	})

	return relationExists, err
}

// RelationAdvanceLifeAndScheduleRemoval advances the life cycle of the relation
// with the input UUID to dying if it is alive, and schedules a removal job for
// the relation, qualified with the input force boolean.
// We don't care if the relation does not exist at this point because:
// - it should have been validated prior to calling this method,
// - the removal job executor will handle that fact.
func (st *State) RelationAdvanceLifeAndScheduleRemoval(
	ctx context.Context, removalUUID, relUUID string, force bool, when time.Time,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	relationUUID := entityUUID{UUID: relUUID}
	lifeStmt, err := st.Prepare(`
UPDATE relation
SET    life_id = 1
WHERE  uuid = $entityUUID.uuid
AND    life_id = 0`, relationUUID)
	if err != nil {
		return errors.Errorf("preparing relation life update: %w", err)
	}

	removalRec := removal{
		UUID:          removalUUID,
		RemovalTypeID: 0,
		EntityUUID:    relUUID,
		Force:         force,
		ScheduledFor:  when,
	}

	removalStmt, err := st.Prepare("INSERT INTO removal (*) VALUES ($removal.*)", removalRec)
	if err != nil {
		return errors.Errorf("preparing relation life update: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, lifeStmt, relationUUID).Run()
		if err != nil {
			return errors.Errorf("advancing relation %q life: %w", relUUID, err)
		}

		err = tx.Query(ctx, removalStmt, removalRec).Run()
		if err != nil {
			return errors.Errorf("scheduling relation %q removal: %w", relUUID, err)
		}

		return nil
	}))
}

// NamespaceForWatchRemovals returns the table name whose UUIDs we
// are watching in order to be notified of new removal jobs.
func (st *State) NamespaceForWatchRemovals() string {
	return "removal"
}
