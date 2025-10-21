// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"time"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/removal"
	"github.com/juju/juju/domain/removal/internal"
	storageerrors "github.com/juju/juju/domain/storage/errors"
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

// EnsureStorageAttachmentNotAliveWithFulfilment ensures that there is no
// storage attachment identified by the input UUID that is still alive
// after this call. This condition is only realised when the storage
// fulfilment for the units charm is met by the removal.
//
// Fulfilment expectation exists to assert the state of the world for which
// the ensure operation was computed on top of.
//
// The following errors may be returned:
// - [removalerrors.StorageFulfilmentNotMet] when the fulfilment requiremnt
// fails.
func (st *State) EnsureStorageAttachmentNotAliveWithFulfilment(
	ctx context.Context,
	saUUID string,
	fulfilment int,
) error {
	return errors.New("no implemented: coming soon")
}

// GetDetachInfoForStorageAttachment returns the information required to
// compute what a units storage requirement will look like after having
// removed the storage attachment.
//
// This information can be used to establish if detaching storage from the
// unit would violate the expectations of the unit's charm.
//
// The following errors may be returned:
// - [storageerrors.StorageAttachmentNotFound] if the storage attachment
// no longer exists in the model.
func (st *State) GetDetachInfoForStorageAttachment(
	ctx context.Context, saUUID string,
) (internal.StorageAttachmentDetachInfo, error) {
	return internal.StorageAttachmentDetachInfo{}, errors.New("no implemented: coming soon")
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
		RemovalTypeID: uint64(removal.StorageAttachmentJob),
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

// GetStorageAttachmentLife returns the life of the unit storage attachment with
// the input UUID.
func (st *State) GetStorageAttachmentLife(ctx context.Context, rUUID string) (life.Life, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return -1, errors.Capture(err)
	}

	var saLife entityLife
	saUUID := entityUUID{UUID: rUUID}

	stmt, err := st.Prepare(`
SELECT &entityLife.life_id
FROM   storage_attachment
WHERE  uuid = $entityUUID.uuid`, saLife, saUUID)
	if err != nil {
		return -1, errors.Errorf("preparing storage attachment life query: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, saUUID).Get(&saLife)
		if errors.Is(err, sqlair.ErrNoRows) {
			return storageerrors.StorageAttachmentNotFound
		} else if err != nil {
			return errors.Errorf("running storage attachment life query: %w", err)
		}

		return nil
	})
	if err != nil {
		return -1, errors.Capture(err)
	}

	return life.Life(saLife.Life), nil
}

// DeleteStorageAttachment removes a unit storage attachment from the database
// completely. If the unit attached to the storage was its owner, then that
// record is deleted too.
func (st *State) DeleteStorageAttachment(ctx context.Context, rUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	saUUID := entityUUID{UUID: rUUID}

	q := `
SELECT suo.storage_instance_uuid AS &entityUUID.uuid
FROM   storage_unit_owner suo
JOIN   storage_attachment sa
       ON  suo.storage_instance_uuid = sa.storage_instance_uuid
	   AND suo.unit_uuid = sa.unit_uuid
WHERE  sa.uuid = $entityUUID.uuid`
	suoStmt, err := st.Prepare(q, saUUID)
	if err != nil {
		return errors.Errorf("preparing unit storage owner query: %w", err)
	}

	dsaStmt, err := st.Prepare("DELETE FROM storage_attachment WHERE uuid = $entityUUID.uuid ", saUUID)
	if err != nil {
		return errors.Errorf("preparing unit storage attachment deletion: %w", err)
	}

	var siUUID entityUUID
	dsoStmt, err := st.Prepare("DELETE FROM storage_unit_owner WHERE storage_instance_uuid = $entityUUID.uuid ", siUUID)
	if err != nil {
		return errors.Errorf("preparing unit storage attachment deletion: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		unitIsOwner := true
		err = tx.Query(ctx, suoStmt, saUUID).Get(&siUUID)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				// At the time of writing, this actually never happens.
				// There is only one unit attachment to a storage instance,
				// and that unit is the owner.
				unitIsOwner = false
			} else {
				return errors.Errorf("running unit storage owner query: %w", err)
			}
		}

		err = tx.Query(ctx, dsaStmt, saUUID).Run()
		if err != nil {
			return errors.Errorf("running unit storage attachment deletion: %w", err)
		}

		if unitIsOwner {
			err = tx.Query(ctx, dsoStmt, siUUID).Run()
			if err != nil {
				return errors.Errorf("running unit storage owner deletion: %w", err)
			}
		}

		return nil
	}))
}
