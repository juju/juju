// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"encoding/json"
	"time"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/removal"
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

// GetAllJobs returns all scheduled removal jobs.
func (st *State) GetAllJobs(ctx context.Context) ([]removal.Job, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare("SELECT &removalJob.* FROM removal", removalJob{})
	if err != nil {
		return nil, errors.Errorf("preparing select jobs query: %w", err)
	}

	var dbJobs []removalJob
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt).GetAll(&dbJobs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("running select jobs query: %w", err)
		}
		return nil
	})

	if err != nil {
		return nil, errors.Capture(err)
	}

	if len(dbJobs) == 0 {
		return nil, nil
	}

	jobs := make([]removal.Job, len(dbJobs))
	for i, job := range dbJobs {
		var arg map[string]any
		if job.Arg.Valid && job.Arg.String != "" {
			if err := json.Unmarshal([]byte(job.Arg.String), &arg); err != nil {
				return nil, errors.Errorf("decoding job arg: %w", err)
			}
		}

		jobs[i] = removal.Job{
			UUID:         removal.UUID(job.UUID),
			RemovalType:  removal.JobType(job.RemovalTypeID),
			EntityUUID:   job.EntityUUID,
			Force:        job.Force,
			ScheduledFor: job.ScheduledFor,
			Arg:          arg,
		}
	}

	return jobs, err
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

// RelationAdvanceLife ensures that there is no relation
// identified by the input UUID, that is still alive.
func (st *State) RelationAdvanceLife(ctx context.Context, rUUID string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	relationUUID := entityUUID{UUID: rUUID}
	stmt, err := st.Prepare(`
UPDATE relation
SET    life_id = 1
WHERE  uuid = $entityUUID.uuid
AND    life_id = 0`, relationUUID)
	if err != nil {
		return errors.Errorf("preparing relation life update: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, relationUUID).Run()
		if err != nil {
			return errors.Errorf("advancing relation life: %w", err)
		}
		return nil
	}))
}

// RelationScheduleRemoval schedules a removal job for the relation with the
// input UUID, qualified with the input force boolean.
// We don't care if the relation does not exist at this point because:
// - it should have been validated prior to calling this method,
// - the removal job executor will handle that fact.
func (st *State) RelationScheduleRemoval(
	ctx context.Context, removalUUID, relUUID string, force bool, when time.Time,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	removalRec := removalJob{
		UUID:          removalUUID,
		RemovalTypeID: 0,
		EntityUUID:    relUUID,
		Force:         force,
		ScheduledFor:  when,
	}

	removalStmt, err := st.Prepare("INSERT INTO removal (*) VALUES ($removalJob.*)", removalRec)
	if err != nil {
		return errors.Errorf("preparing relation removal: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, removalStmt, removalRec).Run()
		if err != nil {
			return errors.Errorf("scheduling relation removal: %w", err)
		}
		return nil
	}))
}

// NamespaceForWatchRemovals returns the table name whose UUIDs we
// are watching in order to be notified of new removal jobs.
func (st *State) NamespaceForWatchRemovals() string {
	return "removal"
}
