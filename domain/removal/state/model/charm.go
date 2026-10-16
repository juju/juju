// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/domain/removal"
	"github.com/juju/juju/internal/errors"
)

// scheduleCharmRemoval schedules a removal job for the charm with the input
// UUID, inside the current transaction. The job is scheduled to be actioned
// as soon as the removal worker picks it up.
// UUIDs are normally generated in the service layer. This is intentionally
// an exception because the charm being orphaned is determined inside this
// transaction. Pre-generating the job UUID would require a separate read
// that can become stale when a concurrent operation removes a unit. The job
// UUID is not returned, and a retried transaction rolls back the UUID from
// its previous attempt, so generating it here is safe and keeps the
// orphaning of the charm atomic with scheduling its removal.
func (st *State) scheduleCharmRemoval(ctx context.Context, tx *sqlair.TX, charmUUID string) error {
	jobUUID, err := removal.NewUUID()
	if err != nil {
		return errors.Errorf("generating charm removal job UUID: %w", err)
	}
	job := removalJob{
		UUID:          jobUUID.String(),
		RemovalTypeID: uint64(removal.CharmJob),
		EntityUUID:    charmUUID,
	}
	stmt, err := st.Prepare(`
INSERT INTO removal (uuid, removal_type_id, entity_uuid)
VALUES ($removalJob.uuid, $removalJob.removal_type_id, $removalJob.entity_uuid)
`, job)
	if err != nil {
		return errors.Capture(err)
	}
	return errors.Capture(tx.Query(ctx, stmt, job).Run())
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
