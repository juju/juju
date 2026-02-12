// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"slices"

	"github.com/canonical/sqlair"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/domain/storage/internal"
	"github.com/juju/juju/internal/errors"
)

// ImportStorageInstances imports storage instances and storage unit
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

// ImportFilesystems imports filesystems from the provided parameters.
func (st *State) ImportFilesystems(ctx context.Context, args []internal.ImportFilesystemArgs) error {
	if len(args) == 0 {
		return nil
	}

	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	insertStorageFilesystemStmt, err := st.Prepare(`
	INSERT INTO storage_filesystem (*) VALUES ($importStorageFilesystem.*)`, importStorageFilesystem{})
	if err != nil {
		return errors.Capture(err)
	}

	insertStorageInstanceFilesystemStmt, err := st.Prepare(`
	INSERT INTO storage_instance_filesystem (*) VALUES ($importStorageInstanceFilesystem.*)`, importStorageInstanceFilesystem{})
	if err != nil {
		return errors.Capture(err)
	}

	fsArgs := make([]importStorageFilesystem, len(args))
	fsInstanceArgs := make([]importStorageInstanceFilesystem, 0, len(args))
	for i, arg := range args {
		fsArgs[i] = importStorageFilesystem{
			UUID:       arg.UUID,
			ID:         arg.ID,
			LifeID:     int(arg.Life),
			ScopeID:    int(arg.Scope),
			ProviderID: arg.ProviderID,
			SizeInMiB:  arg.SizeInMiB,
		}
		if arg.StorageInstanceUUID != "" {
			fsInstanceArgs = append(fsInstanceArgs, importStorageInstanceFilesystem{
				StorageInstanceUUID: arg.StorageInstanceUUID,
				FilesystemUUID:      arg.UUID,
			})
		}
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, insertStorageFilesystemStmt, fsArgs).Run()
		if err != nil {
			return errors.Errorf("inserting storage filesystem rows: %w", err)
		}

		if len(fsInstanceArgs) > 0 {
			err := tx.Query(ctx, insertStorageInstanceFilesystemStmt, fsInstanceArgs).Run()
			if err != nil {
				return errors.Errorf("inserting storage instance filesystem rows: %w", err)
			}
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

// GetNetNodeUUIDsByMachineOrUnitID returns net node UUIDs for all machine or
// and unit names provided. If a machine name or unit name is not found then it
// is excluded from the result.
func (st *State) GetNetNodeUUIDsByMachineOrUnitName(ctx context.Context, machines []string, units []string) (map[string]string, map[string]string, error) {
	if len(machines)+len(units) == 0 {
		return nil, nil, nil
	}
	db, err := st.DB(ctx)
	if err != nil {
		return nil, nil, errors.Capture(err)
	}
	slices.Sort(machines)
	machines = slices.Compact(machines)
	slices.Sort(units)
	units = slices.Compact(units)

	type machineNames []string
	type unitNames []string
	var (
		machineNameInput = machineNames(machines)
		unitNameInput    = unitNames(units)
	)
	stmt, err := st.Prepare(`
SELECT &machineAndUnitNetNodeUUID.*
FROM (
    SELECT name AS machine_name,
           net_node_uuid AS machine_net_node_uuid,
           NULL AS unit_name,
           NULL AS unit_net_node_uuid
    FROM   machine
    WHERE  name IN ($machineNames[:])
    UNION
    SELECT NULL AS machine_name,
           NULL AS machine_net_node_uuid,
           name AS unit_name,
           net_node_uuid AS unit_net_node_uuid
    FROM   unit
    WHERE  name IN ($unitNames[:])
) 
`, machineNameInput, unitNameInput, machineAndUnitNetNodeUUID{})
	if err != nil {
		return nil, nil, errors.Capture(err)
	}
	var netNodeUUIDs []machineAndUnitNetNodeUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, machineNameInput, unitNameInput).GetAll(&netNodeUUIDs)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, nil, errors.Capture(err)
	}
	machineMap := make(map[string]string, len(machineNameInput))
	unitMap := make(map[string]string, len(unitNameInput))
	for _, dbVal := range netNodeUUIDs {
		if dbVal.MachineName.Valid {
			machineMap[dbVal.MachineName.String] = dbVal.MachineNetNodeUUID.String
		}
		if dbVal.UnitName.Valid {
			unitMap[dbVal.UnitName.String] = dbVal.UnitNetNodeUUID.String
		}
	}
	return machineMap, unitMap, nil
}
