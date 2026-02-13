// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"strings"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	coreerrors "github.com/juju/juju/core/errors"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
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

// ImportVolumes associates a volume (either native or volume backed) hosted by
// a cloud provider with a new storage instance (and storage pool) in a model.
func (st *State) ImportVolumes(ctx context.Context, args []internal.ImportVolumeArgs) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		storageVolumeData, storageInstanceVolumeData, err := parseVolumeImportData(args)
		if err != nil {
			return errors.Capture(err)
		}
		if err := st.importStorageVolumes(ctx, tx, storageVolumeData); err != nil {
			return errors.Capture(err)
		}
		if err := st.importStorageInstanceVolumes(ctx, tx, storageInstanceVolumeData); err != nil {
			return errors.Capture(err)
		}
		return nil
	})
}

// storage_instance_volume

func (st *State) importStorageVolumes(ctx context.Context, tx *sqlair.TX, input []importStorageVolume) error {
	insertStmt, err := st.Prepare(`
INSERT INTO storage_volume (*) VALUES ($importStorageVolume.*)
`, importStorageVolume{})
	if err != nil {
		return errors.Errorf("preparing insert volume import statement: %w", err)
	}

	err = tx.Query(ctx, insertStmt, input).Run()
	return err
}

func (st *State) importStorageInstanceVolumes(ctx context.Context, tx *sqlair.TX, input []importStorageInstanceVolume) error {
	insertStmt, err := st.Prepare(`
INSERT INTO storage_instance_volume (*) VALUES ($importStorageInstanceVolume.*)
`, importStorageInstanceVolume{})
	if err != nil {
		return errors.Errorf("preparing insert storage instance volume import statement: %w", err)
	}

	err = tx.Query(ctx, insertStmt, input).Run()
	return err
}

func parseVolumeImportData(args []internal.ImportVolumeArgs) ([]importStorageVolume, []importStorageInstanceVolume, error) {
	out := make([]importStorageVolume, len(args))
	outInstance := make([]importStorageInstanceVolume, len(args))

	for i, arg := range args {
		out[i] = importStorageVolume{
			UUID:             arg.UUID,
			VolumeID:         arg.ID,
			LifeID:           int(arg.LifeID),
			ProvisionScopeID: int(arg.ProvisionScopeID),
			ProviderID:       arg.ProviderID,
			SizeMiB:          arg.SizeMiB,
			HardwareID:       arg.HardwareID,
			WWN:              arg.WWN,
			Persistent:       arg.Persistent,
		}
		outInstance[i] = importStorageInstanceVolume{
			StorageInstanceUUID: arg.StorageInstanceUUID,
			VolumeUUID:          arg.UUID,
		}
	}

	return out, outInstance, nil
}

// GetNetNodeUUIDsByMachineOrUnitID returns net node UUIDs for all machine or
// and unit names provided.
//
// The following errors may be returned:
// - [applicationerrors.UnitNotFound] if not all net node UUIDs are found for the given units.
// - [machineerrors.MachineNotFound] if not all net node UUIDs are found for the given machines.
func (st *State) GetNetNodeUUIDsByMachineOrUnitName(ctx context.Context, machines []string, units []string) (map[string]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Remove duplicates and empty strings from the search
	machineNames := set.NewStrings(machines...)
	machineNames.Remove("")
	unitNames := set.NewStrings(units...)
	unitNames.Remove("")

	if machineNames.Size() == 0 && unitNames.Size() == 0 {
		return map[string]string{}, nil
	}

	type names []string
	machineNetNodeStmt, err := st.Prepare(`
SELECT (name, net_node_uuid) AS (&nameAndNetNodeUUID.*)
FROM   machine
WHERE  name IN ($names[:])
`, names{}, nameAndNetNodeUUID{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	unitNetNodeStmt, err := st.Prepare(`
SELECT (name, net_node_uuid) AS (&nameAndNetNodeUUID.*)
FROM   unit
WHERE  name IN ($names[:])
`, names{}, nameAndNetNodeUUID{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var machineNetNodes, unitNetNodes []nameAndNetNodeUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if len(machineNames) > 0 {
			tx.Query(ctx, machineNetNodeStmt, names(machineNames.Values())).GetAll(&machineNetNodes)
			if errors.Is(err, sql.ErrNoRows) {
				return errors.Errorf("machine(s) %q not found", strings.Join(machineNames.Values(), ", ")).Add(machineerrors.MachineNotFound)
			} else if err != nil {
				return errors.Errorf("getting machine net node UUIDs: %w", err)
			}
		}
		if len(unitNames) == 0 {
			return nil
		}
		tx.Query(ctx, unitNetNodeStmt, names(unitNames.Values())).GetAll(&unitNetNodes)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf("units(s) %q not found", strings.Join(machineNames.Values(), ", ")).Add(applicationerrors.UnitNotFound)
		} else if err != nil {
			return errors.Errorf("getting unit net node UUIDs: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	if machineNames.Size() != len(machineNetNodes) {
		missing := missingNames(machineNames, machineNetNodes)
		return nil, errors.Errorf("machines(s) with names(s) %s not found", strings.Join(missing, ", ")).
			Add(machineerrors.MachineNotFound)
	}
	if unitNames.Size() != len(unitNetNodes) {
		missing := missingNames(unitNames, unitNetNodes)
		return nil, errors.Errorf("units(s) with names(s) %s not found", strings.Join(missing, ", ")).
			Add(applicationerrors.UnitNotFound)
	}
	return transform.SliceToMap(append(machineNetNodes, unitNetNodes...), func(in nameAndNetNodeUUID) (string, string) {
		return in.Name, in.NetNodeUUID
	}), nil
}

func missingNames(names set.Strings, data []nameAndNetNodeUUID) []string {
	return names.
		Difference(set.NewStrings(transform.Slice(data, func(in nameAndNetNodeUUID) string { return in.Name })...)).
		SortedValues()
}
