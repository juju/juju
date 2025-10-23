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
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
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
) (domainstorageprovisioning.StorageAttachmentUUID, error) {
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

	return domainstorageprovisioning.StorageAttachmentUUID(dbVal.UUID), nil
}
