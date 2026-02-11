// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/domain/storage/internal"
	"github.com/juju/juju/internal/errors"
)

// ImportStorageInstances creates new storage instances and storage unit
// owners. Storage unit owners are created if the unit name is provided.
func (st *State) ImportStorageInstances(ctx context.Context, args []internal.ImportStorageInstanceArgs) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	insertStorageInstanceStmt, err := st.Prepare(`
INSERT INTO storage_instance (*) VALUES ($importStorageInstance.*)`, importStorageInstance{})
	if err != nil {
		return errors.Capture(err)
	}

	insertUnitOwnerStmt, err := st.Prepare(`
INSERT INTO storage_unit_owner (*) VALUES ($importStorageUnitOwner.*)`, importStorageUnitOwner{})
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		storageInstances, storageUnitOwners, err := st.transformStorageInstances(ctx, tx, args)
		if err != nil {
			return err
		}

		err = tx.Query(ctx, insertStorageInstanceStmt, storageInstances).Run()
		if err != nil {
			return errors.Errorf("inserting storage instance rows: %w", err)
		}

		err = tx.Query(ctx, insertUnitOwnerStmt, storageUnitOwners).Run()
		if err != nil {
			return errors.Errorf("inserting storage unit owner rows: %w", err)
		}

		return nil
	})
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (st *State) transformStorageInstances(
	ctx context.Context,
	tx *sqlair.TX,
	args []internal.ImportStorageInstanceArgs,
) ([]importStorageInstance, []importStorageUnitOwner, error) {
	lookups, err := st.getImportStorageInstanceLookups(ctx, tx)
	if err != nil {
		return nil, nil, errors.Capture(err)
	}
	storageInstances := make([]importStorageInstance, len(args))
	storageUnitOwners := make([]importStorageUnitOwner, 0)

	for i, arg := range args {
		poolUUID, ok := lookups.StoragePoolUUID[arg.PoolName]
		if !ok {
			return nil, nil, errors.Errorf("pool %q not found for storage instance %q", arg.PoolName, arg.StorageName)
		}
		kind, ok := lookups.Kind[arg.StorageKind]
		if !ok {
			return nil, nil, errors.Errorf("storage kind ID not found for storage instance %q", arg.StorageName)
		}
		storageInstances[i] = importStorageInstance{
			UUID:            arg.UUID,
			LifeID:          arg.Life,
			StorageID:       arg.StorageID,
			StorageKindID:   kind,
			StorageName:     arg.StorageName,
			StoragePoolUUID: poolUUID,
			RequestedSize:   arg.RequestedSizeMiB,
		}

		charmName, unit, err := st.getCharmNameAndUnitUUIDFromUnitName(ctx, tx, arg.UnitName)
		if errors.Is(err, coreerrors.NotFound) {
			// Neither charmName in storage_instance storage_unit_owner rows
			// are required by the DDL.
			continue
		} else if err != nil {
			return nil, nil, errors.Errorf("getting charm and unit uuid from %q: w", err)
		}

		storageInstances[i].CharmName = charmName
		storageUnitOwners = append(storageUnitOwners, importStorageUnitOwner{
			StorageInstanceUUID: arg.UUID,
			UnitUUID:            unit,
		})
	}

	return storageInstances, storageUnitOwners, nil
}

func (st *State) getCharmNameAndUnitUUIDFromUnitName(
	ctx context.Context,
	tx *sqlair.TX,
	unitName string,
) (string, string, error) {
	if unitName == "" {
		return "", "", coreerrors.NotFound
	}
	stmt, err := st.Prepare(`
SELECT (cm.name, u.uuid) AS (&nameAndUUID.*)
FROM   unit AS u
JOIN   charm_metadata AS cm ON u.charm_uuid = cm.charm_uuid
WHERE  u.name = $name.name`, name{}, nameAndUUID{})
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", coreerrors.NotFound
	} else if err != nil {
		return "", "", errors.Capture(err)
	}
	var output nameAndUUID
	err = tx.Query(ctx, stmt, name{Name: unitName}).Get(&output)
	if err != nil {
		return "", "", errors.Errorf("finding charm name and unit uuid for %q: %w", unitName, err)
	}
	return output.Name, output.UUID, nil
}
