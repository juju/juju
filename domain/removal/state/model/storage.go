// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"slices"
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
// Passing cascadeForce will cause this to always return the filesystem and/or
// volume.
//
// The following errors may be returned:
// - [storageerrors.StorageInstanceNotFound] if the storage instance no longer
// exists in the model.
func (st *State) EnsureStorageInstanceNotAliveCascade(
	ctx context.Context, siUUID string, obliterate bool, cascadeForce bool,
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

	forceFilesystemStmt, err := st.Prepare(`
SELECT sf.uuid AS &entityUUID.uuid
FROM   storage_instance_filesystem sif
JOIN   storage_filesystem sf ON sif.storage_filesystem_uuid = sf.uuid
WHERE  sif.storage_instance_uuid = $entityUUID.uuid
`, entityUUID{})
	if err != nil {
		return cascaded, errors.Errorf(
			"preparing filesystem query: %w", err,
		)
	}

	forceVolumeStmt, err := st.Prepare(`
SELECT sv.uuid AS &entityUUID.uuid
FROM   storage_instance_volume siv
JOIN   storage_volume sv ON siv.storage_volume_uuid = sv.uuid
WHERE  siv.storage_instance_uuid = $entityUUID.uuid
`, entityUUID{})
	if err != nil {
		return cascaded, errors.Errorf(
			"preparing volume query: %w", err,
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

		if !cascadeForce {
			return nil
		}

		// Always return the filesystem if this a cascade force.
		fsUUID := entityUUID{}
		err = tx.Query(ctx, forceFilesystemStmt, input).Get(&fsUUID)
		if err == nil {
			result.FileSystemUUIDs = append(result.FileSystemUUIDs, fsUUID.UUID)
		} else if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"running filesystem query: %w", err,
			)
		}

		// Always return the volume if this a cascade force.
		volUUID := entityUUID{}
		err = tx.Query(ctx, forceVolumeStmt, input).Get(&volUUID)
		if err == nil {
			result.VolumeUUIDs = append(result.VolumeUUIDs, volUUID.UUID)
		} else if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"running volume query: %w", err,
			)
		}

		return nil
	})
	if err != nil {
		return cascaded, errors.Capture(err)
	}

	// TODO(storage): re-write this singular case to use hand crafted queries.
	slices.Sort(result.FileSystemUUIDs)
	result.FileSystemUUIDs = slices.Compact(result.FileSystemUUIDs)
	if n := len(result.FileSystemUUIDs); n > 1 {
		return cascaded, errors.Errorf(
			"unexpected number of fs for singular storage instance %q removal",
			siUUID,
		)
	} else if n == 1 {
		cascaded.FileSystemUUID = &result.FileSystemUUIDs[0]
	}

	slices.Sort(result.VolumeUUIDs)
	result.VolumeUUIDs = slices.Compact(result.VolumeUUIDs)
	if n := len(result.VolumeUUIDs); n > 1 {
		return cascaded, errors.Errorf(
			"unexpected number of vol for singular storage instance %q removal",
			siUUID,
		)
	} else if n == 1 {
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

// StorageInstanceScheduleRemoval schedules a removal job for the storage
// instance with the input UUID, qualified with the input force boolean.
//
// We don't care if the storage instance does not exist at this point because:
// - it should have been validated prior to calling this method,
// - the removal job executor will handle that fact.
func (st *State) StorageInstanceScheduleRemoval(
	ctx context.Context, removalUUID, siUUID string, force bool, when time.Time,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	removalRec := removalJob{
		UUID:          removalUUID,
		RemovalTypeID: uint64(removal.StorageInstanceJob),
		EntityUUID:    siUUID,
		Force:         force,
		ScheduledFor:  when,
	}

	stmt, err := st.Prepare("INSERT INTO removal (*) VALUES ($removalJob.*)", removalRec)
	if err != nil {
		return errors.Errorf("preparing storage instance removal: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, removalRec).Run()
		if err != nil {
			return errors.Errorf("scheduling storage instance removal: %w", err)
		}
		return nil
	}))
}

// CheckStorageInstanceHasNoChildren returns true if the storage instance
// with the input UUID has no child filesystem or volume.
func (st *State) CheckStorageInstanceHasNoChildren(
	ctx context.Context, siUUID string,
) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	input := entityUUID{UUID: siUUID}

	stmt, err := st.Prepare(`
SELECT SUM(n) AS &count.count FROM (
	SELECT    COUNT(*) AS n
	FROM      storage_instance_volume
	WHERE     storage_instance_uuid = $entityUUID.uuid
	UNION
	SELECT    COUNT(*) AS n
	FROM      storage_instance_filesystem
	WHERE     storage_instance_uuid = $entityUUID.uuid
)`, input, count{})
	if err != nil {
		return false, errors.Errorf(
			"preparing storage instance check no children query: %w", err,
		)
	}

	var result count
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, input).Get(&result)
		if err != nil {
			return errors.Errorf(
				"running storage instance check no children query: %w", err,
			)
		}
		return nil
	})
	if err != nil {
		return false, errors.Capture(err)
	}

	hasNoChildren := result.Count == 0
	return hasNoChildren, nil
}

// DeleteStorageInstance removes a storage instance from the database
// completely.
func (st *State) DeleteStorageInstance(
	ctx context.Context, siUUID string,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	input := entityUUID{UUID: siUUID}

	deleteUnitOwnerStmt, err := st.Prepare(`
DELETE FROM storage_unit_owner WHERE storage_instance_uuid = $entityUUID.uuid
`, input)
	if err != nil {
		return errors.Errorf(
			"preparing storage instance status deletion: %w", err,
		)
	}

	deleteStorageInstanceStmt, err := st.Prepare(`
DELETE FROM storage_instance WHERE uuid = $entityUUID.uuid
`, input)
	if err != nil {
		return errors.Errorf(
			"preparing storage instance deletion: %w", err,
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, deleteUnitOwnerStmt, input).Run()
		if err != nil {
			return errors.Errorf("deleting storage unit owner: %w", err)
		}
		err = tx.Query(ctx, deleteStorageInstanceStmt, input).Run()
		if err != nil {
			return errors.Errorf("deleting storage instance: %w", err)
		}
		return nil
	})

	return errors.Capture(err)
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

// GetVolumeStatus returns the status of the volume indicated by the input UUID.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeNotFound] when the volume does not exist.
func (st *State) GetVolumeStatus(
	ctx context.Context, volUUID string,
) (int, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return -1, errors.Capture(err)
	}

	inputUUID := entityUUID{UUID: volUUID}

	existsStmt, err := st.Prepare(`
SELECT &entityUUID.*
FROM   storage_volume
WHERE  uuid = $entityUUID.uuid
`, inputUUID)
	if err != nil {
		return -1, errors.Errorf(
			"preparing storage volume exists query: %w", err,
		)
	}

	stmt, err := st.Prepare(`
SELECT &status.*
FROM   storage_volume_status
WHERE  volume_uuid = $entityUUID.uuid
`, inputUUID, status{})
	if err != nil {
		return -1, errors.Errorf(
			"preparing storage volume status query: %w", err,
		)
	}

	var ret status
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, existsStmt, inputUUID).Get(&inputUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return storageprovisioningerrors.VolumeNotFound
		} else if err != nil {
			return errors.Errorf("storage volume exists query: %w", err)
		}
		err = tx.Query(ctx, stmt, inputUUID).Get(&ret)
		if err != nil {
			return errors.Errorf("storage volume status query: %w", err)
		}
		return nil
	})
	if err != nil {
		return -1, errors.Capture(err)
	}

	return ret.StatusID, nil
}

// SetVolumeStatus changes the status of the volume indicated by the input UUID
// and status value.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeNotFound] when the volume does not exist.
func (st *State) SetVolumeStatus(
	ctx context.Context, volUUID string, statusId int,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	inputUUID := entityUUID{UUID: volUUID}
	inputStatus := status{StatusID: statusId}

	existsStmt, err := st.Prepare(`
SELECT &entityUUID.*
FROM   storage_volume
WHERE  uuid = $entityUUID.uuid
`, inputUUID)
	if err != nil {
		return errors.Errorf(
			"preparing storage volume exists query: %w", err,
		)
	}

	stmt, err := st.Prepare(`
UPDATE storage_volume_status
SET    status_id=$status.status_id,
       message='',
       updated_at=DATETIME('now')
WHERE  volume_uuid = $entityUUID.uuid
`, inputUUID, inputStatus)
	if err != nil {
		return errors.Errorf("preparing storage volume status update: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, existsStmt, inputUUID).Get(&inputUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return storageprovisioningerrors.VolumeNotFound
		} else if err != nil {
			return errors.Errorf("storage volume exists query: %w", err)
		}
		err = tx.Query(ctx, stmt, inputUUID, inputStatus).Run()
		if err != nil {
			return errors.Errorf("storage volume status query: %w", err)
		}
		return nil
	})
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// DeleteVolume deletes the volume specified by the input UUID. It also deletes
// the storage instance volume relation if it still exists.
func (st *State) DeleteVolume(ctx context.Context, rUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	volUUID := entityUUID{UUID: rUUID}

	deleteMachineVolumeStmt, err := st.Prepare(`
DELETE FROM machine_volume WHERE volume_uuid = $entityUUID.uuid
`, volUUID)
	if err != nil {
		return errors.Errorf(
			"preparing in machine volume deletion: %w", err,
		)
	}

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
		return errors.Errorf("preparing volume deletion: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, deleteMachineVolumeStmt, volUUID).Run()
		if err != nil {
			return errors.Errorf("deleting machine volume: %w", err)
		}
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

// GetFilesystemStatus returns the status of the filesystem indicated by the
// input UUID.
//
// The following errors may be returned:
// - [storageprovisioningerrors.FilesystemNotFound] when the filesystem does not
// exist.
func (st *State) GetFilesystemStatus(
	ctx context.Context, fsUUID string,
) (int, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return -1, errors.Capture(err)
	}

	inputUUID := entityUUID{UUID: fsUUID}

	existsStmt, err := st.Prepare(`
SELECT &entityUUID.*
FROM   storage_filesystem
WHERE  uuid = $entityUUID.uuid
`, inputUUID)
	if err != nil {
		return -1, errors.Errorf(
			"preparing storage filesystem exists query: %w", err,
		)
	}

	stmt, err := st.Prepare(`
SELECT &status.*
FROM   storage_filesystem_status
WHERE  filesystem_uuid = $entityUUID.uuid
`, inputUUID, status{})
	if err != nil {
		return -1, errors.Errorf(
			"preparing storage filesystem status query: %w", err,
		)
	}

	var ret status
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, existsStmt, inputUUID).Get(&inputUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return storageprovisioningerrors.FilesystemNotFound
		} else if err != nil {
			return errors.Errorf("storage filesystem exists query: %w", err)
		}
		err = tx.Query(ctx, stmt, inputUUID).Get(&ret)
		if err != nil {
			return errors.Errorf("storage filesystem status query: %w", err)
		}
		return nil
	})
	if err != nil {
		return -1, errors.Capture(err)
	}

	return ret.StatusID, nil
}

// SetFilesystemStatus changes the status of the filesystem indicated by the
// input UUID and status value.
//
// The following errors may be returned:
// - [storageprovisioningerrors.FilesystemNotFound] when the filesystem does not
// exist.
func (st *State) SetFilesystemStatus(
	ctx context.Context, fsUUID string, statusId int,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	inputUUID := entityUUID{UUID: fsUUID}
	inputStatus := status{StatusID: statusId}

	existsStmt, err := st.Prepare(`
SELECT &entityUUID.*
FROM   storage_filesystem
WHERE  uuid = $entityUUID.uuid
`, inputUUID)
	if err != nil {
		return errors.Errorf(
			"preparing storage filesystem exists query: %w", err,
		)
	}

	stmt, err := st.Prepare(`
UPDATE storage_filesystem_status
SET    status_id=$status.status_id,
       message='',
       updated_at=DATETIME('now')
WHERE  filesystem_uuid = $entityUUID.uuid
`, inputUUID, inputStatus)
	if err != nil {
		return errors.Errorf(
			"preparing storage filesystem status update: %w", err,
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, existsStmt, inputUUID).Get(&inputUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return storageprovisioningerrors.FilesystemNotFound
		} else if err != nil {
			return errors.Errorf("storage filesystem exists query: %w", err)
		}
		err = tx.Query(ctx, stmt, inputUUID, inputStatus).Run()
		if err != nil {
			return errors.Errorf("storage filesystem status query: %w", err)
		}
		return nil
	})
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// CheckVolumeBackedFilesystemCrossProvisioned returns true if the specified
// uuid is a filesystem that is volume backed, where the filesystem is not
// owned by a machine, where the filesystem is machine provisioned and where
// the volume is model provisioned. This is to handle filesystems that will
// never be de-provisioned by a provisioner.
func (st *State) CheckVolumeBackedFilesystemCrossProvisioned(
	ctx context.Context, fsUUID string,
) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	inputUUID := entityUUID{UUID: fsUUID}

	existsStmt, err := st.Prepare(`
SELECT &entityUUID.*
FROM   storage_filesystem
WHERE  uuid = $entityUUID.uuid
`, inputUUID)
	if err != nil {
		return false, errors.Errorf(
			"preparing storage filesystem exists query: %w", err,
		)
	}

	crossProvisionedStmt, err := st.Prepare(`
SELECT    COUNT(sf.uuid) AS &count.count
FROM      storage_filesystem sf
JOIN      storage_instance_filesystem sif ON sf.uuid = sif.storage_filesystem_uuid
JOIN      storage_instance_volume siv ON sif.storage_instance_uuid = siv.storage_instance_uuid
JOIN      storage_volume sv ON siv.storage_volume_uuid = sv.uuid
LEFT JOIN machine_filesystem mf ON sf.uuid = mf.filesystem_uuid
WHERE     sf.uuid = $entityUUID.uuid AND
          sf.provision_scope_id != sv.provision_scope_id AND
          mf.machine_uuid IS NULL
`, inputUUID, count{})
	if err != nil {
		return false, errors.Errorf(
			"preparing storage filesystem check cross-provisioned query: %w",
			err,
		)
	}

	var res count
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, existsStmt, inputUUID).Get(&inputUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return storageprovisioningerrors.FilesystemNotFound
		} else if err != nil {
			return errors.Errorf("storage filesystem exists query: %w", err)
		}
		err = tx.Query(ctx, crossProvisionedStmt, inputUUID).Get(&res)
		if err != nil {
			return errors.Errorf(
				"storage filesystem check cross-provisioned query: %w", err,
			)
		}
		return nil
	})
	if err != nil {
		return false, errors.Capture(err)
	}

	return res.Count != 0, nil
}

// DeleteFilesystem deletes the filesystem specified by the input UUID. It also
// deletes the storage instance filesystem relation if it still exists.
func (st *State) DeleteFilesystem(ctx context.Context, rUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	fsUUID := entityUUID{UUID: rUUID}

	deleteMachineFilesystemStmt, err := st.Prepare(`
DELETE FROM machine_filesystem WHERE filesystem_uuid = $entityUUID.uuid
`, fsUUID)
	if err != nil {
		return errors.Errorf("preparing machine filesystem deletion: %w", err)
	}

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
		err := tx.Query(ctx, deleteMachineFilesystemStmt, fsUUID).Run()
		if err != nil {
			return errors.Errorf("deleting machine filesystem: %w", err)
		}
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
// attachment indicated by the supplied UUID. If the filesystem attachment is
// the last filesystem attachment of a dying filesystem, then mark that
// filesystem as dead.
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
			"preparing mark filesystem attachment exists dead: %w", err,
		)
	}

	remainingAttachmentsStmt, err := st.Prepare(`
SELECT COUNT(sfaB.uuid) AS &count.count
FROM   storage_filesystem_attachment sfaA
JOIN   storage_filesystem_attachment sfaB ON sfaB.storage_filesystem_uuid = sfaA.storage_filesystem_uuid
WHERE  sfaA.uuid = $entityUUID.uuid AND
       sfaB.uuid != $entityUUID.uuid
`, fsaUUID, count{})
	if err != nil {
		return errors.Errorf(
			"preparing remaining filesystem attachment exists query: %w", err,
		)
	}

	markDyingFilesystemDeadStmt, err := st.Prepare(`
WITH fs AS (
	SELECT storage_filesystem_uuid
	FROM   storage_filesystem_attachment
	WHERE  uuid = $entityUUID.uuid
)
UPDATE storage_filesystem
SET    life_id = 2
WHERE  life_id = 1 AND uuid IN fs
`, fsaUUID)
	if err != nil {
		return errors.Errorf(
			"preparing mark dying filesystem dead query: %w", err,
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

		var remaining count
		err = tx.Query(ctx, remainingAttachmentsStmt, fsaUUID).Get(&remaining)
		if err != nil {
			return errors.Errorf(
				"running remaining filesystem attachment query: %w", err,
			)
		}
		if remaining.Count > 0 {
			return nil
		}

		err = tx.Query(ctx, markDyingFilesystemDeadStmt, fsaUUID).Run()
		if err != nil {
			return errors.Errorf(
				"running mark dying filesystem dead query: %w", err,
			)
		}

		return nil
	})
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// DeleteFilesystemAttachment removes the filesystem attachment with the
// input UUID.
func (st *State) DeleteFilesystemAttachment(
	ctx context.Context, fsaUUID string,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	uuid := entityUUID{UUID: fsaUUID}

	deleteStmt, err := st.Prepare(`
DELETE FROM storage_filesystem_attachment WHERE uuid = $entityUUID.uuid
`, uuid)
	if err != nil {
		return errors.Errorf(
			"preparing filesystem attachment deletion: %w", err,
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, deleteStmt, uuid).Run()
		if err != nil {
			return errors.Errorf("deleting filesystem attachment: %w", err)
		}
		return nil
	})

	return errors.Capture(err)
}

// FilesystemAttachmentScheduleRemoval schedules a removal job for the
// filesystem attachment with the input UUID, qualified with the input force
// boolean.
func (st *State) FilesystemAttachmentScheduleRemoval(
	ctx context.Context,
	removalUUID, fsaUUID string,
	force bool, when time.Time,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	removalRec := removalJob{
		UUID:          removalUUID,
		RemovalTypeID: uint64(removal.StorageFilesystemAttachmentJob),
		EntityUUID:    fsaUUID,
		Force:         force,
		ScheduledFor:  when,
	}

	stmt, err := st.Prepare(`
INSERT INTO removal (*) VALUES ($removalJob.*)
`, removalRec)
	if err != nil {
		return errors.Errorf("preparing filesystem attachment removal: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, removalRec).Run()
		if err != nil {
			return errors.Errorf(
				"scheduling filesystem attachment removal: %w", err,
			)
		}
		return nil
	}))
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
// indicated by the supplied UUID. If the volume attachment is the last volume
// attachment of a dying volume, then mark that volume as dead.
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
			"preparing mark volume attachment dead query: %w", err,
		)
	}

	remainingAttachmentsStmt, err := st.Prepare(`
SELECT SUM(count) AS &count.count FROM (
    SELECT COUNT(svaB.uuid) AS count
    FROM   storage_volume_attachment svaA
    JOIN   storage_volume_attachment svaB ON svaB.storage_volume_uuid = svaA.storage_volume_uuid
    WHERE  svaA.uuid = $entityUUID.uuid AND
           svaB.uuid != $entityUUID.uuid
    UNION
    SELECT COUNT(svap.uuid) AS count
    FROM   storage_volume_attachment sva
    JOIN   storage_volume_attachment_plan svap ON sva.storage_volume_uuid = svap.storage_volume_uuid
    WHERE  sva.uuid = $entityUUID.uuid
)
`, vaUUID, count{})
	if err != nil {
		return errors.Errorf(
			"preparing remaining volume attachment/plan exists query: %w", err,
		)
	}

	markDyingVolumeDeadStmt, err := st.Prepare(`
WITH vol AS (
	SELECT storage_volume_uuid
	FROM   storage_volume_attachment
	WHERE  uuid = $entityUUID.uuid
)
UPDATE storage_volume
SET    life_id = 2
WHERE  life_id = 1 AND uuid IN vol
`, vaUUID)
	if err != nil {
		return errors.Errorf(
			"preparing mark dying volume dead query: %w", err,
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

		var remaining count
		err = tx.Query(ctx, remainingAttachmentsStmt, vaUUID).Get(&remaining)
		if err != nil {
			return errors.Errorf(
				"running remaining volume attachment query: %w", err,
			)
		}
		if remaining.Count > 0 {
			return nil
		}

		err = tx.Query(ctx, markDyingVolumeDeadStmt, vaUUID).Run()
		if err != nil {
			return errors.Errorf(
				"running mark dying volume dead query: %w", err,
			)
		}

		return nil
	})
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// DeleteVolumeAttachment removes the volume attachment with the input UUID.
func (st *State) DeleteVolumeAttachment(
	ctx context.Context, vaUUID string,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	uuid := entityUUID{UUID: vaUUID}

	deleteStmt, err := st.Prepare(`
DELETE FROM storage_volume_attachment WHERE uuid = $entityUUID.uuid
`, uuid)
	if err != nil {
		return errors.Errorf(
			"preparing volume attachment deletion: %w", err,
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, deleteStmt, uuid).Run()
		if err != nil {
			return errors.Errorf("deleting volume attachment: %w", err)
		}
		return nil
	})

	return errors.Capture(err)
}

// VolumeAttachmentScheduleRemoval schedules a removal job for the volume
// attachment with the input UUID, qualified with the input force boolean.
func (st *State) VolumeAttachmentScheduleRemoval(
	ctx context.Context,
	removalUUID, vaUUID string,
	force bool, when time.Time,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	removalRec := removalJob{
		UUID:          removalUUID,
		RemovalTypeID: uint64(removal.StorageVolumeAttachmentJob),
		EntityUUID:    vaUUID,
		Force:         force,
		ScheduledFor:  when,
	}

	stmt, err := st.Prepare(`
INSERT INTO removal (*) VALUES ($removalJob.*)
`, removalRec)
	if err != nil {
		return errors.Errorf("preparing volume attachment removal: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, removalRec).Run()
		if err != nil {
			return errors.Errorf(
				"scheduling volume attachment removal: %w", err,
			)
		}
		return nil
	}))
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

	remainingAttachmentsStmt, err := st.Prepare(`
SELECT SUM(count) AS &count.count FROM (
    SELECT COUNT(svapB.uuid) AS count
    FROM   storage_volume_attachment_plan svapA
    JOIN   storage_volume_attachment_plan svapB ON svapB.storage_volume_uuid = svapA.storage_volume_uuid
    WHERE  svapA.uuid = $entityUUID.uuid AND
           svapB.uuid != $entityUUID.uuid
    UNION
    SELECT COUNT(sva.uuid) AS count
    FROM   storage_volume_attachment_plan svap
    JOIN   storage_volume_attachment sva ON svap.storage_volume_uuid = sva.storage_volume_uuid
    WHERE  svap.uuid = $entityUUID.uuid
)
`, vapUUID, count{})
	if err != nil {
		return errors.Errorf(
			"preparing remaining volume attachment/plan exists query: %w", err,
		)
	}

	markDyingVolumeDeadStmt, err := st.Prepare(`
WITH vol AS (
	SELECT storage_volume_uuid
	FROM   storage_volume_attachment_plan
	WHERE  uuid = $entityUUID.uuid
)
UPDATE storage_volume
SET    life_id = 2
WHERE  life_id = 1 AND uuid IN vol
`, vapUUID)
	if err != nil {
		return errors.Errorf(
			"preparing mark dying volume dead query: %w", err,
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

		var remaining count
		err = tx.Query(ctx, remainingAttachmentsStmt, vapUUID).Get(&remaining)
		if err != nil {
			return errors.Errorf(
				"running remaining volume attachment/plan query: %w", err,
			)
		}
		if remaining.Count > 0 {
			return nil
		}

		err = tx.Query(ctx, markDyingVolumeDeadStmt, vapUUID).Run()
		if err != nil {
			return errors.Errorf(
				"running mark dying volume dead query: %w", err,
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

// DeleteVolumeAttachmentPlan removes the volume attachment plan with the input
// UUID.
func (st *State) DeleteVolumeAttachmentPlan(
	ctx context.Context, vapUUID string,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	uuid := entityUUID{UUID: vapUUID}

	deleteStmt, err := st.Prepare(`
DELETE FROM storage_volume_attachment_plan WHERE uuid = $entityUUID.uuid
`, uuid)
	if err != nil {
		return errors.Errorf(
			"preparing volume attachment plan deletion: %w", err,
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, deleteStmt, uuid).Run()
		if err != nil {
			return errors.Errorf("deleting volume attachmentplan : %w", err)
		}
		return nil
	})

	return errors.Capture(err)
}

// VolumeAttachmentPlanScheduleRemoval schedules a removal job for the volume
// attachment plan with the input UUID, qualified with the input force boolean.
func (st *State) VolumeAttachmentPlanScheduleRemoval(
	ctx context.Context,
	removalUUID, vapUUID string,
	force bool, when time.Time,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	removalRec := removalJob{
		UUID:          removalUUID,
		RemovalTypeID: uint64(removal.StorageVolumeAttachmentPlanJob),
		EntityUUID:    vapUUID,
		Force:         force,
		ScheduledFor:  when,
	}

	stmt, err := st.Prepare(`
INSERT INTO removal (*) VALUES ($removalJob.*)
`, removalRec)
	if err != nil {
		return errors.Errorf(
			"preparing volume attachment plan removal: %w", err,
		)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, removalRec).Run()
		if err != nil {
			return errors.Errorf(
				"scheduling volume attachment plan removal: %w", err,
			)
		}
		return nil
	}))
}

// ensureStorageInstancesNotAliveCascade ensures that the storage instances
// identified by the input UUIDs are no longer alive.
// Filesystems and Volumes that are associated to the storage instances are also
// ensured to be no longer alive. If a Filesystem or Volume id not attached,
// then it goes straight to dead.
func (st *State) ensureStorageInstancesNotAliveCascade(
	ctx context.Context, tx *sqlair.TX, siUUIDs entityUUIDs, obliterate bool,
) (internal.CascadedStorageInstanceLives, error) {
	var cascaded internal.CascadedStorageInstanceLives

	removal := storageRemoval{
		Obliterate: obliterate,
	}
	input := siUUIDs.uuids()

	// First kill the storage instances.
	stmt, err := st.Prepare(`
UPDATE storage_instance
SET    life_id = 1
WHERE  uuid IN ($uuids[:])
`, input)
	if err != nil {
		return cascaded, errors.Errorf(
			"preparing storage instance life update: %w", err,
		)
	}
	if err := tx.Query(ctx, stmt, input).Run(); err != nil {
		return cascaded, errors.Errorf(
			"running storage instance life update: %w", err,
		)
	}
	cascaded.StorageInstanceUUIDs = input

	// Mark any unattached filesystems as Dead.
	qry := `
SELECT    sf.uuid AS &entityUUID.uuid
FROM      storage_instance_filesystem sif
JOIN      storage_filesystem sf ON sif.storage_filesystem_uuid = sf.uuid
LEFT JOIN storage_filesystem_attachment sfa ON sfa.storage_filesystem_uuid = sf.uuid
WHERE     sif.storage_instance_uuid IN ($uuids[:])
AND       sf.life_id != 2 AND
          sfa.uuid IS NULL`

	del := `
UPDATE storage_filesystem
SET    life_id = 2,
       obliterate_on_cleanup = $storageRemoval.obliterate
WHERE  uuid IN ($uuids[:])`

	deadFilesystemUUIDs, err := st.ensureStorageEntitiesNotAlive(
		ctx, tx, siUUIDs, "filesystem", qry, del, removal,
	)
	if err != nil {
		return cascaded, errors.Capture(err)
	}
	cascaded.FileSystemUUIDs = append(cascaded.FileSystemUUIDs,
		deadFilesystemUUIDs...)

	// Mark any Alive filesystems Dying.
	qry = `
SELECT f.uuid AS &entityUUID.uuid
FROM   storage_instance_filesystem i
JOIN   storage_filesystem f ON i.storage_filesystem_uuid = f.uuid
WHERE  i.storage_instance_uuid IN ($uuids[:])
AND    f.life_id = 0`

	del = `
UPDATE storage_filesystem
SET    life_id = 1,
       obliterate_on_cleanup = $storageRemoval.obliterate
WHERE  uuid IN ($uuids[:])`

	dyingFilesystemUUIDs, err := st.ensureStorageEntitiesNotAlive(
		ctx, tx, siUUIDs, "filesystem", qry, del, removal,
	)
	if err != nil {
		return cascaded, errors.Capture(err)
	}
	cascaded.FileSystemUUIDs = append(cascaded.FileSystemUUIDs,
		dyingFilesystemUUIDs...)

	// Mark any unattached volumes as Dead.
	qry = `
SELECT    sv.uuid AS &entityUUID.uuid
FROM      storage_instance_volume siv
JOIN      storage_volume sv ON siv.storage_volume_uuid = sv.uuid
LEFT JOIN storage_volume_attachment sva ON sva.storage_volume_uuid = sv.uuid
WHERE     siv.storage_instance_uuid IN ($uuids[:])
AND       sv.life_id != 2 AND
          sva.uuid IS NULL`

	del = `
UPDATE storage_volume
SET    life_id = 2,
       obliterate_on_cleanup = $storageRemoval.obliterate
WHERE  uuid IN ($uuids[:])`

	deadVolumeUUIDs, err := st.ensureStorageEntitiesNotAlive(
		ctx, tx, siUUIDs, "volume", qry, del, removal,
	)
	if err != nil {
		return cascaded, errors.Capture(err)
	}
	cascaded.VolumeUUIDs = append(cascaded.VolumeUUIDs, deadVolumeUUIDs...)

	// Mark any Alive volumes as Dying.
	qry = `
SELECT &entityUUID.uuid
FROM   storage_instance_volume i
JOIN   storage_volume v ON i.storage_volume_uuid = v.uuid
WHERE  i.storage_instance_uuid IN ($uuids[:])
AND    v.life_id = 0`

	del = `
UPDATE storage_volume
SET    life_id = 1,
       obliterate_on_cleanup = $storageRemoval.obliterate
WHERE  uuid IN ($uuids[:])`

	dyingVolumeUUIDs, err := st.ensureStorageEntitiesNotAlive(
		ctx, tx, siUUIDs, "volume", qry, del, removal,
	)
	if err != nil {
		return cascaded, errors.Capture(err)
	}
	cascaded.VolumeUUIDs = append(cascaded.VolumeUUIDs, dyingVolumeUUIDs...)

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
	`, input)
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
	ctx context.Context, tx *sqlair.TX, siUUID entityUUIDs, entityType, qry, del string, removal storageRemoval,
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
	if stmt, err = st.Prepare(del, input, removal); err != nil {
		return nil, errors.Errorf("preparing %s life update: %w", entityType, err)
	}
	if err := tx.Query(ctx, stmt, input, removal).Run(); err != nil {
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
