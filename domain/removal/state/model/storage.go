// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/juju/internal/errors"
)

// StorageAttachmentExists returns true if a storage attachment with the input
// UUID exists.
func (st *State) StorageAttachmentExists(ctx context.Context, saUUID string) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	attachmentUUID := entityUUID{UUID: saUUID}
	existsStmt, err := st.Prepare(`
SELECT &entityUUID.uuid
FROM   storage_attachment
WHERE  uuid = $entityUUID.uuid`, attachmentUUID)
	if err != nil {
		return false, errors.Errorf("preparing storage attachment exists query: %w", err)
	}

	var attachmentExists bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, existsStmt, attachmentUUID).Get(&attachmentUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("running storage attachment exists query: %w", err)
		}

		attachmentExists = true
		return nil
	})

	return attachmentExists, errors.Capture(err)
}

// EnsureStorageAttachmentNotAlive ensures that there is no storage attachment
// identified by the input UUID, that is still alive.
func (st *State) EnsureStorageAttachmentNotAlive(ctx context.Context, saUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	attachmentUUID := entityUUID{UUID: saUUID}
	stmt, err := st.Prepare(`
UPDATE storage_attachment
SET    life_id = 1
WHERE  uuid = $entityUUID.uuid
AND    life_id = 0`, attachmentUUID)
	if err != nil {
		return errors.Errorf("preparing storage attachment life update: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, attachmentUUID).Run()
		if err != nil {
			return errors.Errorf("advancing storage attachment life: %w", err)
		}
		return nil
	}))
}

// StorageAttachmentScheduleRemoval schedules a removal job for the storage 
// attachment with the input UUID, qualified with the input force boolean.
// We don't care if the attachment does not exist at this point because:
// - it should have been validated prior to calling this method,
// - the removal job executor will handle that fact.
func (st *State) StorageAttachmentScheduleRemoval(
	ctx context.Context, removalUUID, saUUID string, force bool, when time.Time,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	removalRec := removalJob{
		UUID:          removalUUID,
		RemovalTypeID: 6,
		EntityUUID:    saUUID,
		Force:         force,
		ScheduledFor:  when,
	}

	stmt, err := st.Prepare("INSERT INTO removal (*) VALUES ($removalJob.*)", removalRec)
	if err != nil {
		return errors.Errorf("preparing storage attachment removal: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, removalRec).Run()
		if err != nil {
			return errors.Errorf("scheduling storage attachment removal: %w", err)
		}
		return nil
	}))
}