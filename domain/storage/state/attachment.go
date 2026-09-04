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
	"github.com/juju/juju/domain/storage/internal"
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

// GetStorageClassificationForUnits returns the storage instances attached to
// the input units,keyed by unit UUID,along with the minimal information
// needed to classify each instance as destroyed or detached when its unit
// is removed. Units with no attached storage are absent from the returned map.
//
// This method deliberately does not verify that the input units exist. The
// caller is expected to have resolved the units first,so units that vanish
// mid-flight simply contribute no entries.

func (s *State) GetStorageClassificationForUnits(
	ctx context.Context, unitUUIDs []string,
) (map[string][]internal.StorageInstanceClassification, error) {
	if len(unitUUIDs) == 0 {
		return map[string][]internal.StorageInstanceClassification{}, nil
	}

	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := s.Prepare(`
SELECT sa.unit_uuid              AS &storageClassification.unit_uuid,
        si.uuid                  AS &storageClassification.storage_uuid,
        si.storage_id            AS &storageClassification.storage_id,
        sv.persistent            AS &storageClassification.persistent
FROM    storage_attachment AS sa
JOIN    storage_instance AS si ON si.uuid = sa.storage_instance_uuid
LEFT JOIN storage_instance_volume AS siv ON siv.storage_instance_uuid = si.uuid
LEFT JOIN storage_volume AS sv ON sv.uuid = siv.storage_volume_uuid
WHERE   sa.unit_uuid IN ($uuids[:])
ORDER BY sa.unit_uuid, sa.storage_instance_uuid
`, uuids{}, storageClassification{})
	if err != nil {
		return nil, errors.Errorf(
			"preparing storage classification query: %w", err,
		)
	}

	var dbVals []storageClassification
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		dbVals = nil // reset the accumulator at the top of the closure.
		err = tx.Query(ctx, stmt, uuids(unitUUIDs)).GetAll(&dbVals)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting storage classification: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	ret := make(map[string][]internal.StorageInstanceClassification, len(dbVals))
	for _, v := range dbVals {
		instance := internal.StorageInstanceClassification{
			Persistent:  v.Persistent.V,
			StorageID:   v.StorageID,
			StorageUUID: v.StorageUUID,
			UnitUUID:    v.UnitUUID,
		}
		ret[v.UnitUUID] = append(ret[v.UnitUUID], instance)
	}
	return ret, nil
}
