// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"time"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/domain/removal"
	"github.com/juju/juju/internal/errors"
)

// CharmScheduleRemoval schedules a removal job for the charm with the input
// UUID, to be executed at or after the input time.
// We don't care if the charm does not exist at this point because:
// - it should have been validated prior to calling this method,
// - the removal job executor will handle that fact.
func (st *State) CharmScheduleRemoval(
	ctx context.Context, removalUUID, charmUUID string, when time.Time,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	removalRec := removalJob{
		UUID:          removalUUID,
		RemovalTypeID: uint64(removal.CharmJob),
		EntityUUID:    charmUUID,
		ScheduledFor:  when,
	}

	stmt, err := st.Prepare("INSERT INTO removal (*) VALUES ($removalJob.*)", removalRec)
	if err != nil {
		return errors.Errorf("preparing charm removal: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, removalRec).Run(); err != nil {
			return errors.Errorf("scheduling charm removal: %w", err)
		}
		return nil
	}))
}

// CharmExists returns true if a charm exists with the input UUID.
func (st *State) CharmExists(ctx context.Context, charmUUID string) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	cUUID := entityUUID{UUID: charmUUID}
	stmt, err := st.Prepare(`
SELECT &entityUUID.uuid
FROM   charm
WHERE  uuid = $entityUUID.uuid`, cUUID)
	if err != nil {
		return false, errors.Errorf("preparing charm exists query: %w", err)
	}

	var exists bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, cUUID).Get(&cUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("running charm exists query: %w", err)
		}
		exists = true
		return nil
	})
	if err != nil {
		return false, errors.Capture(err)
	}

	return exists, nil
}

// RescheduleJob updates the scheduled time of the removal job with the input
// UUID.
func (st *State) RescheduleJob(ctx context.Context, jobUUID string, when time.Time) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	job := removalJob{
		UUID:         jobUUID,
		ScheduledFor: when,
	}

	stmt, err := st.Prepare(`
UPDATE removal
SET    scheduled_for = $removalJob.scheduled_for
WHERE  uuid = $removalJob.uuid`, job)
	if err != nil {
		return errors.Errorf("preparing reschedule job query: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, job).Run(); err != nil {
			return errors.Errorf("rescheduling removal job: %w", err)
		}
		return nil
	}))
}

// GetUnusedCharmUUIDs returns the UUIDs of charms that are available, were
// created before the input time, and are not referenced by any application
// or unit. The availability and age guards exclude charms that are
// mid-download or mid-deploy: a charm upload becomes available before the
// application row that references it exists.
func (st *State) GetUnusedCharmUUIDs(ctx context.Context, olderThan time.Time) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	cutoff := charmCutoff{CreateTime: olderThan}
	stmt, err := st.Prepare(`
SELECT c.uuid AS &entityUUID.uuid
FROM   charm c
WHERE  c.available = TRUE
AND    c.create_time < $charmCutoff.create_time
AND    NOT EXISTS (
       SELECT 1
       FROM   application a
       WHERE  a.charm_uuid = c.uuid
)
AND    NOT EXISTS (
       SELECT 1
       FROM   unit u
       WHERE  u.charm_uuid = c.uuid
)`, entityUUID{}, cutoff)
	if err != nil {
		return nil, errors.Errorf("preparing unused charms query: %w", err)
	}

	var charms entityUUIDs
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, cutoff).GetAll(&charms)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("running unused charms query: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return charms.uuids(), nil
}
