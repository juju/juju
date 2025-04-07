// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/ipaddress"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/linklayerdevice"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/status"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// DeleteUnit deletes the specified unit.
// If the unit's application is Dying and no
// other references to it exist, true is returned to
// indicate the application could be safely deleted.
// It will fail if the unit is not Dead.
func (st *State) DeleteUnit(ctx context.Context, unitName coreunit.Name) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Capture(err)
	}

	unit := minimalUnit{Name: unitName}
	peerCountQuery := `
SELECT a.life_id as &unitCount.app_life_id, u.life_id AS &unitCount.unit_life_id, count(peer.uuid) AS &unitCount.count
FROM unit u
JOIN application a ON a.uuid = u.application_uuid
LEFT JOIN unit peer ON u.application_uuid = peer.application_uuid AND peer.uuid != u.uuid
WHERE u.name = $minimalUnit.name
`
	peerCountStmt, err := st.Prepare(peerCountQuery, unit, unitCount{})
	if err != nil {
		return false, errors.Capture(err)
	}
	canRemoveApplication := false
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.setUnitLife(ctx, tx, unitName, life.Dead)
		if err != nil {
			return errors.Errorf("setting unit %q to Dead: %w", unitName, err)
		}
		// Count the number of units besides this one
		// belonging to the same application.
		var count unitCount
		err = tx.Query(ctx, peerCountStmt, unit).Get(&count)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("unit %q not found", unitName).Add(applicationerrors.UnitNotFound)
		}
		if err != nil {
			return errors.Errorf("querying peer count for unit %q: %w", unitName, err)
		}
		// This should never happen since this method is called by the service
		// after setting the unit to Dead. But we check anyway.
		// There's no need for a typed error.
		if count.UnitLifeID != life.Dead {
			return errors.Errorf("unit %q is not dead, life is %v", unitName, count.UnitLifeID)
		}

		err = st.deleteUnit(ctx, tx, unitName)
		if err != nil {
			return errors.Errorf("deleting dead unit: %w", err)
		}
		canRemoveApplication = count.Count == 0 && count.ApplicationLifeID != life.Alive
		return nil
	})
	if err != nil {
		return false, errors.Errorf("removing unit %q: %w", unitName, err)
	}
	return canRemoveApplication, nil
}

func (st *State) deleteUnit(ctx context.Context, tx *sqlair.TX, unitName coreunit.Name) error {
	unit := minimalUnit{Name: unitName}

	queryUnit := `SELECT &minimalUnit.* FROM unit WHERE name = $minimalUnit.name`
	queryUnitStmt, err := st.Prepare(queryUnit, unit)
	if err != nil {
		return errors.Capture(err)
	}

	// NOTE: This is a work around because teardown is not implemented yet. Ideally,
	// our workflow will mean that by the time the unit is dead and we are ready to
	// delete it, a worker will have already cleaned up all dependencies. However,
	// this is not the case yet. Remove the secret owner for the unit, leaving the
	// secret orphaned, to ensure we don't get a foreign key violation.
	deleteSecretOwner := `
DELETE FROM secret_unit_owner
WHERE unit_uuid = $minimalUnit.uuid
`
	deleteSecretOwnerStmt, err := st.Prepare(deleteSecretOwner, unit)
	if err != nil {
		return errors.Capture(err)
	}

	deleteUnit := `DELETE FROM unit WHERE name = $minimalUnit.name`
	deleteUnitStmt, err := st.Prepare(deleteUnit, unit)
	if err != nil {
		return errors.Capture(err)
	}

	deleteNode := `
DELETE FROM net_node WHERE uuid = (
    SELECT net_node_uuid FROM unit WHERE name = $minimalUnit.name
)
`
	deleteNodeStmt, err := st.Prepare(deleteNode, unit)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, queryUnitStmt, unit).Get(&unit)
	if errors.Is(err, sqlair.ErrNoRows) {
		// Unit already deleted is a no op.
		return nil
	}
	if err != nil {
		return errors.Errorf("looking up UUID for unit %q: %w", unitName, err)
	}

	err = tx.Query(ctx, deleteSecretOwnerStmt, unit).Run()
	if err != nil {
		return errors.Errorf("deleting secret owner for unit %q: %w", unitName, err)
	}

	if err := st.deleteCloudContainer(ctx, tx, unit.UUID, unit.NetNodeID); err != nil {
		return errors.Errorf("deleting cloud container for unit %q: %w", unitName, err)
	}

	if err := st.deletePorts(ctx, tx, unit.UUID); err != nil {
		return errors.Errorf("deleting port ranges for unit %q: %w", unitName, err)
	}

	if err := st.deleteConstraints(ctx, tx, unit.UUID); err != nil {
		return errors.Errorf("deleting constraints for unit %q: %w", unitName, err)
	}
	// TODO(units) - delete storage, annotations

	if err := st.deleteSimpleUnitReferences(ctx, tx, unit.UUID); err != nil {
		return errors.Errorf("deleting associated records for unit %q: %w", unitName, err)
	}

	if err := tx.Query(ctx, deleteUnitStmt, unit).Run(); err != nil {
		return errors.Errorf("deleting unit %q: %w", unitName, err)
	}
	if err := tx.Query(ctx, deleteNodeStmt, unit).Run(); err != nil {
		return errors.Errorf("deleting net node for unit  %q: %w", unitName, err)
	}
	return nil
}

func (st *State) deleteSimpleUnitReferences(ctx context.Context, tx *sqlair.TX, unitUUID coreunit.UUID) error {
	unit := minimalUnit{UUID: unitUUID}

	for _, table := range []string{
		"unit_agent_version",
		"unit_state",
		"unit_state_charm",
		"unit_state_relation",
		"unit_agent_status",
		"unit_workload_status",
		"k8s_pod_status",
	} {
		deleteUnitReference := fmt.Sprintf(`DELETE FROM %s WHERE unit_uuid = $minimalUnit.uuid`, table)
		deleteUnitReferenceStmt, err := st.Prepare(deleteUnitReference, unit)
		if err != nil {
			return errors.Capture(err)
		}

		if err := tx.Query(ctx, deleteUnitReferenceStmt, unit).Run(); err != nil {
			return errors.Errorf("deleting reference to unit in table %q: %w", table, err)
		}
	}
	return nil
}

func (st *State) getUnitLifeAndNetNode(ctx context.Context, tx *sqlair.TX, unitUUID coreunit.UUID) (life.Life, string, error) {
	unit := minimalUnit{UUID: unitUUID}
	queryUnit := `
SELECT &minimalUnit.*
FROM unit
WHERE uuid = $minimalUnit.uuid
`
	queryUnitStmt, err := st.Prepare(queryUnit, unit)
	if err != nil {
		return 0, "", errors.Capture(err)
	}

	err = tx.Query(ctx, queryUnitStmt, unit).Get(&unit)
	if err != nil {
		if !errors.Is(err, sqlair.ErrNoRows) {
			return 0, "", errors.Errorf("querying unit %q life: %w", unitUUID, err)
		}
		return 0, "", errors.Errorf("%w: %s", applicationerrors.UnitNotFound, unitUUID)
	}
	return unit.LifeID, unit.NetNodeID, nil
}

// SetUnitLife sets the life of the specified unit, returning an error
// satisfying [applicationerrors.UnitNotFound] if the unit is not found.
func (st *State) SetUnitLife(ctx context.Context, unitName coreunit.Name, l life.Life) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.setUnitLife(ctx, tx, unitName, l)
	})
	if err != nil {
		return errors.Errorf("updating unit life for %q: %w", unitName, err)
	}
	return nil
}

// TODO(units) - check for subordinates and storage attachments
// For IAAS units, we need to do additional checks - these are still done in mongo.
// If a unit still has subordinates, return applicationerrors.UnitHasSubordinates.
// If a unit still has storage attachments, return applicationerrors.UnitHasStorageAttachments.
func (st *State) setUnitLife(ctx context.Context, tx *sqlair.TX, unitName coreunit.Name, l life.Life) error {
	unit := minimalUnit{Name: unitName, LifeID: l}
	query := `
SELECT &minimalUnit.uuid
FROM unit
WHERE name = $minimalUnit.name
`
	stmt, err := st.Prepare(query, unit)
	if err != nil {
		return errors.Capture(err)
	}

	updateLifeQuery := `
UPDATE unit
SET life_id = $minimalUnit.life_id
WHERE name = $minimalUnit.name
-- we ensure the life can never go backwards.
AND life_id < $minimalUnit.life_id
`

	updateLifeStmt, err := st.Prepare(updateLifeQuery, unit)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, unit).Get(&unit)
	if errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("unit %q not found", unitName).Add(applicationerrors.UnitNotFound)
	} else if err != nil {
		return errors.Errorf("querying unit %q: %w", unitName, err)
	}
	return tx.Query(ctx, updateLifeStmt, unit).Run()

}

// status data. If returns an error satisfying [applicationerrors.UnitNotFound]
// if the unit doesn't exist.
func (st *State) setUnitAgentStatus(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID coreunit.UUID,
	sts *status.StatusInfo[status.UnitAgentStatusType],
) error {
	if sts == nil {
		return nil
	}

	statusID, err := status.EncodeAgentStatus(sts.Status)
	if err != nil {
		return errors.Capture(err)
	}

	statusInfo := unitStatusInfo{
		UnitUUID:  unitUUID,
		StatusID:  statusID,
		Message:   sts.Message,
		Data:      sts.Data,
		UpdatedAt: sts.Since,
	}
	stmt, err := st.Prepare(`
INSERT INTO unit_agent_status (*) VALUES ($unitStatusInfo.*)
ON CONFLICT(unit_uuid) DO UPDATE SET
    status_id = excluded.status_id,
    message = excluded.message,
    updated_at = excluded.updated_at,
    data = excluded.data;
`, statusInfo)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, stmt, statusInfo).Run(); internaldatabase.IsErrConstraintForeignKey(err) {
		return errors.Errorf("%w: %q", applicationerrors.UnitNotFound, unitUUID)
	} else if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// setUnitWorkloadStatus saves the given unit workload status, overwriting any
// current status data. If returns an error satisfying
// [applicationerrors.UnitNotFound] if the unit doesn't exist.
func (st *State) setUnitWorkloadStatus(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID coreunit.UUID,
	sts *status.StatusInfo[status.WorkloadStatusType],
) error {
	if sts == nil {
		return nil
	}

	statusID, err := status.EncodeWorkloadStatus(sts.Status)
	if err != nil {
		return errors.Capture(err)
	}

	statusInfo := unitStatusInfo{
		UnitUUID:  unitUUID,
		StatusID:  statusID,
		Message:   sts.Message,
		Data:      sts.Data,
		UpdatedAt: sts.Since,
	}
	stmt, err := st.Prepare(`
INSERT INTO unit_workload_status (*) VALUES ($unitStatusInfo.*)
ON CONFLICT(unit_uuid) DO UPDATE SET
    status_id = excluded.status_id,
    message = excluded.message,
    updated_at = excluded.updated_at,
    data = excluded.data;
`, statusInfo)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, stmt, statusInfo).Run(); internaldatabase.IsErrConstraintForeignKey(err) {
		return errors.Errorf("%w: %q", applicationerrors.UnitNotFound, unitUUID)
	} else if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// InitialWatchStatementUnitLife returns the initial namespace query for the
// application unit life watcher.
func (st *State) InitialWatchStatementUnitLife(appName string) (string, eventsource.NamespaceQuery) {
	queryFunc := func(ctx context.Context, runner database.TxnRunner) ([]string, error) {
		app := applicationName{Name: appName}
		stmt, err := st.Prepare(`
SELECT u.uuid AS &unitDetails.uuid
FROM unit u
JOIN application a ON a.uuid = u.application_uuid
WHERE a.name = $applicationName.name
`, app, unitDetails{})
		if err != nil {
			return nil, errors.Capture(err)
		}
		var result []unitDetails
		err = runner.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			err := tx.Query(ctx, stmt, app).GetAll(&result)
			if errors.Is(err, sqlair.ErrNoRows) {
				return nil
			}
			return errors.Capture(err)
		})
		if err != nil {
			return nil, errors.Errorf("querying unit IDs for %q: %w", appName, err)
		}
		uuids := make([]string, len(result))
		for i, r := range result {
			uuids[i] = r.UnitUUID.String()
		}
		return uuids, nil
	}
	return "unit", queryFunc
}

// GetApplicationUnitLife returns the life values for the specified units of the
// given application. The supplied ids may belong to a different application;
// the application name is used to filter.
func (st *State) GetApplicationUnitLife(ctx context.Context, appName string, ids ...coreunit.UUID) (map[coreunit.UUID]life.Life, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}
	unitUUIDs := unitUUIDs(ids)

	lifeQuery := `
SELECT (u.uuid, u.life_id) AS (&unitDetails.*)
FROM unit u
JOIN application a ON a.uuid = u.application_uuid
WHERE u.uuid IN ($unitUUIDs[:])
AND a.name = $applicationName.name
`

	app := applicationName{Name: appName}
	lifeStmt, err := st.Prepare(lifeQuery, app, unitDetails{}, unitUUIDs)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var lifes []unitDetails
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, lifeStmt, unitUUIDs, app).GetAll(&lifes)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return errors.Capture(err)
	})
	if err != nil {
		return nil, errors.Errorf("querying unit life for %q: %w", appName, err)
	}
	result := make(map[coreunit.UUID]life.Life)
	for _, u := range lifes {
		result[u.UnitUUID] = u.LifeID
	}
	return result, nil
}

// AddIAASUnits adds the specified units to the application.
//   - If any of the units already exists [applicationerrors.UnitAlreadyExists] is returned.
//   - If the application is not alive, [applicationerrors.ApplicationNotAlive] is returned.
//   - If the application is not found, [applicationerrors.ApplicationNotFound] is returned.
func (st *State) AddIAASUnits(
	ctx context.Context, storageParentDir string, appUUID coreapplication.ID, charmUUID corecharm.ID, args ...application.AddUnitArg,
) error {
	if len(args) == 0 {
		return nil
	}

	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkApplicationAlive(ctx, tx, appUUID); err != nil {
			return errors.Capture(err)
		}

		// TODO(storage) - read and use storage directives
		for _, arg := range args {
			insertArg := application.InsertUnitArg{
				UnitName:    arg.UnitName,
				Constraints: arg.Constraints,
				UnitStatusArg: application.UnitStatusArg{
					AgentStatus:    arg.UnitStatusArg.AgentStatus,
					WorkloadStatus: arg.UnitStatusArg.WorkloadStatus,
				},
				StorageParentDir: storageParentDir,
			}
			if err = st.insertIAASUnit(ctx, tx, appUUID, insertArg); err != nil {
				return errors.Errorf("inserting unit %q: %w ", arg.UnitName, err)
			}
		}
		return nil
	})
	return errors.Capture(err)
}

// AddCAASUnits adds the specified units to the application.
//   - If any of the units already exists [applicationerrors.UnitAlreadyExists] is returned.
//   - If the application is not alive, [applicationerrors.ApplicationNotAlive] is returned.
//   - If the application is not found, [applicationerrors.ApplicationNotFound] is returned.
func (st *State) AddCAASUnits(
	ctx context.Context, storageParentDir string, appUUID coreapplication.ID, charmUUID corecharm.ID, args ...application.AddUnitArg,
) error {
	if len(args) == 0 {
		return nil
	}

	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// TODO(storage) - read and use storage directives
		for _, arg := range args {
			insertArg := application.InsertUnitArg{
				UnitName:    arg.UnitName,
				Constraints: arg.Constraints,
				UnitStatusArg: application.UnitStatusArg{
					AgentStatus:    arg.UnitStatusArg.AgentStatus,
					WorkloadStatus: arg.UnitStatusArg.WorkloadStatus,
				},
				StorageParentDir: storageParentDir,
			}
			if err = st.insertCAASUnit(ctx, tx, appUUID, insertArg); err != nil {
				return errors.Errorf("inserting unit %q: %w ", arg.UnitName, err)
			}
		}
		return nil
	})
	return errors.Capture(err)
}

// InsertIAASUnits inserts the fully formed units for the specified IAAS application.
// This is only used when inserting units during model migration.
//   - If any of the units already exists [applicationerrors.UnitAlreadyExists] is returned.
//   - If the application is not alive, [applicationerrors.ApplicationNotAlive] is returned.
//   - If the application is not found, [applicationerrors.ApplicationNotFound] is returned.
func (st *State) InsertMigratingIAASUnits(ctx context.Context, appUUID coreapplication.ID, units ...application.InsertUnitArg) error {
	if len(units) == 0 {
		return nil
	}
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for _, arg := range units {
			if err := st.insertIAASUnit(ctx, tx, appUUID, arg); err != nil {
				return errors.Errorf("inserting IAAS unit %q: %w", arg.UnitName, err)
			}
		}
		return nil
	})
}

// InsertCAASUnits inserts the fully formed units for the specified CAAS application.
// This is only used when inserting units during model migration.
//   - If any of the units already exists [applicationerrors.UnitAlreadyExists] is returned.
//   - If the application is not alive, [applicationerrors.ApplicationNotAlive] is returned.
//   - If the application is not found, [applicationerrors.ApplicationNotFound] is returned.
func (st *State) InsertMigratingCAASUnits(ctx context.Context, appUUID coreapplication.ID, units ...application.InsertUnitArg) error {
	if len(units) == 0 {
		return nil
	}
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for _, arg := range units {
			if err := st.insertCAASUnit(ctx, tx, appUUID, arg); err != nil {
				return errors.Errorf("inserting CAAS unit %q: %w", arg.UnitName, err)
			}
		}
		return nil
	})
}

// SetRunningAgentBinaryVersion sets the running agent binary version for the
// provided unit uuid. Any previously set values for this unit uuid will
// be overwritten by this call.
//
// The following errors can be expected:
// - [errors.UnitNotFound] if the unit does not exist.
// - [coreerrors.NotSupported] if the architecture is not known to the database.
func (st *State) SetRunningAgentBinaryVersion(ctx context.Context, uuid coreunit.UUID, version coreagentbinary.Version) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	archMap := architectureMap{Name: version.Arch}

	archMapStmt, err := st.Prepare(`
SELECT id AS &architectureMap.id FROM architecture WHERE name = $architectureMap.name
`, archMap)
	if err != nil {
		return errors.Capture(err)
	}

	unitUUID := unitUUID{UnitUUID: uuid}

	unitAgentVersion := unitAgentVersion{
		UnitUUID: unitUUID.UnitUUID.String(),
		Version:  version.Number.String(),
	}

	upsertRunningVersionStmt, err := st.Prepare(`
INSERT INTO unit_agent_version (*)
VALUES ($unitAgentVersion.*)
ON CONFLICT (unit_uuid) DO
UPDATE SET version = excluded.version,
           architecture_id = excluded.architecture_id
`, unitAgentVersion)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {

		// Check if unit exists and is not dead.
		err := st.checkUnitNotDead(ctx, tx, unitUUID)
		if err != nil {
			return errors.Errorf(
				"checking unit %q exists: %w", uuid, err,
			)
		}

		// Look up architecture ID.
		err = tx.Query(ctx, archMapStmt, archMap).Get(&archMap)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"architecture %q is unsupported", version.Arch,
			).Add(coreerrors.NotSupported)
		} else if err != nil {
			return errors.Errorf(
				"looking up id for architecture %q: %w", version.Arch, err,
			)
		}

		unitAgentVersion.ArchitectureID = archMap.ID
		return tx.Query(ctx, upsertRunningVersionStmt, unitAgentVersion).Run()
	})

	if err != nil {
		return errors.Errorf(
			"setting running agent binary version for unit %q: %w",
			uuid, err,
		)
	}

	return nil
}

// GetUnitUUIDByName returns the UUID for the named unit, returning an error
// satisfying [applicationerrors.UnitNotFound] if the unit doesn't exist.
func (st *State) GetUnitUUIDByName(ctx context.Context, name coreunit.Name) (coreunit.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}
	unitName := unitName{Name: name}

	query, err := st.Prepare(`
SELECT &unitUUID.*
FROM unit
WHERE name = $unitName.name
`, unitUUID{}, unitName)
	if err != nil {
		return "", errors.Errorf("preparing query: %w", err)
	}

	unitUUID := unitUUID{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, query, unitName).Get(&unitUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("unit %q not found", name).Add(applicationerrors.UnitNotFound)
		}
		return err
	})
	if err != nil {
		return "", errors.Errorf("querying unit name: %w", err)
	}

	return unitUUID.UnitUUID, nil
}

func (st *State) getUnitDetails(ctx context.Context, tx *sqlair.TX, unitName coreunit.Name) (*unitDetails, error) {
	unit := unitDetails{
		Name: unitName,
	}

	getUnit := `SELECT &unitDetails.* FROM unit WHERE name = $unitDetails.name`
	getUnitStmt, err := st.Prepare(getUnit, unit)
	if err != nil {
		return nil, errors.Capture(err)
	}

	err = tx.Query(ctx, getUnitStmt, unit).Get(&unit)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("unit %q not found", unitName).Add(applicationerrors.UnitNotFound)
	} else if err != nil {
		return nil, errors.Capture(err)
	}

	return &unit, nil
}

func makeCloudContainerArg(unitName coreunit.Name, cloudContainer application.CloudContainerParams) *application.CloudContainer {
	result := &application.CloudContainer{
		ProviderID: cloudContainer.ProviderID,
		Ports:      cloudContainer.Ports,
	}
	if cloudContainer.Address != nil {
		// TODO(units) - handle the cloudContainer.Address space ID
		// For k8s we'll initially create a /32 subnet off the container address
		// and add that to the default space.
		result.Address = &application.ContainerAddress{
			// For cloud containers, the device is a placeholder without
			// a MAC address and once inserted, not updated. It just exists
			// to tie the address to the net node corresponding to the
			// cloud container.
			Device: application.ContainerDevice{
				Name:              fmt.Sprintf("placeholder for %q cloud container", unitName),
				DeviceTypeID:      linklayerdevice.DeviceTypeUnknown,
				VirtualPortTypeID: linklayerdevice.NonVirtualPortType,
			},
			Value:       cloudContainer.Address.Value,
			AddressType: ipaddress.MarshallAddressType(cloudContainer.Address.AddressType()),
			Scope:       ipaddress.MarshallScope(cloudContainer.Address.Scope),
			Origin:      ipaddress.MarshallOrigin(network.OriginProvider),
			ConfigType:  ipaddress.MarshallConfigType(network.ConfigDHCP),
		}
		if cloudContainer.AddressOrigin != nil {
			result.Address.Origin = ipaddress.MarshallOrigin(*cloudContainer.AddressOrigin)
		}
	}
	return result
}

// RegisterCAASUnit registers the specified CAAS application unit.
// The following errors can be expected:
// - [applicationerrors.ApplicationNotAlive] when the application is not alive
// - [applicationerrors.UnitAlreadyExists] when the unit exists
// - [applicationerrors.UnitNotAssigned] when the unit was not assigned
func (st *State) RegisterCAASUnit(ctx context.Context, appName string, arg application.RegisterCAASUnitArg) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	cloudContainerParams := application.CloudContainerParams{
		ProviderID: arg.ProviderID,
		Ports:      arg.Ports,
	}
	if arg.Address != nil {
		addr := network.NewSpaceAddress(*arg.Address, network.WithScope(network.ScopeMachineLocal))
		cloudContainerParams.Address = &addr
		origin := network.OriginProvider
		cloudContainerParams.AddressOrigin = &origin
	}
	cloudContainer := makeCloudContainerArg(arg.UnitName, cloudContainerParams)

	now := ptr(st.clock.Now())
	insertArg := application.InsertUnitArg{
		UnitName: arg.UnitName,
		Password: &application.PasswordInfo{
			PasswordHash:  arg.PasswordHash,
			HashAlgorithm: application.HashAlgorithmSHA256,
		},
		CloudContainer: cloudContainer,
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusAllocating,
				Since:  now,
			},
			WorkloadStatus: &status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusWaiting,
				Message: corestatus.MessageInstallingAgent,
				Since:   now,
			},
		},
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		appDetails, err := st.getApplicationDetails(ctx, tx, appName)
		if err != nil {
			return errors.Errorf("querying life for application %q: %w", appName, err)
		}
		if appDetails.LifeID != life.Alive {
			return errors.Errorf("registering application %q: %w", appName, applicationerrors.ApplicationNotAlive)
		}
		appUUID := appDetails.UUID

		unitLife, err := st.getLifeForUnitName(ctx, tx, arg.UnitName)
		if errors.Is(err, applicationerrors.UnitNotFound) {
			appScale, err := st.getApplicationScaleState(ctx, tx, appUUID)
			if err != nil {
				return errors.Errorf("getting application scale state for app %q: %w", appUUID, err)
			}
			if arg.OrderedId >= appScale.Scale ||
				(appScale.Scaling && arg.OrderedId >= appScale.ScaleTarget) {
				return errors.Errorf("unrequired unit %s is not assigned", arg.UnitName).Add(applicationerrors.UnitNotAssigned)
			}

			return st.insertCAASUnit(ctx, tx, appUUID, insertArg)
		} else if err != nil {
			return errors.Errorf("checking unit life %q: %w", arg.UnitName, err)
		}
		if unitLife == life.Dead {
			return errors.Errorf("dead unit %q already exists", arg.UnitName).Add(applicationerrors.UnitAlreadyExists)
		}

		// Unit already exists and is not dead. Update the cloud container.
		toUpdate, err := st.getUnitDetails(ctx, tx, arg.UnitName)
		if err != nil {
			return errors.Capture(err)
		}
		err = st.upsertUnitCloudContainer(ctx, tx, toUpdate.Name, toUpdate.UnitUUID, toUpdate.NetNodeID, cloudContainer)
		if err != nil {
			return errors.Errorf("updating cloud container for unit %q: %w", arg.UnitName, err)
		}

		err = st.setUnitPassword(ctx, tx, toUpdate.UnitUUID, application.PasswordInfo{
			PasswordHash:  arg.PasswordHash,
			HashAlgorithm: application.HashAlgorithmSHA256,
		})
		if err != nil {
			return errors.Errorf("setting password for unit %q: %w", arg.UnitName, err)
		}
		return nil
	})
	if err != nil {
		return errors.Capture(err)
	}
	return nil
}

func (st *State) setUnitPassword(ctx context.Context, tx *sqlair.TX, unitUUID coreunit.UUID, password application.PasswordInfo) error {
	info := unitPassword{
		UnitUUID:                unitUUID,
		PasswordHash:            password.PasswordHash,
		PasswordHashAlgorithmID: password.HashAlgorithm,
	}
	updatePasswordStmt, err := st.Prepare(`
UPDATE unit SET
    password_hash = $unitPassword.password_hash,
    password_hash_algorithm_id = $unitPassword.password_hash_algorithm_id
WHERE uuid = $unitPassword.uuid
`, info)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, updatePasswordStmt, info).Run()
	if err != nil {
		return errors.Errorf("updating password for unit %q: %w", unitUUID, err)
	}
	return nil
}

func (st *State) insertCAASUnit(
	ctx context.Context,
	tx *sqlair.TX,
	appUUID coreapplication.ID,
	args application.InsertUnitArg,
) error {
	unitUUID, nodeUUID, err := st.insertUnit(ctx, tx, appUUID, args)
	if err != nil {
		return errors.Errorf("inserting unit for CAAS application %q: %w", appUUID, err)
	}
	if len(args.Storage) == 0 {
		return nil
	}

	attachArgs, err := st.insertUnitStorage(ctx, tx, appUUID, unitUUID, args.Storage, args.StoragePoolKind)
	if err != nil {
		return errors.Errorf("creating storage for unit %q: %w", args.UnitName, err)
	}
	err = st.attachUnitStorage(ctx, tx, args.StorageParentDir, args.StoragePoolKind, unitUUID, nodeUUID, attachArgs)
	if err != nil {
		return errors.Errorf("attaching storage for unit %q: %w", args.UnitName, err)
	}
	return nil
}

func (st *State) insertIAASUnit(
	ctx context.Context,
	tx *sqlair.TX,
	appUUID coreapplication.ID,
	args application.InsertUnitArg,
) error {
	_, err := st.getUnitDetails(ctx, tx, args.UnitName)
	if err == nil {
		return errors.Errorf("unit %q already exists", args.UnitName).Add(applicationerrors.UnitAlreadyExists)
	}
	if !errors.Is(err, applicationerrors.UnitNotFound) {
		return errors.Errorf("looking up unit %q: %w", args.UnitName, err)
	}

	unitUUID, _, err := st.insertUnit(ctx, tx, appUUID, args)
	if err != nil {
		return errors.Errorf("inserting unit for application %q: %w", appUUID, err)
	}
	if _, err := st.insertUnitStorage(ctx, tx, appUUID, unitUUID, args.Storage, args.StoragePoolKind); err != nil {
		return errors.Errorf("creating storage for unit %q: %w", args.UnitName, err)
	}
	return nil
}

func (st *State) insertUnit(
	ctx context.Context, tx *sqlair.TX,
	appUUID coreapplication.ID,
	args application.InsertUnitArg,
) (coreunit.UUID, string, error) {
	if err := st.checkApplicationAlive(ctx, tx, appUUID); err != nil {
		return "", "", errors.Capture(err)
	}

	charmUUID, err := st.getCharmIDByApplicationID(ctx, tx, appUUID)
	if err != nil {
		return "", "", errors.Errorf("getting charm for application %q: %w", appUUID, err)
	}

	unitUUID, err := coreunit.NewUUID()
	if err != nil {
		return "", "", errors.Capture(err)
	}
	nodeUUID, err := uuid.NewUUID()
	if err != nil {
		return "", "", errors.Capture(err)
	}
	createParams := unitDetails{
		ApplicationID: appUUID,
		UnitUUID:      unitUUID,
		CharmUUID:     charmUUID,
		Name:          args.UnitName,
		NetNodeID:     nodeUUID.String(),
		LifeID:        life.Alive,
	}
	if args.Password != nil {
		// Unit passwords are optional when we insert a unit (they're mainly
		// used for CAAS units). If they are set they must be unique across
		// all units.
		createParams.PasswordHash = sql.NullString{
			String: args.Password.PasswordHash,
			Valid:  true,
		}
		createParams.PasswordHashAlgorithmID = sql.NullInt16{
			Int16: int16(args.Password.HashAlgorithm),
			Valid: true,
		}
	}

	createUnit := `INSERT INTO unit (*) VALUES ($unitDetails.*)`
	createUnitStmt, err := st.Prepare(createUnit, createParams)
	if err != nil {
		return "", "", errors.Capture(err)
	}

	createNode := `INSERT INTO net_node (uuid) VALUES ($unitDetails.net_node_uuid)`
	createNodeStmt, err := st.Prepare(createNode, createParams)
	if err != nil {
		return "", "", errors.Capture(err)
	}

	if err := tx.Query(ctx, createNodeStmt, createParams).Run(); err != nil {
		return "", "", errors.Errorf("creating net node for unit %q: %w", args.UnitName, err)
	}
	if err := tx.Query(ctx, createUnitStmt, createParams).Run(); err != nil {
		return "", "", errors.Errorf("creating unit for unit %q: %w", args.UnitName, err)
	}
	if args.CloudContainer != nil {
		if err := st.upsertUnitCloudContainer(ctx, tx, args.UnitName, unitUUID, nodeUUID.String(), args.CloudContainer); err != nil {
			return "", "", errors.Errorf("creating cloud container for unit %q: %w", args.UnitName, err)
		}
	}

	if err := st.setUnitConstraints(ctx, tx, unitUUID, args.Constraints); err != nil {
		return "", "", errors.Errorf("setting constraints for unit %q: %w", args.UnitName, err)
	}

	if err := st.setUnitAgentStatus(ctx, tx, unitUUID, args.AgentStatus); err != nil {
		return "", "", errors.Errorf("saving agent status for unit %q: %w", args.UnitName, err)
	}
	if err := st.setUnitWorkloadStatus(ctx, tx, unitUUID, args.WorkloadStatus); err != nil {
		return "", "", errors.Errorf("saving workload status for unit %q: %w", args.UnitName, err)
	}
	return unitUUID, nodeUUID.String(), nil
}

// UpdateCAASUnit updates the cloud container for specified unit,
// returning an error satisfying [applicationerrors.UnitNotFoundError]
// if the unit doesn't exist.
func (st *State) UpdateCAASUnit(ctx context.Context, unitName coreunit.Name, params application.UpdateCAASUnitParams) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	var cloudContainer *application.CloudContainer
	if params.ProviderID != nil {
		cloudContainerParams := application.CloudContainerParams{
			ProviderID: *params.ProviderID,
			Ports:      params.Ports,
		}
		if params.Address != nil {
			addr := network.NewSpaceAddress(*params.Address, network.WithScope(network.ScopeMachineLocal))
			cloudContainerParams.Address = &addr
			origin := network.OriginProvider
			cloudContainerParams.AddressOrigin = &origin
		}
		cloudContainer = makeCloudContainerArg(unitName, cloudContainerParams)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		toUpdate, err := st.getUnitDetails(ctx, tx, unitName)
		if err != nil {
			return errors.Errorf("getting unit %q: %w", unitName, err)
		}

		if cloudContainer != nil {
			err = st.upsertUnitCloudContainer(ctx, tx, toUpdate.Name, toUpdate.UnitUUID, toUpdate.NetNodeID, cloudContainer)
			if err != nil {
				return errors.Errorf("updating cloud container for unit %q: %w", unitName, err)
			}
		}

		if err := st.setUnitAgentStatus(ctx, tx, toUpdate.UnitUUID, params.AgentStatus); err != nil {
			return errors.Errorf("saving unit %q agent status: %w", unitName, err)
		}

		if err := st.setUnitWorkloadStatus(ctx, tx, toUpdate.UnitUUID, params.WorkloadStatus); err != nil {
			return errors.Errorf("saving unit %q workload status: %w", unitName, err)
		}
		if err := st.setCloudContainerStatus(ctx, tx, toUpdate.UnitUUID, params.CloudContainerStatus); err != nil {
			return errors.Errorf("saving unit %q cloud container status: %w", unitName, err)
		}

		return nil
	})
	if err != nil {
		return errors.Errorf("updating CAAS unit %q: %w", unitName, err)
	}
	return nil
}

// GetUnitRefreshAttributes returns the unit refresh attributes for the
// specified unit. If the unit is not found, an error satisfying
// [applicationerrors.UnitNotFound] is returned. This doesn't take into account
// life, so it can return the attributes of a unit even if it's dead.
func (st *State) GetUnitRefreshAttributes(ctx context.Context, unitName coreunit.Name) (application.UnitAttributes, error) {
	db, err := st.DB()
	if err != nil {
		return application.UnitAttributes{}, errors.Capture(err)
	}

	unit := unitAttributes{
		Name: unitName,
	}

	getUnit := `SELECT &unitAttributes.* FROM v_unit_attribute WHERE name = $unitAttributes.name`
	getUnitStmt, err := st.Prepare(getUnit, unit)
	if err != nil {
		return application.UnitAttributes{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, getUnitStmt, unit).Get(&unit)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("unit %q not found", unitName).Add(applicationerrors.UnitNotFound)
		} else if err != nil {
			return errors.Capture(err)
		}

		return nil
	})
	if err != nil {
		return application.UnitAttributes{}, errors.Errorf("getting unit %q: %w", unitName, err)
	}

	resolveMode, err := encodeResolveMode(unit.ResolveMode)
	if err != nil {
		return application.UnitAttributes{}, errors.Errorf("encoding resolve mode for unit %q: %w", unitName, err)
	}

	return application.UnitAttributes{
		Life:        unit.LifeID,
		ProviderID:  unit.ProviderID.String,
		ResolveMode: resolveMode,
	}, nil
}

// GetModelConstraints returns the currently set constraints for the model.
// The following error types can be expected:
// - [modelerrors.NotFound]: when no model exists to set constraints for.
// - [modelerrors.ConstraintsNotFound]: when no model constraints have been
// set for the model.
// Note: This method should mirror the model domain method of the same name.
func (st *State) GetModelConstraints(
	ctx context.Context,
) (constraints.Constraints, error) {
	db, err := st.DB()
	if err != nil {
		return constraints.Constraints{}, errors.Capture(err)
	}

	selectTagStmt, err := st.Prepare(
		"SELECT &dbConstraintTag.* FROM v_model_constraint_tag", dbConstraintTag{},
	)
	if err != nil {
		return constraints.Constraints{}, errors.Capture(err)
	}

	selectSpaceStmt, err := st.Prepare(
		"SELECT &dbConstraintSpace.* FROM v_model_constraint_space", dbConstraintSpace{},
	)
	if err != nil {
		return constraints.Constraints{}, errors.Capture(err)
	}

	selectZoneStmt, err := st.Prepare(
		"SELECT &dbConstraintZone.* FROM v_model_constraint_zone", dbConstraintZone{})
	if err != nil {
		return constraints.Constraints{}, errors.Capture(err)
	}

	var (
		cons   dbConstraint
		tags   []dbConstraintTag
		spaces []dbConstraintSpace
		zones  []dbConstraintZone
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := modelExists(ctx, st, tx); err != nil {
			return errors.Capture(err)
		}

		cons, err = st.getModelConstraints(ctx, tx)
		if err != nil {
			return errors.Capture(err)
		}
		err = tx.Query(ctx, selectTagStmt).GetAll(&tags)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting constraint tags: %w", err)
		}
		err = tx.Query(ctx, selectSpaceStmt).GetAll(&spaces)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting constraint spaces: %w", err)
		}
		err = tx.Query(ctx, selectZoneStmt).GetAll(&zones)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting constraint zones: %w", err)
		}
		return nil
	})
	if err != nil {
		return constraints.Constraints{}, errors.Capture(err)
	}

	return cons.toValue(tags, spaces, zones)
}

// SetUnitConstraints sets the unit constraints for the specified application
// ID. This method overwrites the full constraints on every call. If invalid
// constraints are provided (e.g. invalid container type or non-existing space),
// a [applicationerrors.InvalidUnitConstraints] error is returned. If the unit
// is dead, an error satisfying [applicationerrors.UnitIsDead] is returned.
func (st *State) SetUnitConstraints(ctx context.Context, inUnitUUID coreunit.UUID, cons constraints.Constraints) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.setUnitConstraints(ctx, tx, inUnitUUID, cons)
	})
}

func (st *State) setUnitConstraints(ctx context.Context, tx *sqlair.TX, inUnitUUID coreunit.UUID, cons constraints.Constraints) error {
	cUUID, err := uuid.NewUUID()
	if err != nil {
		return errors.Capture(err)
	}
	cUUIDStr := cUUID.String()

	selectConstraintUUIDQuery := `
SELECT &constraintUUID.*
FROM unit_constraint
WHERE unit_uuid = $unitConstraintUUID.unit_uuid
`
	selectConstraintUUIDStmt, err := st.Prepare(selectConstraintUUIDQuery, constraintUUID{}, unitConstraintUUID{})
	if err != nil {
		return errors.Errorf("preparing select unit constraint uuid query: %w", err)
	}

	// Check that spaces provided as constraints do exist in the space table.
	selectSpaceQuery := `SELECT &spaceUUID.uuid FROM space WHERE name = $spaceName.name`
	selectSpaceStmt, err := st.Prepare(selectSpaceQuery, spaceUUID{}, spaceName{})
	if err != nil {
		return errors.Errorf("preparing select space query: %w", err)
	}

	// Cleanup all previous tags, spaces and zones from their join tables.
	deleteConstraintTagsQuery := `DELETE FROM constraint_tag WHERE constraint_uuid = $constraintUUID.constraint_uuid`
	deleteConstraintTagsStmt, err := st.Prepare(deleteConstraintTagsQuery, constraintUUID{})
	if err != nil {
		return errors.Errorf("preparing delete constraint tags query: %w", err)
	}
	deleteConstraintSpacesQuery := `DELETE FROM constraint_space WHERE constraint_uuid = $constraintUUID.constraint_uuid`
	deleteConstraintSpacesStmt, err := st.Prepare(deleteConstraintSpacesQuery, constraintUUID{})
	if err != nil {
		return errors.Errorf("preparing delete constraint spaces query: %w", err)
	}
	deleteConstraintZonesQuery := `DELETE FROM constraint_zone WHERE constraint_uuid = $constraintUUID.constraint_uuid`
	deleteConstraintZonesStmt, err := st.Prepare(deleteConstraintZonesQuery, constraintUUID{})
	if err != nil {
		return errors.Errorf("preparing delete constraint zones query: %w", err)
	}

	selectContainerTypeIDQuery := `SELECT &containerTypeID.id FROM container_type WHERE value = $containerTypeVal.value`
	selectContainerTypeIDStmt, err := st.Prepare(selectContainerTypeIDQuery, containerTypeID{}, containerTypeVal{})
	if err != nil {
		return errors.Errorf("preparing select container type id query: %w", err)
	}

	insertConstraintsQuery := `
INSERT INTO "constraint"(*)
VALUES ($setConstraint.*)
ON CONFLICT (uuid) DO UPDATE SET
    arch = excluded.arch,
    cpu_cores = excluded.cpu_cores,
    cpu_power = excluded.cpu_power,
    mem = excluded.mem,
    root_disk= excluded.root_disk,
    root_disk_source = excluded.root_disk_source,
    instance_role = excluded.instance_role,
    instance_type = excluded.instance_type,
    container_type_id = excluded.container_type_id,
    virt_type = excluded.virt_type,
    allocate_public_ip = excluded.allocate_public_ip,
    image_id = excluded.image_id
`
	insertConstraintsStmt, err := st.Prepare(insertConstraintsQuery, setConstraint{})
	if err != nil {
		return errors.Errorf("preparing insert constraints query: %w", err)
	}

	insertConstraintTagsQuery := `INSERT INTO constraint_tag(*) VALUES ($setConstraintTag.*)`
	insertConstraintTagsStmt, err := st.Prepare(insertConstraintTagsQuery, setConstraintTag{})
	if err != nil {
		return errors.Errorf("preparing insert constraint tags query: %w", err)
	}

	insertConstraintSpacesQuery := `INSERT INTO constraint_space(*) VALUES ($setConstraintSpace.*)`
	insertConstraintSpacesStmt, err := st.Prepare(insertConstraintSpacesQuery, setConstraintSpace{})
	if err != nil {
		return errors.Capture(err)
	}

	insertConstraintZonesQuery := `INSERT INTO constraint_zone(*) VALUES ($setConstraintZone.*)`
	insertConstraintZonesStmt, err := st.Prepare(insertConstraintZonesQuery, setConstraintZone{})
	if err != nil {
		return errors.Capture(err)
	}

	insertUnitConstraintsQuery := `
INSERT INTO unit_constraint(*)
VALUES ($setUnitConstraint.*)
ON CONFLICT (unit_uuid) DO NOTHING
`
	insertUnitConstraintsStmt, err := st.Prepare(insertUnitConstraintsQuery, setUnitConstraint{})
	if err != nil {
		return errors.Errorf("preparing insert unit constraints query: %w", err)
	}

	if err := st.checkUnitNotDead(ctx, tx, unitUUID{UnitUUID: inUnitUUID}); err != nil {
		return errors.Capture(err)
	}

	var containerTypeID containerTypeID
	if cons.Container != nil {
		err = tx.Query(ctx, selectContainerTypeIDStmt, containerTypeVal{Value: string(*cons.Container)}).Get(&containerTypeID)
		if errors.Is(err, sql.ErrNoRows) {
			st.logger.Warningf(ctx, "cannot set constraints, container type %q does not exist", *cons.Container)
			return applicationerrors.InvalidUnitConstraints
		}
		if err != nil {
			return errors.Capture(err)
		}
	}

	// First check if the constraint already exists, in that case
	// we need to update it, unsetting the nil values.
	var retrievedConstraintUUID constraintUUID
	err = tx.Query(ctx, selectConstraintUUIDStmt, unitConstraintUUID{UnitUUID: inUnitUUID.String()}).Get(&retrievedConstraintUUID)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Capture(err)
	} else if err == nil {
		cUUIDStr = retrievedConstraintUUID.ConstraintUUID
	}

	// Cleanup tags, spaces and zones from their join tables.
	if err := tx.Query(ctx, deleteConstraintTagsStmt, constraintUUID{ConstraintUUID: cUUIDStr}).Run(); err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, deleteConstraintSpacesStmt, constraintUUID{ConstraintUUID: cUUIDStr}).Run(); err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, deleteConstraintZonesStmt, constraintUUID{ConstraintUUID: cUUIDStr}).Run(); err != nil {
		return errors.Capture(err)
	}

	constraints := encodeConstraints(cUUIDStr, cons, containerTypeID.ID)

	if err := tx.Query(ctx, insertConstraintsStmt, constraints).Run(); err != nil {
		return errors.Capture(err)
	}

	if cons.Tags != nil {
		for _, tag := range *cons.Tags {
			constraintTag := setConstraintTag{ConstraintUUID: cUUIDStr, Tag: tag}
			if err := tx.Query(ctx, insertConstraintTagsStmt, constraintTag).Run(); err != nil {
				return errors.Capture(err)
			}
		}
	}

	if cons.Spaces != nil {
		for _, space := range *cons.Spaces {
			// Make sure the space actually exists.
			var spaceUUID spaceUUID
			err := tx.Query(ctx, selectSpaceStmt, spaceName{Name: space.SpaceName}).Get(&spaceUUID)
			if errors.Is(err, sql.ErrNoRows) {
				st.logger.Warningf(ctx, "cannot set constraints, space %q does not exist", space)
				return applicationerrors.InvalidUnitConstraints
			}
			if err != nil {
				return errors.Capture(err)
			}

			constraintSpace := setConstraintSpace{ConstraintUUID: cUUIDStr, Space: space.SpaceName, Exclude: space.Exclude}
			if err := tx.Query(ctx, insertConstraintSpacesStmt, constraintSpace).Run(); err != nil {
				return errors.Capture(err)
			}
		}
	}

	if cons.Zones != nil {
		for _, zone := range *cons.Zones {
			constraintZone := setConstraintZone{ConstraintUUID: cUUIDStr, Zone: zone}
			if err := tx.Query(ctx, insertConstraintZonesStmt, constraintZone).Run(); err != nil {
				return errors.Capture(err)
			}
		}
	}

	return errors.Capture(
		tx.Query(ctx, insertUnitConstraintsStmt, setUnitConstraint{
			UnitUUID:       inUnitUUID.String(),
			ConstraintUUID: cUUIDStr,
		}).Run(),
	)
}

func (st *State) upsertUnitCloudContainer(
	ctx context.Context, tx *sqlair.TX, unitName coreunit.Name, unitUUID coreunit.UUID, netNodeUUID string, cc *application.CloudContainer,
) error {
	containerInfo := cloudContainer{
		UnitUUID:   unitUUID,
		ProviderID: cc.ProviderID,
	}

	queryStmt, err := st.Prepare(`
SELECT &cloudContainer.*
FROM k8s_pod
WHERE unit_uuid = $cloudContainer.unit_uuid
`, containerInfo)
	if err != nil {
		return errors.Capture(err)
	}

	insertStmt, err := st.Prepare(`
INSERT INTO k8s_pod (*) VALUES ($cloudContainer.*)
`, containerInfo)
	if err != nil {
		return errors.Capture(err)
	}

	updateStmt, err := st.Prepare(`
UPDATE k8s_pod SET
    provider_id = $cloudContainer.provider_id
WHERE unit_uuid = $cloudContainer.unit_uuid
`, containerInfo)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, queryStmt, containerInfo).Get(&containerInfo)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("looking up cloud container %q: %w", unitName, err)
	}
	if err == nil {
		newProviderID := cc.ProviderID
		if newProviderID != "" &&
			containerInfo.ProviderID != newProviderID {
			st.logger.Debugf(ctx, "unit %q has provider id %q which changed to %q",
				unitName, containerInfo.ProviderID, newProviderID)
		}
		containerInfo.ProviderID = newProviderID
		if err := tx.Query(ctx, updateStmt, containerInfo).Run(); err != nil {
			return errors.Errorf("updating cloud container for unit %q: %w", unitName, err)
		}
	} else {
		if err := tx.Query(ctx, insertStmt, containerInfo).Run(); err != nil {
			return errors.Errorf("inserting cloud container for unit %q: %w", unitName, err)
		}
	}

	if cc.Address != nil {
		if err := st.upsertCloudContainerAddress(ctx, tx, unitName, netNodeUUID, *cc.Address); err != nil {
			return errors.Errorf("updating cloud container address for unit %q: %w", unitName, err)
		}
	}
	if cc.Ports != nil {
		if err := st.upsertCloudContainerPorts(ctx, tx, unitUUID, *cc.Ports); err != nil {
			return errors.Errorf("updating cloud container ports for unit %q: %w", unitName, err)
		}
	}
	return nil
}

func (st *State) upsertCloudContainerAddress(
	ctx context.Context, tx *sqlair.TX, unitName coreunit.Name, netNodeID string, address application.ContainerAddress,
) error {
	// First ensure the address link layer device is upserted.
	// For cloud containers, the device is a placeholder without
	// a MAC address. It just exits to tie the address to the
	// net node corresponding to the cloud container.
	cloudContainerDeviceInfo := cloudContainerDevice{
		Name:              address.Device.Name,
		NetNodeID:         netNodeID,
		DeviceTypeID:      int(address.Device.DeviceTypeID),
		VirtualPortTypeID: int(address.Device.VirtualPortTypeID),
	}

	selectCloudContainerDeviceStmt, err := st.Prepare(`
SELECT &cloudContainerDevice.uuid
FROM link_layer_device
WHERE net_node_uuid = $cloudContainerDevice.net_node_uuid
`, cloudContainerDeviceInfo)
	if err != nil {
		return errors.Capture(err)
	}

	insertCloudContainerDeviceStmt, err := st.Prepare(`
INSERT INTO link_layer_device (*) VALUES ($cloudContainerDevice.*)
`, cloudContainerDeviceInfo)
	if err != nil {
		return errors.Capture(err)
	}

	// See if the link layer device exists, if not insert it.
	err = tx.Query(ctx, selectCloudContainerDeviceStmt, cloudContainerDeviceInfo).Get(&cloudContainerDeviceInfo)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("querying cloud container link layer device for unit %q: %w", unitName, err)
	}
	if errors.Is(err, sqlair.ErrNoRows) {
		deviceUUID, err := uuid.NewUUID()
		if err != nil {
			return errors.Capture(err)
		}
		cloudContainerDeviceInfo.UUID = deviceUUID.String()
		if err := tx.Query(ctx, insertCloudContainerDeviceStmt, cloudContainerDeviceInfo).Run(); err != nil {
			return errors.Errorf("inserting cloud container device for unit %q: %w", unitName, err)
		}
	}
	deviceUUID := cloudContainerDeviceInfo.UUID

	// Now process the address details.
	ipAddr := ipAddress{
		Value:        address.Value,
		ConfigTypeID: int(address.ConfigType),
		TypeID:       int(address.AddressType),
		OriginID:     int(address.Origin),
		ScopeID:      int(address.Scope),
		DeviceID:     deviceUUID,
	}

	selectAddressUUIDStmt, err := st.Prepare(`
SELECT &ipAddress.uuid
FROM ip_address
WHERE device_uuid = $ipAddress.device_uuid;
`, ipAddr)
	if err != nil {
		return errors.Capture(err)
	}

	upsertAddressStmt, err := sqlair.Prepare(`
INSERT INTO ip_address (*)
VALUES ($ipAddress.*)
ON CONFLICT(uuid) DO UPDATE SET
    address_value = excluded.address_value,
    type_id = excluded.type_id,
    scope_id = excluded.scope_id,
    origin_id = excluded.origin_id,
    config_type_id = excluded.config_type_id
`, ipAddr)
	if err != nil {
		return errors.Capture(err)
	}

	// Container addresses are never deleted unless the container itself is deleted.
	// First see if there's an existing address recorded.
	err = tx.Query(ctx, selectAddressUUIDStmt, ipAddr).Get(&ipAddr)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("querying existing cloud container address for device %q: %w", deviceUUID, err)
	}

	// Create a UUID for new addresses.
	if errors.Is(err, sqlair.ErrNoRows) {
		addrUUID, err := uuid.NewUUID()
		if err != nil {
			return errors.Capture(err)
		}
		ipAddr.AddressUUID = addrUUID.String()
	}

	// Update the address values.
	if err = tx.Query(ctx, upsertAddressStmt, ipAddr).Run(); err != nil {
		return errors.Errorf("updating cloud container address attributes for device %q: %w", deviceUUID, err)
	}
	return nil
}

func (st *State) upsertCloudContainerPorts(ctx context.Context, tx *sqlair.TX, unitUUID coreunit.UUID, portValues []string) error {
	type ports []string

	ccPort := cloudContainerPort{
		UnitUUID: unitUUID,
	}
	deleteStmt, err := st.Prepare(`
DELETE FROM k8s_pod_port
WHERE port NOT IN ($ports[:])
AND unit_uuid = $cloudContainerPort.unit_uuid;
`, ports{}, ccPort)
	if err != nil {
		return errors.Capture(err)
	}

	upsertStmt, err := sqlair.Prepare(`
INSERT INTO k8s_pod_port (*)
VALUES ($cloudContainerPort.*)
ON CONFLICT(unit_uuid, port)
DO NOTHING
`, ccPort)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, deleteStmt, ports(portValues), ccPort).Run(); err != nil {
		return errors.Errorf("removing cloud container ports for %q: %w", unitUUID, err)
	}

	for _, port := range portValues {
		ccPort.Port = port
		if err := tx.Query(ctx, upsertStmt, ccPort).Run(); err != nil {
			return errors.Errorf("updating cloud container ports for %q: %w", unitUUID, err)
		}
	}

	return nil
}

func (st *State) deleteCloudContainer(ctx context.Context, tx *sqlair.TX, unitUUID coreunit.UUID, netNodeUUID string) error {
	cloudContainer := cloudContainer{UnitUUID: unitUUID}

	if err := st.deleteCloudContainerPorts(ctx, tx, unitUUID); err != nil {
		return errors.Capture(err)
	}

	if err := st.deleteCloudContainerAddresses(ctx, tx, netNodeUUID); err != nil {
		return errors.Capture(err)
	}

	deleteCloudContainerStmt, err := st.Prepare(`
DELETE FROM k8s_pod
WHERE unit_uuid = $cloudContainer.unit_uuid`, cloudContainer)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, deleteCloudContainerStmt, cloudContainer).Run(); err != nil {
		return errors.Capture(err)
	}
	return nil
}

func (st *State) deleteCloudContainerAddresses(ctx context.Context, tx *sqlair.TX, netNodeID string) error {
	unit := minimalUnit{
		NetNodeID: netNodeID,
	}
	deleteAddressStmt, err := st.Prepare(`
DELETE FROM ip_address
WHERE device_uuid IN (
    SELECT device_uuid FROM link_layer_device lld
    WHERE lld.net_node_uuid = $minimalUnit.net_node_uuid
)
`, unit)
	if err != nil {
		return errors.Capture(err)
	}
	deleteDeviceStmt, err := st.Prepare(`
DELETE FROM link_layer_device
WHERE net_node_uuid = $minimalUnit.net_node_uuid`, unit)
	if err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, deleteAddressStmt, unit).Run(); err != nil {
		return errors.Errorf("removing cloud container addresses for %q: %w", netNodeID, err)
	}
	if err := tx.Query(ctx, deleteDeviceStmt, unit).Run(); err != nil {
		return errors.Errorf("removing cloud container link layer devices for %q: %w", netNodeID, err)
	}
	return nil
}

func (st *State) deleteCloudContainerPorts(ctx context.Context, tx *sqlair.TX, unitUUID coreunit.UUID) error {
	cloudContainer := cloudContainer{
		UnitUUID: unitUUID,
	}
	deleteStmt, err := st.Prepare(`
DELETE FROM k8s_pod_port
WHERE unit_uuid = $cloudContainer.unit_uuid`, cloudContainer)
	if err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, deleteStmt, cloudContainer).Run(); err != nil {
		return errors.Errorf("removing cloud container ports for %q: %w", unitUUID, err)
	}
	return nil
}

func (st *State) deletePorts(ctx context.Context, tx *sqlair.TX, unitUUID coreunit.UUID) error {
	unit := minimalUnit{UUID: unitUUID}

	deletePortRange := `
DELETE FROM port_range
WHERE unit_uuid = $minimalUnit.uuid
`
	deletePortRangeStmt, err := st.Prepare(deletePortRange, unit)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, deletePortRangeStmt, unit).Run(); err != nil {
		return errors.Errorf("cannot delete port range records: %w", err)
	}

	return nil
}

func (st *State) deleteConstraints(ctx context.Context, tx *sqlair.TX, unitUUID coreunit.UUID) error {
	unit := minimalUnit{UUID: unitUUID}

	deleteUnitConstraint := `
DELETE FROM unit_constraint
WHERE unit_uuid = $minimalUnit.uuid
`
	deleteUnitConstraintStmt, err := st.Prepare(deleteUnitConstraint, unit)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, deleteUnitConstraintStmt, unit).Run(); err != nil {
		return errors.Errorf("cannot delete unit constraint records: %w", err)
	}
	return nil
}

// SetCloudContainerStatusAtomic saves the given cloud container status, overwriting
// any current status data. If returns an error satisfying
// [applicationerrors.UnitNotFound] if the unit doesn't exist.
func (st *State) setCloudContainerStatus(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID coreunit.UUID,
	sts *status.StatusInfo[status.CloudContainerStatusType],
) error {
	if sts == nil {
		return nil
	}

	statusID, err := status.EncodeCloudContainerStatus(sts.Status)
	if err != nil {
		return errors.Capture(err)
	}

	statusInfo := unitStatusInfo{
		UnitUUID:  unitUUID,
		StatusID:  statusID,
		Message:   sts.Message,
		Data:      sts.Data,
		UpdatedAt: sts.Since,
	}
	stmt, err := st.Prepare(`
INSERT INTO k8s_pod_status (*) VALUES ($unitStatusInfo.*)
ON CONFLICT(unit_uuid) DO UPDATE SET
    status_id = excluded.status_id,
    message = excluded.message,
    updated_at = excluded.updated_at,
    data = excluded.data;
`, statusInfo)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, stmt, statusInfo).Run(); internaldatabase.IsErrConstraintForeignKey(err) {
		return errors.Errorf("%w: %q", applicationerrors.UnitNotFound, unitUUID)
	} else if err != nil {
		return errors.Capture(err)
	}
	return nil
}

func modelExists(ctx context.Context, preparer domain.Preparer, tx *sqlair.TX) error {
	var modelUUID dbUUID
	stmt, err := preparer.Prepare(`SELECT &dbUUID.uuid FROM model;`, modelUUID)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, stmt).Get(&modelUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("model does not exist").Add(modelerrors.NotFound)
	}
	if err != nil {
		return errors.Errorf("checking model if model exists: %w", err)
	}

	return nil
}

// getModelConstraints returns the values set in the constraints table for the
// current model. If no constraints are currently set
// for the model an error satisfying [modelerrors.ConstraintsNotFound] will be
// returned.
func (st *State) getModelConstraints(
	ctx context.Context,
	tx *sqlair.TX,
) (dbConstraint, error) {
	var constraint dbConstraint

	stmt, err := st.Prepare("SELECT &dbConstraint.* FROM v_model_constraint", constraint)
	if err != nil {
		return dbConstraint{}, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt).Get(&constraint)
	if errors.Is(err, sql.ErrNoRows) {
		return dbConstraint{}, errors.New(
			"no constraints set for model",
		).Add(modelerrors.ConstraintsNotFound)
	}
	if err != nil {
		return dbConstraint{}, errors.Errorf("getting model constraints: %w", err)
	}
	return constraint, nil
}

func encodeResolveMode(mode sql.NullInt16) (string, error) {
	if !mode.Valid {
		return "none", nil
	}

	switch mode.Int16 {
	case 0:
		return "retry-hooks", nil
	case 1:
		return "no-hooks", nil
	default:
		return "", errors.Errorf("unknown resolve mode %d", mode.Int16).Add(coreerrors.NotSupported)
	}
}
