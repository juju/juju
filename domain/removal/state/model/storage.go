// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"time"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/domain/removal/internal"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
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

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.ensureStorageAttachmentNotAlive(ctx, tx, saUUID)
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
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	var (
		fulfilmentDBVal count
		entityUUID      = entityUUID{UUID: saUUID}
	)

	// Notes (TLM): This sql exists to count how many storage instances are
	// currently being used to fulfil a charm storage needed on a given
	// unit. We do not consider storage attachments that are not alive in the
	// count.
	//
	// Table alias suffixed with Entity are established on to the attachment
	// being removed. Table aliases suffixed with Rel are attachments onto
	// related attachments for the same storage the entity is fulfilling.
	fulfilmentQ := `
SELECT COUNT(saRel.uuid) AS &count.count
FROM   storage_attachment saEntity
JOIN   storage_attachment saRel ON saEntity.unit_uuid = saRel.unit_uuid
JOIN   storage_instance siEntity ON saEntity.storage_instance_uuid = siEntity.uuid
JOIN   storage_instance siRel ON saRel.storage_instance_uuid = siRel.uuid
                           AND siRel.storage_name = siEntity.storage_name
AND    saEntity.uuid = $entityUUID.uuid
AND    saRel.uuid != $entityUUID.uuid
AND    saRel.life_id = 0
`

	fulfilmentStmt, err := st.Prepare(fulfilmentQ, entityUUID, fulfilmentDBVal)
	if err != nil {
		return errors.Errorf("preparing storage attachment fulfilment check: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkStorageAttachmentExists(ctx, tx, saUUID)
		if err != nil {
			return errors.Errorf("checking storage attachment %q exists: %w", saUUID, err)
		}
		if !exists {
			// If it doesn't exist then get out early. Operations after this
			// could presume existence.
			return nil
		}

		err = tx.Query(ctx, fulfilmentStmt, entityUUID).Get(&fulfilmentDBVal)
		if err != nil {
			return errors.Errorf(
				"getting current fulfilment count associated with storage attachment %q: %w",
				saUUID, err,
			)
		}

		if fulfilmentDBVal.Count != fulfilment {
			return errors.Errorf(
				"fulfilment expectation %d differs from current value %d",
				fulfilment, fulfilmentDBVal.Count,
			).Add(removalerrors.StorageFulfilmentNotMet)
		}

		return st.ensureStorageAttachmentNotAlive(ctx, tx, saUUID)
	})
	return errors.Capture(err)
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
	db, err := st.DB(ctx)
	if err != nil {
		return internal.StorageAttachmentDetachInfo{}, errors.Capture(err)
	}

	var (
		dbVal     storageAttachmentDetachInfo
		uuidInput = entityUUID{UUID: saUUID}
	)

	q := `
WITH
fulfilment AS (
    SELECT COUNT(saA.uuid) AS fulfilment
    FROM   storage_attachment saE
    JOIN   storage_attachment saA ON saE.unit_uuid = saA.unit_uuid
    JOIN   storage_instance siE ON saE.storage_instance_uuid = siE.uuid
    JOIN   storage_instance siA ON saA.storage_instance_uuid = siA.uuid
                               AND siA.storage_name = siE.storage_name
    AND    saE.uuid = $entityUUID.uuid
    AND    saA.life_id = 0
)
SELECT &storageAttachmentDetachInfo.* FROM (
    SELECT cs.name AS charm_storage_name,
           f.fulfilment AS count_fulfilment,
           cs.count_min AS required_count_min,
           sa.life_id,
           u.life_id AS unit_life_id,
           u.uuid AS unit_uuid
    FROM   storage_attachment sa, fulfilment f
    JOIN   storage_instance si ON sa.storage_instance_uuid = si.uuid
    JOIN   unit u ON sa.unit_uuid = u.uuid
    JOIN   charm_storage cs ON cs.charm_uuid = u.charm_uuid
                           AND cs.name = si.storage_name
    WHERE  sa.uuid = $entityUUID.uuid
)
`

	stmt, err := st.Prepare(q, uuidInput, dbVal)
	if err != nil {
		return internal.StorageAttachmentDetachInfo{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkStorageAttachmentExists(ctx, tx, saUUID)
		if err != nil {
			return errors.Errorf(
				"checking if storage attachment exists: %w", err,
			)
		}
		if !exists {
			return errors.Errorf(
				"storage attachment %q does not exist in the model", saUUID,
			).Add(storageerrors.StorageAttachmentNotFound)
		}

		return tx.Query(ctx, stmt, uuidInput).Get(&dbVal)
	})
	if err != nil {
		return internal.StorageAttachmentDetachInfo{}, errors.Capture(err)
	}

	return internal.StorageAttachmentDetachInfo{
		CharmStorageName: dbVal.CharmStorageName,
		CountFulfilment:  dbVal.CountFulfilment,
		Life:             dbVal.LifeID,
		RequiredCountMin: dbVal.RequiredCountMin,
		UnitLife:         dbVal.UnitLifeID,
		UnitUUID:         dbVal.UnitUUID,
	}, nil
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
//
// The following errors may be returned:
// - [storageerrors.StorageAttachmentNotFound] if the storage attachment
// no longer exists in the model.
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

// EnsureStorageAttachmentDeadCascade ensures that the storage attachment is
// dead and that all filesystem attachments, volume attachments and volume
// attachment plans are dying.
//
// The following errors may be returned:
// - [storageerrors.StorageAttachmentNotFound] when the storage attachment for
// the given UUID is not found.
func (st *State) EnsureStorageAttachmentDeadCascade(
	ctx context.Context, uuid string,
) (internal.CascadedStorageProvisionedAttachmentLives, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return internal.CascadedStorageProvisionedAttachmentLives{}, errors.Capture(err)
	}

	saUUID := entityUUID{UUID: uuid}
	existsStmt, err := st.Prepare(`
SELECT &entityUUID.uuid
FROM   storage_attachment
WHERE  uuid = $entityUUID.uuid`, saUUID)
	if err != nil {
		return internal.CascadedStorageProvisionedAttachmentLives{}, errors.Errorf(
			"preparing storage attachment exists query: %w", err,
		)
	}

	var cascaded internal.CascadedStorageProvisionedAttachmentLives
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, existsStmt, saUUID).Get(&saUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return storageerrors.StorageAttachmentNotFound
		} else if err != nil {
			return errors.Errorf(
				"running storage attachment exists query: %w", err,
			)
		}

		cascaded, err = st.ensureStorageAttachmentDead(ctx, tx, saUUID)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return internal.CascadedStorageProvisionedAttachmentLives{}, errors.Capture(err)
	}

	return cascaded, nil
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

// EnsureStorageInstanceNotAliveCascade ensures that there is no storage
// instance identified by the input UUID, that is still alive.
//
// The following errors may be returned:
// - [storageerrors.StorageInstanceNotFound] if the storage instance no longer
// exists in the model.
func (st *State) EnsureStorageInstanceNotAliveCascade(
	ctx context.Context, siUUID string, obliterate bool,
) (internal.CascadedStorageFilesystemVolumeLives, error) {
	var cascaded internal.CascadedStorageFilesystemVolumeLives
	db, err := st.DB(ctx)
	if err != nil {
		return cascaded, errors.Capture(err)
	}

	input := entityUUID{UUID: siUUID}

	existsStmt, err := st.Prepare(`
SELECT &entityUUID.uuid
FROM   storage_instance
WHERE  uuid = $entityUUID.uuid`, input)
	if err != nil {
		return cascaded, errors.Errorf(
			"preparing storage instance exists query: %w", err,
		)
	}

	attachmentCountStmt, err := st.Prepare(`
SELECT COUNT(*) AS &count.count
FROM   storage_attachment
WHERE  storage_instance_uuid = $entityUUID.uuid`, input, count{})
	if err != nil {
		return cascaded, errors.Errorf(
			"preparing storage attachment count query: %w", err,
		)
	}

	var result internal.CascadedStorageInstanceLives
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, existsStmt, input).Get(&input)
		if errors.Is(err, sqlair.ErrNoRows) {
			return storageerrors.StorageInstanceNotFound
		} else if err != nil {
			return errors.Errorf(
				"running storage instance exists query: %w", err,
			)
		}

		cnt := count{}
		err = tx.Query(ctx, attachmentCountStmt, input).Get(&cnt)
		if err != nil {
			return errors.Errorf(
				"running storage attachment count query: %w", err,
			)
		}
		if cnt.Count > 0 {
			return removalerrors.StorageInstanceStillAttached
		}

		result, err = st.ensureStorageInstancesNotAliveCascade(
			ctx, tx, []entityUUID{input}, obliterate)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return cascaded, errors.Capture(err)
	}

	// TODO(storage): re-write this singular case to use hand crafted queries.
	if l := len(result.FileSystemUUIDs); l > 1 {
		return cascaded, errors.Errorf(
			"unexpected number of fs for singular storage instance %q removal",
			siUUID,
		)
	} else if l == 1 {
		cascaded.FileSystemUUID = &result.FileSystemUUIDs[0]
	}
	if l := len(result.VolumeUUIDs); l > 1 {
		return cascaded, errors.Errorf(
			"unexpected number of vol for singular storage instance %q removal",
			siUUID,
		)
	} else if l == 1 {
		cascaded.VolumeUUID = &result.VolumeUUIDs[0]
	}

	return cascaded, nil
}

// GetStorageInstanceLife returns the life of the storage instance with the
// input UUID.
//
// The following errors may be returned:
// - [storageerrors.StorageInstanceNotFound] if the storage instance no longer
// exists in the model.
func (st *State) GetStorageInstanceLife(
	ctx context.Context, siUUID string,
) (life.Life, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return -1, errors.Capture(err)
	}

	var saLife entityLife
	input := entityUUID{UUID: siUUID}

	stmt, err := st.Prepare(`
SELECT &entityLife.life_id
FROM   storage_instance
WHERE  uuid = $entityUUID.uuid`, saLife, input)
	if err != nil {
		return -1, errors.Errorf("preparing storage instance life query: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, input).Get(&saLife)
		if errors.Is(err, sqlair.ErrNoRows) {
			return storageerrors.StorageInstanceNotFound
		} else if err != nil {
			return errors.Errorf("running storage instance life query: %w", err)
		}

		return nil
	})
	if err != nil {
		return -1, errors.Capture(err)
	}

	return life.Life(saLife.Life), nil
}
// GetVolumeLife returns the life of the volume with the input UUID.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeNotFound] when no volume exists for the
// uuid.
func (st *State) GetVolumeLife(
	ctx context.Context, rUUID string,
) (life.Life, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return -1, errors.Capture(err)
	}

	var volLife entityLife
	volUUID := entityUUID{UUID: rUUID}

	stmt, err := st.Prepare(`
SELECT &entityLife.life_id
FROM   storage_volume
WHERE  uuid = $entityUUID.uuid`, volLife, volUUID)
	if err != nil {
		return -1, errors.Errorf("preparing volume life query: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, volUUID).Get(&volLife)
		if errors.Is(err, sqlair.ErrNoRows) {
			return storageprovisioningerrors.VolumeNotFound
		} else if err != nil {
			return errors.Errorf("running volume life query: %w", err)
		}

		return nil
	})
	if err != nil {
		return -1, errors.Capture(err)
	}

	return life.Life(volLife.Life), nil
}

// VolumeScheduleRemoval schedules a removal job for the volume with the input
// UUID, qualified with the input force boolean.
// We don't care if the volume does not exist at this point because:
// - it should have been validated prior to calling this method,
// - the removal job executor will handle that fact.
func (st *State) VolumeScheduleRemoval(
	ctx context.Context, removalUUID, volUUID string, force bool, when time.Time,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	removalRec := removalJob{
		UUID:          removalUUID,
		RemovalTypeID: uint64(removal.StorageVolumeJob),
		EntityUUID:    volUUID,
		Force:         force,
		ScheduledFor:  when,
	}

	stmt, err := st.Prepare("INSERT INTO removal (*) VALUES ($removalJob.*)", removalRec)
	if err != nil {
		return errors.Errorf("preparing volume removal: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, removalRec).Run()
		if err != nil {
			return errors.Errorf("scheduling volume removal: %w", err)
		}
		return nil
	}))
}

// DeleteVolume deletes the volume specified by the input UUID. It also deletes
// the storage instance volume relation if it still exists.
func (st *State) DeleteVolume(ctx context.Context, rUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	volUUID := entityUUID{UUID: rUUID}

	deleteStorageInstanceVolumeStmt, err := st.Prepare(`
DELETE FROM storage_instance_volume WHERE storage_volume_uuid = $entityUUID.uuid
`, volUUID)
	if err != nil {
		return errors.Errorf(
			"preparing in storage instance volume deletion: %w", err,
		)
	}

	deleteVolumeStatusStmt, err := st.Prepare(`
DELETE FROM storage_volume_status WHERE volume_uuid = $entityUUID.uuid
`, volUUID)
	if err != nil {
		return errors.Errorf(
			"preparing in volume status deletion: %w", err,
		)
	}

	deleteVolumeStmt, err := st.Prepare(`
DELETE FROM storage_volume WHERE uuid = $entityUUID.uuid
`, volUUID)
	if err != nil {
		return errors.Errorf("preparing volumee deletion: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, deleteStorageInstanceVolumeStmt, volUUID).Run()
		if err != nil {
			return errors.Errorf(
				"deleting storage instance volume: %w", err,
			)
		}
		err = tx.Query(ctx, deleteVolumeStatusStmt, volUUID).Run()
		if err != nil {
			return errors.Errorf("deleting volume status: %w", err)
		}
		err = tx.Query(ctx, deleteVolumeStmt, volUUID).Run()
		if err != nil {
			return errors.Errorf("deleting volume: %w", err)
		}
		return nil
	})

	return errors.Capture(err)
}

// GetFilesystemLife returns the life of the filesystem with the input UUID.
//
// The following errors may be returned:
// - [storageprovisioningerrors.FilesystemNotFound] when no filesystem exists
// for the uuid.
func (st *State) GetFilesystemLife(
	ctx context.Context, rUUID string,
) (life.Life, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return -1, errors.Capture(err)
	}

	var fsLife entityLife
	fsUUID := entityUUID{UUID: rUUID}

	stmt, err := st.Prepare(`
SELECT &entityLife.life_id
FROM   storage_filesystem
WHERE  uuid = $entityUUID.uuid`, fsLife, fsUUID)
	if err != nil {
		return -1, errors.Errorf("preparing filesystem life query: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, fsUUID).Get(&fsLife)
		if errors.Is(err, sqlair.ErrNoRows) {
			return storageprovisioningerrors.FilesystemNotFound
		} else if err != nil {
			return errors.Errorf("running filesystem life query: %w", err)
		}

		return nil
	})
	if err != nil {
		return -1, errors.Capture(err)
	}

	return life.Life(fsLife.Life), nil
}

// FilesystemScheduleRemoval schedules a removal job for the filesystem with the
// input UUID, qualified with the input force boolean.
// We don't care if the filesystem does not exist at this point because:
// - it should have been validated prior to calling this method,
// - the removal job executor will handle that fact.
func (st *State) FilesystemScheduleRemoval(
	ctx context.Context, removalUUID, fsUUID string, force bool, when time.Time,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	removalRec := removalJob{
		UUID:          removalUUID,
		RemovalTypeID: uint64(removal.StorageFilesystemJob),
		EntityUUID:    fsUUID,
		Force:         force,
		ScheduledFor:  when,
	}

	stmt, err := st.Prepare("INSERT INTO removal (*) VALUES ($removalJob.*)", removalRec)
	if err != nil {
		return errors.Errorf("preparing filesystem removal: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, removalRec).Run()
		if err != nil {
			return errors.Errorf("scheduling filesystem removal: %w", err)
		}
		return nil
	}))
}

// DeleteFilesystem deletes the filesystem specified by the input UUID. It also
// deletes the storage instance filesystem relation if it still exists.
func (st *State) DeleteFilesystem(ctx context.Context, rUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	fsUUID := entityUUID{UUID: rUUID}

	deleteStorageInstanceFilesystemStmt, err := st.Prepare(`
DELETE FROM storage_instance_filesystem WHERE storage_filesystem_uuid = $entityUUID.uuid
`, fsUUID)
	if err != nil {
		return errors.Errorf(
			"preparing in storage instance filesystem deletion: %w", err,
		)
	}

	deleteFilesystemStatusStmt, err := st.Prepare(`
DELETE FROM storage_filesystem_status WHERE filesystem_uuid = $entityUUID.uuid
`, fsUUID)
	if err != nil {
		return errors.Errorf(
			"preparing in filesystem status deletion: %w", err,
		)
	}

	deleteFilesystemStmt, err := st.Prepare(`
DELETE FROM storage_filesystem WHERE uuid = $entityUUID.uuid
`, fsUUID)
	if err != nil {
		return errors.Errorf("preparing filesystem deletion: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, deleteStorageInstanceFilesystemStmt, fsUUID).Run()
		if err != nil {
			return errors.Errorf(
				"deleting storage instance filesystem: %w", err,
			)
		}
		err = tx.Query(ctx, deleteFilesystemStatusStmt, fsUUID).Run()
		if err != nil {
			return errors.Errorf("deleting filesystem status: %w", err)
		}
		err = tx.Query(ctx, deleteFilesystemStmt, fsUUID).Run()
		if err != nil {
			return errors.Errorf("deleting filesystem: %w", err)
		}
		return nil
	})

	return errors.Capture(err)
}

// GetFilesystemAttachmentLife returns the life of the filesystem attachment
// indicated by the supplied UUID.
//
// The following errors may be returned:
// - [storageprovisioningerrors.FilesystemAttachmentNotFound] there is no
// filesystem attachment for the provided UUID.
func (st *State) GetFilesystemAttachmentLife(
	ctx context.Context, rUUID string,
) (life.Life, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return -1, errors.Capture(err)
	}

	var fsaLife entityLife
	fsaUUID := entityUUID{UUID: rUUID}

	stmt, err := st.Prepare(`
SELECT &entityLife.life_id
FROM   storage_filesystem_attachment
WHERE  uuid = $entityUUID.uuid`, fsaLife, fsaUUID)
	if err != nil {
		return -1, errors.Errorf(
			"preparing filesystem attachment life query: %w", err,
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, fsaUUID).Get(&fsaLife)
		if errors.Is(err, sqlair.ErrNoRows) {
			return storageprovisioningerrors.FilesystemAttachmentNotFound
		} else if err != nil {
			return errors.Errorf(
				"running filesystem attachment life query: %w", err,
			)
		}

		return nil
	})
	if err != nil {
		return -1, errors.Capture(err)
	}

	return life.Life(fsaLife.Life), nil
}

// MarkFilesystemAttachmentAsDead updates the life to dead of the filesystem
// attachment indicated by the supplied UUID.
//
// The following errors may be returned:
// - [storageprovisioningerrors.FilesystemAttachmentNotFound] there is no
// filesystem attachment for the provided UUID.
func (st *State) MarkFilesystemAttachmentAsDead(
	ctx context.Context, rUUID string,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	fsaUUID := entityUUID{UUID: rUUID}
	existsStmt, err := st.Prepare(`
SELECT &entityUUID.uuid
FROM   storage_filesystem_attachment
WHERE  uuid = $entityUUID.uuid`, fsaUUID)
	if err != nil {
		return errors.Errorf(
			"preparing filesystem attachment exists query: %w", err,
		)
	}

	markDeadStmt, err := st.Prepare(`
UPDATE storage_filesystem_attachment
SET    life_id = 2
WHERE  uuid = $entityUUID.uuid`, fsaUUID)
	if err != nil {
		return errors.Errorf(
			"preparing filesystem attachment exists query: %w", err,
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, existsStmt, fsaUUID).Get(&fsaUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return storageprovisioningerrors.FilesystemAttachmentNotFound
		} else if err != nil {
			return errors.Errorf(
				"running filesystem attachment exists query: %w", err,
			)
		}

		err = tx.Query(ctx, markDeadStmt, fsaUUID).Run()
		if err != nil {
			return errors.Errorf(
				"running mark filesystem attachment dead query: %w", err,
			)
		}

		return nil
	})
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// GetVolumeAttachmentLife returns the life of the volume attachment indicated
// by the supplied UUID.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeAttachmentNotFound] there is no volume
// attachment for the provided UUID.
func (st *State) GetVolumeAttachmentLife(
	ctx context.Context, rUUID string,
) (life.Life, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return -1, errors.Capture(err)
	}

	var vaLife entityLife
	vaUUID := entityUUID{UUID: rUUID}

	stmt, err := st.Prepare(`
SELECT &entityLife.life_id
FROM   storage_volume_attachment
WHERE  uuid = $entityUUID.uuid`, vaLife, vaUUID)
	if err != nil {
		return -1, errors.Errorf(
			"preparing volume attachment life query: %w", err,
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, vaUUID).Get(&vaLife)
		if errors.Is(err, sqlair.ErrNoRows) {
			return storageprovisioningerrors.VolumeAttachmentNotFound
		} else if err != nil {
			return errors.Errorf(
				"running volume attachment life query: %w", err,
			)
		}

		return nil
	})
	if err != nil {
		return -1, errors.Capture(err)
	}

	return life.Life(vaLife.Life), nil
}

// MarkVolumeAttachmentAsDead updates the life to dead of the volume attachment
// indicated by the supplied UUID.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeAttachmentNotFound] there is no
// filesystem attachment for the provided UUID.
func (st *State) MarkVolumeAttachmentAsDead(
	ctx context.Context, rUUID string,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	vaUUID := entityUUID{UUID: rUUID}
	existsStmt, err := st.Prepare(`
SELECT &entityUUID.uuid
FROM   storage_volume_attachment
WHERE  uuid = $entityUUID.uuid`, vaUUID)
	if err != nil {
		return errors.Errorf(
			"preparing volume attachment exists query: %w", err,
		)
	}

	markDeadStmt, err := st.Prepare(`
UPDATE storage_volume_attachment
SET    life_id = 2
WHERE  uuid = $entityUUID.uuid`, vaUUID)
	if err != nil {
		return errors.Errorf(
			"preparing volume attachment exists query: %w", err,
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, existsStmt, vaUUID).Get(&vaUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return storageprovisioningerrors.VolumeAttachmentNotFound
		} else if err != nil {
			return errors.Errorf(
				"running volume attachment exists query: %w", err,
			)
		}

		err = tx.Query(ctx, markDeadStmt, vaUUID).Run()
		if err != nil {
			return errors.Errorf(
				"running mark volume attachment dead query: %w", err,
			)
		}

		return nil
	})
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// GetVolumeAttachmentPlanLife returns the life of the volume attachment plan
// indicated by the supplied UUID.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeAttachmentPlanNotFound] there is no
// volume attachment plan for the provided UUID.
func (st *State) GetVolumeAttachmentPlanLife(
	ctx context.Context, rUUID string,
) (life.Life, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return -1, errors.Capture(err)
	}

	var vapLife entityLife
	vapUUID := entityUUID{UUID: rUUID}

	stmt, err := st.Prepare(`
SELECT &entityLife.life_id
FROM   storage_volume_attachment_plan
WHERE  uuid = $entityUUID.uuid`, vapLife, vapUUID)
	if err != nil {
		return -1, errors.Errorf(
			"preparing volume attachment plan life query: %w", err,
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, vapUUID).Get(&vapLife)
		if errors.Is(err, sqlair.ErrNoRows) {
			return storageprovisioningerrors.VolumeAttachmentPlanNotFound
		} else if err != nil {
			return errors.Errorf(
				"running volume attachment plan life query: %w", err,
			)
		}

		return nil
	})
	if err != nil {
		return -1, errors.Capture(err)
	}

	return life.Life(vapLife.Life), nil
}

// MarkVolumeAttachmentPlanAsDead updates the life to dead of the volume
// attachment plan indicated by the supplied UUID.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeAttachmentPlanNotFound] there is no
// filesystem attachment for the provided UUID.
func (st *State) MarkVolumeAttachmentPlanAsDead(
	ctx context.Context, rUUID string,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	vapUUID := entityUUID{UUID: rUUID}
	existsStmt, err := st.Prepare(`
SELECT &entityUUID.uuid
FROM   storage_volume_attachment_plan
WHERE  uuid = $entityUUID.uuid`, vapUUID)
	if err != nil {
		return errors.Errorf(
			"preparing volume attachment plan exists query: %w", err,
		)
	}

	markDeadStmt, err := st.Prepare(`
UPDATE storage_volume_attachment_plan
SET    life_id = 2
WHERE  uuid = $entityUUID.uuid`, vapUUID)
	if err != nil {
		return errors.Errorf(
			"preparing volume attachment plan exists query: %w", err,
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, existsStmt, vapUUID).Get(&vapUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return storageprovisioningerrors.VolumeAttachmentPlanNotFound
		} else if err != nil {
			return errors.Errorf(
				"running volume attachment plan exists query: %w", err,
			)
		}

		err = tx.Query(ctx, markDeadStmt, vapUUID).Run()
		if err != nil {
			return errors.Errorf(
				"running mark volume attachment plan dead query: %w", err,
			)
		}

		return nil
	})
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// checkStorageAttachmentExists is a internal transaction helper for verifying
// if a storage attachment by the supplied uuid exists.
func (st *State) checkStorageAttachmentExists(
	ctx context.Context, tx *sqlair.TX, saUUID string,
) (bool, error) {
	uuidInput := entityUUID{UUID: saUUID}

	checkQ := `
SELECT &entityUUID.*
FROM   storage_attachment
WHERE  uuid = $entityUUID.uuid
`
	stmt, err := st.Prepare(checkQ, uuidInput)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, uuidInput).Get(&uuidInput)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}

	return true, nil
}

// ensureStorageInstanceNotAliveCascade ensures that the storage instance
// identitied by the input UUID is no longer alive.
// If any of the instance's volumes or file-systems have a provisioning scope
// of "machine", those entities will also no longer be alive.
// All attachments to those volumes or file-systems will no longer be alive.
// All entities who's lives were advanced are indicated in the return.
func (st *State) ensureStorageInstanceNotAliveCascade(
	ctx context.Context, tx *sqlair.TX, siUUID entityUUID,
) (internal.CascadedStorageInstanceLives, error) {
	var cascaded internal.CascadedStorageInstanceLives

	// First kill the storage instance.
	stmt, err := st.Prepare("UPDATE storage_instance SET life_id = 1 WHERE uuid = $entityUUID.uuid", siUUID)
	if err != nil {
		return cascaded, errors.Errorf("preparing storage instance life update: %w", err)
	}
	if err := tx.Query(ctx, stmt, siUUID).Run(); err != nil {
		return cascaded, errors.Errorf("running storage instance life update: %w", err)
	}

	qry := `
SELECT &entityUUID.uuid
FROM   storage_instance_filesystem f
       JOIN storage_filesystem_attachment a ON f.storage_filesystem_uuid = a.storage_filesystem_uuid
WHERE  f.storage_instance_uuid = $entityUUID.uuid
AND    a.life_id = 0`

	del := "UPDATE storage_filesystem_attachment SET life_id = 1 WHERE uuid = $entityUUID.uuid"

	if cascaded.FileSystemAttachmentUUID, err = st.ensureStorageEntityNotAlive(
		ctx, tx, siUUID, "file-system attachment", qry, del,
	); err != nil {
		return cascaded, errors.Capture(err)
	}

	qry = `
SELECT &entityUUID.uuid
FROM   storage_instance_volume v
       JOIN storage_volume_attachment a ON v.storage_volume_uuid = a.storage_volume_uuid
WHERE  v.storage_instance_uuid = $entityUUID.uuid
AND    a.life_id = 0`

	del = "UPDATE storage_volume_attachment SET life_id = 1 WHERE uuid = $entityUUID.uuid"

	if cascaded.VolumeAttachmentUUID, err = st.ensureStorageEntityNotAlive(
		ctx, tx, siUUID, "volume attachment", qry, del,
	); err != nil {
		return cascaded, errors.Capture(err)
	}

	qry = `
SELECT &entityUUID.uuid
FROM   storage_instance_volume v
       JOIN storage_volume_attachment_plan a ON v.storage_volume_uuid = a.storage_volume_uuid
WHERE  v.storage_instance_uuid = $entityUUID.uuid
AND    a.life_id = 0`

	del = "UPDATE storage_volume_attachment_plan SET life_id = 1 WHERE uuid = $entityUUID.uuid"

	if cascaded.VolumeAttachmentPlanUUID, err = st.ensureStorageEntityNotAlive(
		ctx, tx, siUUID, "volume attachment plan", qry, del,
	); err != nil {
		return cascaded, errors.Capture(err)
	}

	// File-systems are set to "dying" if they were provisioned with
	// "machine" scope *unless* they are volume-backed and the volume
	// was not provisioned with "machine" scope.
	qry = `
SELECT f.uuid AS &entityUUID.uuid
FROM   storage_instance_filesystem i
       JOIN storage_filesystem f ON i.storage_filesystem_uuid = f.uuid
	   LEFT JOIN storage_instance_volume iv ON i.storage_instance_uuid = iv.storage_instance_uuid
	   LEFT JOIN storage_volume v ON iv.storage_volume_uuid = v.uuid AND v.provision_scope_id = 0
WHERE  i.storage_instance_uuid = $entityUUID.uuid
AND    v.uuid IS NULL
AND    f.provision_scope_id = 1
AND    f.life_id = 0`

	del = "UPDATE storage_filesystem SET life_id = 1 WHERE uuid = $entityUUID.uuid"

	if cascaded.FileSystemUUID, err = st.ensureStorageEntityNotAlive(
		ctx, tx, siUUID, "file-system", qry, del,
	); err != nil {
		return cascaded, errors.Capture(err)
	}

	qry = `
SELECT &entityUUID.uuid
FROM   storage_instance_volume i
       JOIN storage_volume v ON i.storage_volume_uuid = v.uuid
WHERE  i.storage_instance_uuid = $entityUUID.uuid
AND    v.provision_scope_id = 1
AND    v.life_id = 0`

	del = "UPDATE storage_volume SET life_id = 1 WHERE uuid = $entityUUID.uuid"

	if cascaded.VolumeUUID, err = st.ensureStorageEntityNotAlive(
		ctx, tx, siUUID, "volume", qry, del,
	); err != nil {
		return cascaded, errors.Capture(err)
	}

	return cascaded, nil
}

func (st *State) ensureStorageAttachmentDead(
	ctx context.Context, tx *sqlair.TX, saUUID entityUUID,
) (internal.CascadedStorageProvisionedAttachmentLives, error) {
	var cascaded internal.CascadedStorageProvisionedAttachmentLives

	stmt, err := st.Prepare(`
SELECT &entityUUID.*
FROM   storage_attachment
WHERE  uuid = $entityUUID.uuid
AND    life_id = 1`, saUUID)
	if err != nil {
		return cascaded, errors.Errorf(
			"preparing dying storage attachment query: %w", err,
		)
	}

	err = tx.Query(ctx, stmt, saUUID).Get(&saUUID)
	if errors.Is(err, sqlair.ErrNoRows) {
		return cascaded, nil
	} else if err != nil {
		return cascaded, errors.Errorf(
			"running dying storage attachments query: %w", err,
		)
	}

	stmt, err = st.Prepare(`
UPDATE storage_attachment
SET    life_id = 2
WHERE  uuid = $entityUUID.uuid`, saUUID)
	if err != nil {
		return cascaded, errors.Errorf(
			"preparing live storage attachment update: %w", err,
		)
	}

	err = tx.Query(ctx, stmt, saUUID).Run()
	if err != nil {
		return cascaded, errors.Errorf(
			"running live storage attachment update: %w", err,
		)
	}

	sfaStmt, err := st.Prepare(`
SELECT sfa.uuid AS &entityUUID.uuid
FROM   storage_attachment sa
       JOIN storage_instance_filesystem sif ON sa.storage_instance_uuid = sif.storage_instance_uuid
       JOIN storage_filesystem_attachment sfa ON sif.storage_filesystem_uuid = sfa.storage_filesystem_uuid
WHERE  sa.uuid = $entityUUID.uuid
AND    sfa.life_id = 0`, entityUUID{})
	if err != nil {
		return cascaded, errors.Errorf(
			"preparing live filesystem attachments query: %w", err,
		)
	}

	var sfaUUIDs entityUUIDs
	err = tx.Query(ctx, sfaStmt, saUUID).GetAll(&sfaUUIDs)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return cascaded, errors.Errorf(
			"running live filesystem attachments query: %w", err,
		)
	}

	for _, v := range sfaUUIDs {
		cascaded.FileSystemAttachmentUUIDs = append(
			cascaded.FileSystemAttachmentUUIDs, v.UUID)
	}

	if len(sfaUUIDs) > 0 {
		input := sfaUUIDs.uuids()

		sfaDelStmt, err := st.Prepare(`
	UPDATE storage_filesystem_attachment
	SET    life_id = 1
	WHERE  uuid IN ($uuids[:])
	`, input)
		if err != nil {
			return cascaded, errors.Errorf(
				"preparing live filesystem attachments update: %w", err,
			)
		}

		err = tx.Query(ctx, sfaDelStmt, input).Run()
		if err != nil {
			return cascaded, errors.Errorf(
				"running live filesystem attachments update: %w", err,
			)
		}
	}

	svaStmt, err := st.Prepare(`
SELECT sva.uuid AS &entityUUID.uuid
FROM   storage_attachment sa
       JOIN storage_instance_volume siv ON sa.storage_instance_uuid = siv.storage_instance_uuid
       JOIN storage_volume_attachment sva ON siv.storage_volume_uuid = sva.storage_volume_uuid
WHERE  sa.uuid = $entityUUID.uuid
AND    sva.life_id = 0`, entityUUID{})
	if err != nil {
		return cascaded, errors.Errorf(
			"preparing live volume attachments query: %w", err,
		)
	}

	var svaUUIDs entityUUIDs
	err = tx.Query(ctx, svaStmt, saUUID).GetAll(&svaUUIDs)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return cascaded, errors.Errorf(
			"running live unit volume attachments query: %w", err,
		)
	}

	for _, v := range svaUUIDs {
		cascaded.VolumeAttachmentUUIDs = append(
			cascaded.VolumeAttachmentUUIDs, v.UUID)
	}

	if len(svaUUIDs) > 0 {
		input := svaUUIDs.uuids()

		svaDelStmt, err := st.Prepare(`
	UPDATE storage_volume_attachment
	SET    life_id = 1
	WHERE  uuid IN ($uuids[:])
	`, input)
		if err != nil {
			return cascaded, errors.Errorf(
				"preparing live volume attachments update: %w", err,
			)
		}

		err = tx.Query(ctx, svaDelStmt, input).Run()
		if err != nil {
			return cascaded, errors.Errorf(
				"running live volume attachments update: %w", err,
			)
		}
	}

	svapStmt, err := st.Prepare(`
SELECT svap.uuid AS &entityUUID.uuid
FROM   storage_attachment sa
       JOIN storage_instance_volume siv ON sa.storage_instance_uuid = siv.storage_instance_uuid
       JOIN storage_volume_attachment_plan svap ON siv.storage_volume_uuid = svap.storage_volume_uuid
WHERE  sa.uuid = $entityUUID.uuid
AND    svap.life_id = 0`, saUUID)
	if err != nil {
		return cascaded, errors.Errorf(
			"preparing live volume attachment plans query: %w", err,
		)
	}

	var svapUUIDs entityUUIDs
	err = tx.Query(ctx, svapStmt, saUUID).GetAll(&svapUUIDs)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return cascaded, errors.Errorf(
			"running live volume attachment plans query: %w", err,
		)
	}

	for _, v := range svapUUIDs {
		cascaded.VolumeAttachmentPlanUUIDs = append(
			cascaded.VolumeAttachmentPlanUUIDs, v.UUID)
	}

	if len(svapUUIDs) > 0 {
		input := svapUUIDs.uuids()

		svapDelStmt, err := st.Prepare(`
	UPDATE storage_volume_attachment_plan
	SET    life_id = 1
	WHERE  uuid IN ($uuids[:])
	`, sqlair.S{})
		if err != nil {
			return cascaded, errors.Errorf(
				"preparing live volume attachment plans update: %w", err,
			)
		}

		err = tx.Query(ctx, svapDelStmt, input).Run()
		if err != nil {
			return cascaded, errors.Errorf(
				"running live volume attachment plans update: %w", err,
			)
		}
	}

	return cascaded, nil
}

func (st *State) ensureStorageEntitiesNotAlive(
	ctx context.Context, tx *sqlair.TX, siUUID entityUUIDs, entityType, qry, del string,
) ([]string, error) {
	stmt, err := st.Prepare(qry, uuids{}, entityUUID{})
	if err != nil {
		return nil, errors.Errorf("preparing %s query: %w", entityType, err)
	}

	var dying entityUUIDs
	err = tx.Query(ctx, stmt, siUUID.uuids()).GetAll(&dying)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Errorf("running %s query: %w", entityType, err)
	}

	input := dying.uuids()
	if stmt, err = st.Prepare(del, input); err != nil {
		return nil, errors.Errorf("preparing %s life update: %w", entityType, err)
	}
	if err := tx.Query(ctx, stmt, input).Run(); err != nil {
		return nil, errors.Errorf("running %s life update: %w", entityType, err)
	}

	return input, nil
}

// EnsureStorageAttachmentNotAlive ensures that there is no storage attachment
// identified by the input UUID, that is still alive.
func (st *State) ensureStorageAttachmentNotAlive(
	ctx context.Context, tx *sqlair.TX, saUUID string,
) error {
	attachmentUUID := entityUUID{UUID: saUUID}
	stmt, err := st.Prepare(`
UPDATE storage_attachment
SET    life_id = 1
WHERE  uuid = $entityUUID.uuid
AND    life_id = 0`, attachmentUUID)
	if err != nil {
		return errors.Errorf("preparing storage attachment life update: %w", err)
	}

	err = tx.Query(ctx, stmt, attachmentUUID).Run()
	if err != nil {
		return errors.Errorf("advancing storage attachment life: %w", err)
	}
	return nil
}
