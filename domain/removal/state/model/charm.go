// Copyright 2026 Canonical Ltd.
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
// UUID, to be actioned at the input time.
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

// CharmExists reports whether the charm with the input UUID exists.
func (st *State) CharmExists(ctx context.Context, charmUUID string) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	charmID := entityUUID{UUID: charmUUID}
	stmt, err := st.Prepare(`
SELECT uuid AS &entityUUID.uuid
FROM   charm
WHERE  uuid = $entityUUID.uuid
`, charmID)
	if err != nil {
		return false, errors.Capture(err)
	}

	exists := false
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists = false
		var charm entityUUID
		err := tx.Query(ctx, stmt, charmID).Get(&charm)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Capture(err)
		}
		exists = true
		return nil
	})
	return exists, errors.Capture(err)
}

// CharmHasReferencedResources reports whether any resource of the charm with
// the input UUID is still referenced by an application, pending application,
// or unit. Such resources pin the charm: its charm_resource rows cannot be
// deleted until the references drop.
func (st *State) CharmHasReferencedResources(ctx context.Context, charmUUID string) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	charmID := entityUUID{UUID: charmUUID}
	stmt, err := st.Prepare(`
SELECT r.uuid AS &entityUUID.uuid
FROM   resource AS r
WHERE  r.charm_uuid = $entityUUID.uuid
AND    (
    EXISTS (
        SELECT 1
        FROM   application_resource AS ar
        WHERE  ar.resource_uuid = r.uuid
    )
    OR EXISTS (
        SELECT 1
        FROM   pending_application_resource AS par
        WHERE  par.resource_uuid = r.uuid
    )
    OR EXISTS (
        SELECT 1
        FROM   unit_resource AS ur
        WHERE  ur.resource_uuid = r.uuid
    )
)
LIMIT  1
`, charmID)
	if err != nil {
		return false, errors.Capture(err)
	}

	pinned := false
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		pinned = false
		var resource entityUUID
		err := tx.Query(ctx, stmt, charmID).Get(&resource)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Capture(err)
		}
		pinned = true
		return nil
	})
	return pinned, errors.Capture(err)
}
