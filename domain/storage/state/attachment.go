// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	coreunit "github.com/juju/juju/core/unit"
	domainapplicationerrors "github.com/juju/juju/domain/application/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
)

// checkStorageInstanceExists checks if a storage instance with the given UUID
// exists in the model.
func (s *State) checkStorageInstanceExists(
	ctx context.Context, tx *sqlair.TX, uuid domainstorage.StorageInstanceUUID,
) (bool, error) {

	entityUUIDInput := entityUUID{UUID: uuid.String()}

	stmt, err := s.Prepare(
		"SELECT &entityUUID.* FROM storage_instance WHERE uuid = $entityUUID.uuid",
		entityUUIDInput,
	)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, entityUUIDInput).Get(&entityUUIDInput)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}

	return true, nil
}

// GetStorageAttachmentUUIDForStorageIDAndUnit returns the
// [domainstorageprovisioning.StorageAttachmentUUID] associated with the given
// storage instance uuid and unit uuid.
//
// The following errors may be returned:
// - [domainstorageerrors.StorageInstanceNotFound]
// if the storage instance for the supplied uuid no longer exists.
// - [domainapplicationerrors.UnitNotFound] if the unit no longer exists for the
// supplied uuid.
func (s *State) GetStorageAttachmentUUIDForStorageInstanceAndUnit(
	ctx context.Context,
	sUUID domainstorage.StorageInstanceUUID,
	uUUID coreunit.UUID,
) (domainstorage.StorageAttachmentUUID, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}
	var (
		storageInstanceUUIDInput = storageInstanceUUID{UUID: sUUID.String()}
		unitUUIDInput            = unitUUID{UUID: uUUID.String()}
		dbVal                    entityUUID
	)
	stmt, err := s.Prepare(`
SELECT &entityUUID.*
FROM   storage_attachment
WHERE  storage_instance_uuid = $storageInstanceUUID.uuid
AND    unit_uuid = $unitUUID.uuid`,
		storageInstanceUUIDInput, unitUUIDInput, dbVal,
	)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := s.checkStorageInstanceExists(ctx, tx, sUUID)
		if err != nil {
			return errors.Errorf(
				"checking if storage instance %q exists: %w", sUUID, err,
			)
		}
		if !exists {
			return errors.Errorf(
				"storage instance %q does not exist", sUUID,
			).Add(domainstorageerrors.StorageInstanceNotFound)
		}

		exists, err = s.checkUnitExists(ctx, tx, uUUID)
		if err != nil {
			return errors.Errorf("checking if unit %q exists: %w", uUUID, err)
		}
		if !exists {
			return errors.Errorf(
				"unit %q does not exist", uUUID,
			).Add(domainapplicationerrors.UnitNotFound)
		}

		err = tx.Query(ctx, stmt, storageInstanceUUIDInput, unitUUIDInput).Get(&dbVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.New(
				"storage attachment does not exist in the model",
			).Add(domainstorageerrors.StorageAttachmentNotFound)
		}

		return err
	})
	if err != nil {
		return "", errors.Capture(err)
	}

	return domainstorage.StorageAttachmentUUID(dbVal.UUID), nil
}

// GetStorageInstanceAttachments returns the set of attachments a storage
// instance has. If the storage instance has no attachments then an empty
// slice.
//
// The following errors may be returned:
// - [domainstorageerrors.StorageInstanceNotFound] if the storage instance for
// the supplied uuid does not exist.
func (s *State) GetStorageInstanceAttachments(
	ctx context.Context,
	uuid domainstorage.StorageInstanceUUID,
) ([]domainstorage.StorageAttachmentUUID, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	uuidInput := storageInstanceUUID{UUID: uuid.String()}

	attachQ := `
SELECT &entityUUID.*
FROM storage_attachment
WHERE storage_instance_uuid = $storageInstanceUUID.uuid
`

	stmt, err := s.Prepare(attachQ, uuidInput, entityUUID{})
	if err != nil {
		return nil, errors.Errorf(
			"preparing storage instance attachments statement: %w", err,
		)
	}

	dbVals := []entityUUID{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := s.checkStorageInstanceExists(ctx, tx, uuid)
		if err != nil {
			return errors.Errorf(
				"checking storage instance %q exists: %w", uuid, err,
			)
		}
		if !exists {
			return errors.Errorf(
				"storage instance %q does not exist in the model", uuid,
			).Add(domainstorageerrors.StorageInstanceNotFound)
		}

		err = tx.Query(ctx, stmt, uuidInput).GetAll(&dbVals)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	rval := make([]domainstorage.StorageAttachmentUUID, 0, len(dbVals))
	for _, dbVal := range dbVals {
		rval = append(rval, domainstorage.StorageAttachmentUUID(dbVal.UUID))
	}
	return rval, nil
}

// GetStorageInstanceUUIDsForUnit returns the storage instance UUIDs attached
// to the unit with the input UUID. If the unit has no storage attachments an
// empty slice is returned.
//
// The following errors may be returned:
// - [domainapplicationerrors.UnitNotFound] if the unit does not exist.
func (s *State) GetStorageInstanceUUIDsForUnit(
	ctx context.Context, unitUUIDStr string,
) ([]domainstorage.StorageInstanceUUID, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	unitUUIDInput := entityUUID{UUID: unitUUIDStr}

	stmt, err := s.Prepare(`
SELECT si.uuid AS &entityUUID.uuid
FROM   storage_attachment sa
JOIN   storage_instance si ON sa.storage_instance_uuid = si.uuid
WHERE  sa.unit_uuid = $entityUUID.uuid`, entityUUID{})
	if err != nil {
		return nil, errors.Errorf(
			"preparing storage instance UUIDs for unit query: %w", err,
		)
	}

	var dbVals []entityUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := s.checkUnitExists(ctx, tx, coreunit.UUID(unitUUIDStr))
		if err != nil {
			return errors.Capture(err)
		}
		if !exists {
			return errors.Errorf(
				"unit %q does not exist", unitUUIDStr,
			).Add(domainapplicationerrors.UnitNotFound)
		}

		err = tx.Query(ctx, stmt, unitUUIDInput).GetAll(&dbVals)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"getting storage instance UUIDs for unit %q: %w", unitUUIDStr, err,
			)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	rval := make([]domainstorage.StorageInstanceUUID, 0, len(dbVals))
	for _, dbVal := range dbVals {
		rval = append(rval, domainstorage.StorageInstanceUUID(dbVal.UUID))
	}
	return rval, nil
}
