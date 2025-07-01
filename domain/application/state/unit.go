// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/ipaddress"
	"github.com/juju/juju/domain/life"
	machinestate "github.com/juju/juju/domain/machine/state"
	modelerrors "github.com/juju/juju/domain/model/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	domainsequence "github.com/juju/juju/domain/sequence"
	sequencestate "github.com/juju/juju/domain/sequence/state"
	"github.com/juju/juju/domain/status"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

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

// InitialWatchStatementUnitAddressesHash returns the initial namespace query
// for the unit addresses hash watcher as well as the tables to be watched
// (ip_address and application_endpoint)
func (st *State) InitialWatchStatementUnitAddressesHash(appUUID coreapplication.ID, netNodeUUID string) (string, string, eventsource.NamespaceQuery) {
	queryFunc := func(ctx context.Context, runner database.TxnRunner) ([]string, error) {

		var (
			spaceAddresses   []spaceAddress
			endpointBindings map[string]network.SpaceUUID
		)
		err := runner.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			var err error
			spaceAddresses, err = st.getNetNodeSpaceAddresses(ctx, tx, netNodeUUID)
			if err != nil {
				return errors.Capture(err)
			}
			endpointBindings, err = st.getEndpointBindings(ctx, tx, appUUID)
			if err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		if err != nil {
			return nil, errors.Errorf("querying application %q addresses hash: %w", appUUID, err)
		}
		hash, err := st.hashAddressesAndEndpoints(spaceAddresses, endpointBindings)
		if err != nil {
			return nil, errors.Capture(err)
		}
		return []string{hash}, nil
	}
	return "ip_address", "application_endpoint", queryFunc
}

// InitialWatchStatementUnitInsertDeleteOnNetNode returns the initial namespace
// query for unit insert and deletes events on a specific net node, as well as
// the watcher namespace to watch.
func (st *State) InitialWatchStatementUnitInsertDeleteOnNetNode(netNodeUUID string) (string, eventsource.NamespaceQuery) {
	return "unit_insert_delete", func(ctx context.Context, runner database.TxnRunner) ([]string, error) {
		var unitNames []coreunit.Name
		err := runner.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			var err error
			unitNames, err = st.getUnitNamesForNetNode(ctx, tx, netNodeUUID)
			return err
		})
		if err != nil {
			return nil, errors.Errorf("querying unit names for net node %q: %w", netNodeUUID, err)
		}
		return transform.Slice(unitNames, func(unitName coreunit.Name) string {
			return unitName.String()
		}), nil
	}
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

// GetAllUnitLifeForApplication returns a map of the unit names and their lives
// for the given application.
//   - If the application is not found, [applicationerrors.ApplicationNotFound]
//     is returned.
func (st *State) GetAllUnitLifeForApplication(ctx context.Context, appID coreapplication.ID) (map[coreunit.Name]life.Life, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	ident := applicationID{ID: appID}
	appExistsQuery := `
SELECT &applicationID.*
FROM application
WHERE uuid = $applicationID.uuid;
`
	appExistsStmt, err := st.Prepare(appExistsQuery, ident)
	if err != nil {
		return nil, errors.Errorf("preparing query for application %q: %w", ident.ID, err)
	}

	lifeQuery := `
SELECT (u.name, u.life_id) AS (&unitDetails.*)
FROM unit u
WHERE u.application_uuid = $applicationID.uuid
`

	app := applicationID{ID: appID}
	lifeStmt, err := st.Prepare(lifeQuery, app, unitDetails{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var lifes []unitDetails
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, appExistsStmt, ident).Get(&ident)
		if errors.Is(err, sql.ErrNoRows) {
			return applicationerrors.ApplicationNotFound
		} else if err != nil {
			return errors.Errorf("checking application %q exists: %w", ident.ID, err)
		}
		err = tx.Query(ctx, lifeStmt, app).GetAll(&lifes)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return errors.Capture(err)
	})
	if err != nil {
		return nil, errors.Errorf("querying unit life for %q: %w", appID, err)
	}
	result := make(map[coreunit.Name]life.Life)
	for _, u := range lifes {
		result[u.Name] = u.LifeID
	}
	return result, nil
}

// GetUnitMachineName gets the unit's machine name.
//
// The following errors may be returned:
//   - [applicationerrors.UnitMachineNotAssigned] if the unit does not have a
//     machine assigned.
//   - [applicationerrors.UnitNotFound] if the unit cannot be found.
//   - [applicationerrors.UnitIsDead] if the unit is dead.
func (st *State) GetUnitMachineName(ctx context.Context, unitName coreunit.Name) (coremachine.Name, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}
	arg := getUnitMachineName{
		UnitName: unitName,
	}
	stmt, err := st.Prepare(`
SELECT (m.name) AS (&getUnitMachineName.*)
FROM   unit AS u
JOIN   machine AS m ON u.net_node_uuid = m.net_node_uuid
WHERE  u.name = $getUnitMachineName.unit_name
`, arg)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkUnitNotDeadByName(ctx, tx, unitName); err != nil {
			return errors.Capture(err)
		}

		err = tx.Query(ctx, stmt, arg).Get(&arg)
		if errors.Is(err, sqlair.ErrNoRows) {
			return applicationerrors.UnitMachineNotAssigned
		}
		return errors.Capture(err)
	})
	if err != nil {
		return "", errors.Capture(err)
	}

	return arg.MachineName, nil
}

// GetUnitMachineUUID gets the unit's machine UUID.
//
// The following errors may be returned:
//   - [applicationerrors.UnitMachineNotAssigned] if the unit does not have a
//     machine assigned.
//   - [applicationerrors.UnitNotFound] if the unit cannot be found.
//   - [applicationerrors.UnitIsDead] if the unit is dead.
func (st *State) GetUnitMachineUUID(ctx context.Context, unitName coreunit.Name) (coremachine.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}
	arg := getUnitMachineUUID{
		UnitName: unitName,
	}
	stmt, err := st.Prepare(`
SELECT (m.uuid) AS (&getUnitMachineUUID.*)
FROM   unit AS u
JOIN   machine AS m ON u.net_node_uuid = m.net_node_uuid
WHERE  u.name = $getUnitMachineUUID.unit_name
`, arg)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkUnitNotDeadByName(ctx, tx, unitName); err != nil {
			return errors.Capture(err)
		}

		err = tx.Query(ctx, stmt, arg).Get(&arg)
		if errors.Is(err, sqlair.ErrNoRows) {
			return applicationerrors.UnitMachineNotAssigned
		}
		return errors.Capture(err)
	})
	if err != nil {
		return "", errors.Capture(err)
	}

	return arg.MachineUUID, nil
}

// AddIAASUnits adds the specified units to the application. Returns the unit
// names, along with all of the machine names that were created for the
// units. This machines aren't associated with the units, as this is for a
// recording artifact.
//   - If any of the units already exists [applicationerrors.UnitAlreadyExists]
//     is returned.
//   - If the application is not alive, [applicationerrors.ApplicationNotAlive]
//     is returned.
//   - If the application is not found, [applicationerrors.ApplicationNotFound]
//     is returned.
func (st *State) AddIAASUnits(
	ctx context.Context, appUUID coreapplication.ID, args ...application.AddIAASUnitArg,
) ([]coreunit.Name, []coremachine.Name, error) {
	if len(args) == 0 {
		return nil, nil, nil
	}

	db, err := st.DB()
	if err != nil {
		return nil, nil, errors.Capture(err)
	}

	var (
		unitNames    []coreunit.Name
		machineNames []coremachine.Name
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkApplicationAlive(ctx, tx, appUUID); err != nil {
			return errors.Capture(err)
		}

		// TODO(storage) - read and use storage directives
		for _, arg := range args {
			unitName, err := st.newUnitName(ctx, tx, appUUID)
			if err != nil {
				return errors.Errorf("getting new unit name for application %q: %w", appUUID, err)
			}
			unitNames = append(unitNames, unitName)

			insertArg := application.InsertIAASUnitArg{
				InsertUnitArg: application.InsertUnitArg{
					UnitName:    unitName,
					Constraints: arg.Constraints,
					Placement:   arg.Placement,
					UnitStatusArg: application.UnitStatusArg{
						AgentStatus:    arg.UnitStatusArg.AgentStatus,
						WorkloadStatus: arg.UnitStatusArg.WorkloadStatus,
					},
				},
				Platform: arg.Platform,
				Nonce:    arg.Nonce,
			}

			mNames, err := st.insertIAASUnit(ctx, tx, appUUID, insertArg)
			if err != nil {
				return errors.Errorf("inserting unit %q: %w ", unitName, err)
			}
			machineNames = append(machineNames, mNames...)
		}
		return nil
	})
	return unitNames, machineNames, errors.Capture(err)
}

// AddCAASUnits adds the specified units to the application.
//   - If any of the units already exists [applicationerrors.UnitAlreadyExists] is returned.
//   - If the application is not alive, [applicationerrors.ApplicationNotAlive] is returned.
//   - If the application is not found, [applicationerrors.ApplicationNotFound] is returned.
func (st *State) AddCAASUnits(
	ctx context.Context, appUUID coreapplication.ID, args ...application.AddUnitArg,
) ([]coreunit.Name, error) {
	if len(args) == 0 {
		return nil, nil
	}

	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	var unitNames []coreunit.Name
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// TODO(storage) - read and use storage directives
		for _, arg := range args {
			unitName, err := st.newUnitName(ctx, tx, appUUID)
			if err != nil {
				return errors.Errorf("getting new unit name for application %q: %w", appUUID, err)
			}
			unitNames = append(unitNames, unitName)

			insertArg := application.InsertUnitArg{
				UnitName:    unitName,
				Constraints: arg.Constraints,
				Placement:   arg.Placement,
				UnitStatusArg: application.UnitStatusArg{
					AgentStatus:    arg.UnitStatusArg.AgentStatus,
					WorkloadStatus: arg.UnitStatusArg.WorkloadStatus,
				},
			}
			if err = st.insertCAASUnit(ctx, tx, appUUID, insertArg); err != nil {
				return errors.Errorf("inserting unit %q: %w ", unitName, err)
			}
		}
		return nil
	})
	return unitNames, errors.Capture(err)
}

// AddIAASSubordinateUnit adds a unit to the specified subordinate application
// to the IAAS application on the same machine as the given principal unit and
// records the principal-subordinate relationship.
//
// The following error types can be expected:
//   - [applicationerrors.ApplicationNotFound] when the subordinate application
//     cannot be found.
//   - [applicationerrors.UnitNotFound] when the unit cannot be found.
func (st *State) AddIAASSubordinateUnit(
	ctx context.Context,
	arg application.SubordinateUnitArg,
) (coreunit.Name, []coremachine.Name, error) {
	db, err := st.DB()
	if err != nil {
		return "", nil, errors.Capture(err)
	}

	var (
		unitName     coreunit.Name
		machineNames []coremachine.Name
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Check the application is alive.
		if err := st.checkApplicationAlive(ctx, tx, arg.SubordinateAppID); err != nil {
			return errors.Capture(err)
		}
		if err := st.checkUnitNotDeadByName(ctx, tx, arg.PrincipalUnitName); err != nil {
			return errors.Capture(err)
		}

		// Check this unit does not already have a subordinate unit from this
		// application.
		if err := st.checkNoSubordinateExists(ctx, tx, arg.SubordinateAppID, arg.PrincipalUnitName); err != nil {
			return errors.Errorf("checking if subordinate already exists: %w", err)
		}

		// Generate a new unit name.
		unitName, err = st.newUnitName(ctx, tx, arg.SubordinateAppID)
		if err != nil {
			return errors.Errorf("getting new unit name for application %q: %w", arg.SubordinateAppID, err)
		}

		// Insert the new unit.
		// TODO(storage) - read and use storage directives
		insertArg := application.InsertIAASUnitArg{
			InsertUnitArg: application.InsertUnitArg{
				UnitName:      unitName,
				UnitStatusArg: arg.UnitStatusArg,
			},
		}
		// Place the subordinate on the same machine as the principal unit.
		machineName, err := st.getUnitMachineName(ctx, tx, arg.PrincipalUnitName)
		if err != nil {
			return errors.Errorf("getting unit machine name: %w", err)
		}
		insertArg.Placement = deployment.Placement{
			Type:      deployment.PlacementTypeMachine,
			Directive: machineName.String(),
		}

		if machineNames, err = st.insertIAASUnit(ctx, tx, arg.SubordinateAppID, insertArg); err != nil {
			return errors.Errorf("inserting subordinate unit %q: %w", unitName, err)
		}

		// Record the principal/subordinate relationship.
		if err := st.recordUnitPrincipal(ctx, tx, arg.PrincipalUnitName, unitName); err != nil {
			return errors.Errorf("recording principal-subordinate relationship: %w", err)
		}

		return nil
	})
	if err != nil {
		return "", nil, errors.Capture(err)
	}

	return unitName, machineNames, nil
}

// GetUnitPrincipal gets the subordinates principal unit. If no principal unit
// is found, for example, when the unit is not a subordinate, then false is
// returned.
func (st *State) GetUnitPrincipal(
	ctx context.Context,
	unitName coreunit.Name,
) (coreunit.Name, bool, error) {
	db, err := st.DB()
	if err != nil {
		return "", false, errors.Capture(err)
	}

	arg := getPrincipal{
		SubordinateUnitName: unitName,
	}

	stmt, err := st.Prepare(`
SELECT principal.name AS &getPrincipal.principal_unit_name
FROM   unit AS principal
JOIN   unit_principal AS up ON principal.uuid = up.principal_uuid
JOIN   unit AS sub ON up.unit_uuid = sub.uuid
WHERE  sub.name = $getPrincipal.subordinate_unit_name
`, arg)
	if err != nil {
		return "", false, errors.Capture(err)
	}

	ok := true
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, arg).Get(&arg)
		if errors.Is(err, sqlair.ErrNoRows) {
			ok = false
			return nil
		}
		return err
	})
	return arg.PrincipalUnitName, ok, err
}

// checkNoSubordinateExists returns
// [applicationerrors.UnitAlreadyHasSubordinate] if the specified unit already
// has a subordinate for the given application.
func (st *State) checkNoSubordinateExists(
	ctx context.Context,
	tx *sqlair.TX,
	subordinateAppID coreapplication.ID,
	unitName coreunit.Name,
) error {
	type getSubordinate struct {
		PrincipalUnitName coreunit.Name      `db:"principal_unit_name"`
		ApplicationID     coreapplication.ID `db:"application_uuid"`
	}
	subordinate := getSubordinate{
		PrincipalUnitName: unitName,
		ApplicationID:     subordinateAppID,
	}

	stmt, err := st.Prepare(`
SELECT u1.name AS &getSubordinate.principal_unit_name
FROM   unit u1
JOIN   unit_principal up ON up.principal_uuid = u1.uuid
JOIN   unit u2 ON u2.uuid = up.unit_uuid
WHERE  u1.name = $getSubordinate.principal_unit_name
AND    u2.application_uuid = $getSubordinate.application_uuid
`, subordinate)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, subordinate).Get(&subordinate)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil
	} else if err != nil {
		return errors.Capture(err)
	}

	return applicationerrors.UnitAlreadyHasSubordinate
}

// IsSubordinateApplication returns true if the application is a subordinate
// application.
// The following errors may be returned:
// - [appliationerrors.ApplicationNotFound] if the application does not exist
func (st *State) IsSubordinateApplication(
	ctx context.Context,
	applicationUUID coreapplication.ID,
) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Capture(err)
	}

	type getSubordinate struct {
		ApplicationUUID coreapplication.ID `db:"application_uuid"`
		Subordinate     bool               `db:"subordinate"`
	}
	subordinate := getSubordinate{
		ApplicationUUID: applicationUUID,
	}

	stmt, err := st.Prepare(`
SELECT subordinate AS &getSubordinate.subordinate
FROM   charm_metadata cm
JOIN   charm c ON c.uuid = cm.charm_uuid
JOIN   application a ON a.charm_uuid = c.uuid
WHERE  a.uuid = $getSubordinate.application_uuid
`, subordinate)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, subordinate).Get(&subordinate)
		if errors.Is(err, sqlair.ErrNoRows) {
			return applicationerrors.ApplicationNotFound
		}
		return err
	})
	if err != nil {
		return false, errors.Capture(err)
	}

	return subordinate.Subordinate, nil
}

// GetUnitSubordinates returns the names of all the subordinate units of the
// given principal unit.
//
// If the principal unit cannot be found, [applicationerrors.UnitNotFound] is
// returned.
func (st *State) GetUnitSubordinates(ctx context.Context, unitName coreunit.Name) ([]coreunit.Name, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	type subName struct {
		Name coreunit.Name `db:"name"`
	}
	type principalName struct {
		Name coreunit.Name `db:"name"`
	}
	pName := principalName{Name: unitName}
	stmt, err := st.Prepare(`
SELECT sub.name AS &subName.*
FROM   unit AS sub
JOIN   unit_principal AS up ON sub.uuid = up.unit_uuid
JOIN   unit AS principal ON up.principal_uuid = principal.uuid
WHERE  principal.name = $principalName.name
`, pName, subName{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var subNames []subName
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkUnitExistsByName(ctx, tx, unitName); err != nil {
			return errors.Errorf("checking unit exists: %w", err)
		}

		err := tx.Query(ctx, stmt, pName).GetAll(&subNames)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("getting unit subordinates: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return transform.Slice(subNames, func(s subName) coreunit.Name { return s.Name }), nil

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

	var uuid coreunit.UUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		uuid, err = st.getUnitUUIDByName(ctx, tx, name)
		if err != nil {
			return errors.Errorf("getting unit UUID by name %q: %w", name, err)
		}
		return nil
	})
	if err != nil {
		return "", errors.Capture(err)
	}

	return uuid, nil
}

func (st *State) getUnitUUIDByName(
	ctx context.Context,
	tx *sqlair.TX,
	name coreunit.Name,
) (coreunit.UUID, error) {
	unitName := unitName{Name: name}

	query, err := st.Prepare(`
SELECT &unitUUID.*
FROM   unit
WHERE  name = $unitName.name
`, unitUUID{}, unitName)
	if err != nil {
		return "", errors.Errorf("preparing query: %w", err)
	}

	unitUUID := unitUUID{}
	err = tx.Query(ctx, query, unitName).Get(&unitUUID)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("unit %q not found", name).Add(applicationerrors.UnitNotFound)
	}
	return unitUUID.UnitUUID, errors.Capture(err)
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
				DeviceTypeID:      domainnetwork.DeviceTypeUnknown,
				VirtualPortTypeID: domainnetwork.NonVirtualPortType,
			},
			Value:       cloudContainer.Address.Value,
			AddressType: ipaddress.MarshallAddressType(cloudContainer.Address.AddressType()),
			// The k8s container must have the lowest scope. This is needed to
			// ensure that these are correctly matched with respect to k8s
			// service addresses when retrieving unit public/private addresses.
			Scope:      ipaddress.MarshallScope(network.ScopeMachineLocal),
			Origin:     ipaddress.MarshallOrigin(network.OriginProvider),
			ConfigType: ipaddress.MarshallConfigType(network.ConfigDHCP),
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

			if appScale.Scaling {
				// While scaling, we use the scaling target.
				if arg.OrderedId >= appScale.ScaleTarget {
					return errors.Errorf("unrequired unit %s is not assigned", arg.UnitName).Add(applicationerrors.UnitNotAssigned)
				}
			} else {
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
	_, err := st.getUnitDetails(ctx, tx, args.UnitName)
	if err == nil {
		return errors.Errorf("unit %q already exists", args.UnitName).Add(applicationerrors.UnitAlreadyExists)
	} else if !errors.Is(err, applicationerrors.UnitNotFound) {
		return errors.Errorf("looking up unit %q: %w", args.UnitName, err)
	}

	unitUUID, err := coreunit.NewUUID()
	if err != nil {
		return errors.Capture(err)
	}

	netNodeUUID, err := st.insertNetNode(ctx, tx)
	if err != nil {
		return errors.Capture(err)
	}

	if err := st.insertUnit(ctx, tx, appUUID, unitUUID, netNodeUUID, insertUnitArg{
		UnitName:       args.UnitName,
		CloudContainer: args.CloudContainer,
		Password:       args.Password,
		Constraints:    args.Constraints,
		UnitStatusArg:  args.UnitStatusArg,
	}); err != nil {
		return errors.Errorf("inserting unit for CAAS application %q: %w", appUUID, err)
	}

	// If there is no storage, return early.
	if len(args.Storage) == 0 {
		return nil
	}

	attachArgs, err := st.insertUnitStorage(ctx, tx, appUUID, unitUUID, args.Storage, args.StoragePoolKind)
	if err != nil {
		return errors.Errorf("creating storage for unit %q: %w", args.UnitName, err)
	}
	err = st.attachUnitStorage(ctx, tx, args.StoragePoolKind, unitUUID, netNodeUUID, attachArgs)
	if err != nil {
		return errors.Errorf("attaching storage for unit %q: %w", args.UnitName, err)
	}
	return nil
}

func (st *State) insertIAASUnit(
	ctx context.Context,
	tx *sqlair.TX,
	appUUID coreapplication.ID,
	args application.InsertIAASUnitArg,
) ([]coremachine.Name, error) {
	_, err := st.getUnitDetails(ctx, tx, args.UnitName)
	if err == nil {
		return nil, errors.Errorf("unit %q already exists", args.UnitName).Add(applicationerrors.UnitAlreadyExists)
	} else if !errors.Is(err, applicationerrors.UnitNotFound) {
		return nil, errors.Errorf("looking up unit %q: %w", args.UnitName, err)
	}

	unitUUID, err := coreunit.NewUUID()
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Handle the placement of the net node and machines accompanying the unit.
	nodeUUID, machineNames, err := machinestate.PlaceMachine(ctx, tx, st, args.Placement, args.Platform, args.Nonce, st.clock)
	if err != nil {
		return nil, errors.Errorf("getting net node UUID from placement %+v: %w", args.Placement, err)
	}

	if err := st.insertUnit(ctx, tx, appUUID, unitUUID, nodeUUID, insertUnitArg{
		UnitName:       args.UnitName,
		CloudContainer: args.CloudContainer,
		Password:       args.Password,
		Constraints:    args.Constraints,
		UnitStatusArg:  args.UnitStatusArg,
	}); err != nil {
		return nil, errors.Errorf("inserting unit for application %q: %w", appUUID, err)
	}
	if _, err := st.insertUnitStorage(ctx, tx, appUUID, unitUUID, args.Storage, args.StoragePoolKind); err != nil {
		return nil, errors.Errorf("creating storage for unit %q: %w", args.UnitName, err)
	}
	return machineNames, nil
}

type insertUnitArg struct {
	UnitName       coreunit.Name
	CloudContainer *application.CloudContainer
	Password       *application.PasswordInfo
	Constraints    constraints.Constraints
	application.UnitStatusArg
}

func (st *State) insertUnit(
	ctx context.Context, tx *sqlair.TX,
	appUUID coreapplication.ID,
	unitUUID coreunit.UUID,
	netNodeUUID string,
	args insertUnitArg,
) error {
	if err := st.checkApplicationAlive(ctx, tx, appUUID); err != nil {
		return errors.Capture(err)
	}

	charmUUID, err := st.getCharmIDByApplicationID(ctx, tx, appUUID)
	if err != nil {
		return errors.Errorf("getting charm for application %q: %w", appUUID, err)
	}

	createParams := unitDetails{
		ApplicationID: appUUID,
		UnitUUID:      unitUUID,
		CharmUUID:     charmUUID,
		Name:          args.UnitName,
		NetNodeID:     netNodeUUID,
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
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, createUnitStmt, createParams).Run(); err != nil {
		return errors.Errorf("creating unit for unit %q: %w", args.UnitName, err)
	}
	if args.CloudContainer != nil {
		if err := st.upsertUnitCloudContainer(ctx, tx, args.UnitName, unitUUID, netNodeUUID, args.CloudContainer); err != nil {
			return errors.Errorf("creating cloud container for unit %q: %w", args.UnitName, err)
		}
	}
	if err := st.setUnitConstraints(ctx, tx, unitUUID, args.Constraints); err != nil {
		return errors.Errorf("setting constraints for unit %q: %w", args.UnitName, err)
	}
	if err := st.setUnitAgentStatus(ctx, tx, unitUUID, args.AgentStatus); err != nil {
		return errors.Errorf("setting agent status for unit %q: %w", args.UnitName, err)
	}
	if err := st.setUnitWorkloadStatus(ctx, tx, unitUUID, args.WorkloadStatus); err != nil {
		return errors.Errorf("setting workload status for unit %q: %w", args.UnitName, err)
	}
	if err := st.setUnitWorkloadVersion(ctx, tx, args.UnitName, ""); err != nil {
		return errors.Errorf("setting workload version for unit %q: %w", args.UnitName, err)
	}
	return nil
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
		if err := st.setK8sPodStatus(ctx, tx, toUpdate.UnitUUID, params.K8sPodStatus); err != nil {
			return errors.Errorf("saving unit %q k8s pod status: %w", unitName, err)
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

// GetAllUnitNames returns a slice of all unit names in the model.
func (st *State) GetAllUnitNames(ctx context.Context) ([]coreunit.Name, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	query := `SELECT &unitName.* FROM unit`
	stmt, err := st.Prepare(query, unitName{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var result []unitName
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&result)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return transform.Slice(result, func(r unitName) coreunit.Name {
		return r.Name
	}), nil
}

// GetUnitNamesForApplication returns a slice of the unit names for the given application
// The following errors may be returned:
// - [applicationerrors.ApplicationIsDead] if the application is dead
// - [applicationerrors.ApplicationNotFound] if the application does not exist
func (st *State) GetUnitNamesForApplication(ctx context.Context, uuid coreapplication.ID) ([]coreunit.Name, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	appUUID := applicationID{ID: uuid}
	query := ` SELECT &unitName.* FROM unit WHERE application_uuid = $applicationID.uuid`
	stmt, err := st.Prepare(query, unitName{}, appUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var result []unitName
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.checkApplicationNotDead(ctx, tx, uuid)
		if err != nil {
			return errors.Capture(err)
		}
		err = tx.Query(ctx, stmt, appUUID).GetAll(&result)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return transform.Slice(result, func(r unitName) coreunit.Name {
		return r.Name
	}), nil
}

// GetUnitNamesForNetNode returns a slice of the unit names for the given net node
// The following errors may be returned:
func (st *State) GetUnitNamesForNetNode(ctx context.Context, uuid string) ([]coreunit.Name, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	var unitNames []coreunit.Name
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		unitNames, err = st.getUnitNamesForNetNode(ctx, tx, uuid)
		return err
	})
	if err != nil {
		return nil, errors.Errorf("querying unit names for net node %q: %w", uuid, err)
	}
	return unitNames, nil
}

func (st *State) getUnitNamesForNetNode(ctx context.Context, tx *sqlair.TX, uuid string) ([]coreunit.Name, error) {
	netNodeUUID := netNodeUUID{NetNodeUUID: uuid}
	verifyExistsQuery := `SELECT COUNT(*) AS &countResult.count FROM net_node WHERE uuid = $netNodeUUID.uuid`
	verifyExistsStmt, err := st.Prepare(verifyExistsQuery, countResult{}, netNodeUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	query := `SELECT &unitName.* FROM unit WHERE net_node_uuid = $netNodeUUID.uuid`
	stmt, err := st.Prepare(query, unitName{}, netNodeUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var count countResult
	if err := tx.Query(ctx, verifyExistsStmt, netNodeUUID).Get(&count); err != nil {
		return nil, errors.Capture(err)
	}
	if count.Count == 0 {
		return nil, applicationerrors.NetNodeNotFound
	}

	var result []unitName
	err = tx.Query(ctx, stmt, netNodeUUID).GetAll(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Capture(err)
	}

	return transform.Slice(result, func(r unitName) coreunit.Name {
		return r.Name
	}), nil
}

// SetUnitWorkloadVersion sets the workload version for the given unit.
func (st *State) SetUnitWorkloadVersion(ctx context.Context, unitName coreunit.Name, version string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.setUnitWorkloadVersion(ctx, tx, unitName, version)
	})
	if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// setUnitWorkloadVersion workload version sets the denormalized workload
// version on both the unit and the application. These are on separate tables,
// so we need to do two separate queries. This prevents the workload version
// from trigging a cascade of unwanted updates to the application and or unit
// tables.
func (st *State) setUnitWorkloadVersion(
	ctx context.Context,
	tx *sqlair.TX,
	unitName coreunit.Name,
	version string,
) error {
	unitQuery, err := st.Prepare(`
INSERT INTO unit_workload_version (*)
VALUES ($unitWorkloadVersion.*)
ON CONFLICT (unit_uuid) DO UPDATE SET
    version = excluded.version;
`, unitWorkloadVersion{})
	if err != nil {
		return errors.Capture(err)
	}

	appQuery, err := st.Prepare(`
INSERT INTO application_workload_version (*)
VALUES ($applicationWorkloadVersion.*)
ON CONFLICT (application_uuid) DO UPDATE SET
	version = excluded.version;
`, applicationWorkloadVersion{})
	if err != nil {
		return errors.Capture(err)
	}

	details, err := st.getUnitDetails(ctx, tx, unitName)
	if err != nil {
		return errors.Errorf("getting unit uuid for %q: %w", unitName, err)
	}

	if err := tx.Query(ctx, unitQuery, unitWorkloadVersion{
		UnitUUID: details.UnitUUID,
		Version:  version,
	}).Run(); err != nil {
		return errors.Errorf("setting workload version for unit %q: %w", unitName, err)
	}

	if err := tx.Query(ctx, appQuery, applicationWorkloadVersion{
		ApplicationUUID: details.ApplicationID,
		Version:         version,
	}).Run(); err != nil {
		return errors.Errorf("setting workload version for application %q: %w", details.ApplicationID, err)
	}
	return nil
}

// GetWorkloadVersion returns the workload version for the given unit.
func (st *State) GetUnitWorkloadVersion(ctx context.Context, unitName coreunit.Name) (string, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	query, err := st.Prepare(`
SELECT &unitWorkloadVersion.version
FROM   unit_workload_version
WHERE  unit_uuid = $unitWorkloadVersion.unit_uuid
	`, unitWorkloadVersion{})
	if err != nil {
		return "", errors.Capture(err)
	}

	var version unitWorkloadVersion
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		unitUUID, err := st.getUnitUUIDByName(ctx, tx, unitName)
		if err != nil {
			return errors.Errorf("getting unit uuid for %q: %w", unitName, err)
		}

		if err := tx.Query(ctx, query, unitWorkloadVersion{
			UnitUUID: unitUUID,
		}).Get(&version); errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("getting workload version for %q: %w", unitName, err)
		}
		return nil
	})
	if err != nil {
		return "", errors.Capture(err)
	}
	return version.Version, nil
}

// newUnitName returns a new name for the unit. It increments the unit counter
// on the application.
func (st *State) newUnitName(
	ctx context.Context,
	tx *sqlair.TX,
	appID coreapplication.ID,
) (coreunit.Name, error) {

	var nextUnitNum uint64
	appName, err := st.getApplicationName(ctx, tx, appID)
	if err != nil {
		return "", errors.Capture(err)
	}

	namespace := domainsequence.MakePrefixNamespace(application.ApplicationSequenceNamespace, appName)
	nextUnitNum, err = sequencestate.NextValue(ctx, st, tx, namespace)
	if err != nil {
		return "", errors.Errorf("getting next unit number: %w", err)
	}

	return coreunit.NewNameFromParts(appName, int(nextUnitNum))
}

// recordUnitPrincipal records a subordinate-principal relationship between
// units.
func (st *State) recordUnitPrincipal(
	ctx context.Context,
	tx *sqlair.TX,
	principalUnitName, subordinateUnitName coreunit.Name,
) error {
	type unitPrincipal struct {
		PrincipalUUID   coreunit.UUID `db:"principal_uuid"`
		SubordinateUUID coreunit.UUID `db:"unit_uuid"`
	}
	arg := unitPrincipal{}
	stmt, err := st.Prepare(`
INSERT INTO unit_principal (*)
VALUES ($unitPrincipal.*)
`, arg)
	if err != nil {
		return errors.Capture(err)
	}

	arg.PrincipalUUID, err = st.getUnitUUIDByName(ctx, tx, principalUnitName)
	if err != nil {
		return errors.Errorf("getting principal unit uuid: %w", err)
	}

	arg.SubordinateUUID, err = st.getUnitUUIDByName(ctx, tx, subordinateUnitName)
	if err != nil {
		return errors.Errorf("getting principal unit uuid: %w", err)
	}

	err = tx.Query(ctx, stmt, arg).Run()
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// getUnitMachineName returns the name of the machine the unit is running on. If
// the unit is not associated with a machine, the name returned is empty.
func (st *State) getUnitMachineName(
	ctx context.Context,
	tx *sqlair.TX,
	unitName coreunit.Name,
) (coremachine.Name, error) {
	arg := getUnitMachine{
		UnitName: unitName,
	}
	stmt, err := st.Prepare(`
SELECT m.name AS &getUnitMachine.machine_name
FROM   unit u
JOIN   machine m ON u.net_node_uuid = m.net_node_uuid
WHERE  u.name = $getUnitMachine.unit_name
`, arg)
	if err != nil {
		return "", errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, arg).Get(&arg)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", applicationerrors.MachineNotFound
	} else if err != nil {
		return "", errors.Capture(err)
	}

	return arg.UnitMachine, nil
}

// GetUnitK8sPodInfo returns information about the k8s pod for the given unit.
// If any of the requested pieces of data are not present yet, zero values will
// be returned in their place.
// The following errors may be returned:
// - [applicationerrors.UnitNotFound] if the unit does not exist
// - [applicationerrors.UnitIsDead] if the unit is dead
func (st *State) GetUnitK8sPodInfo(ctx context.Context, name coreunit.Name) (application.K8sPodInfo, error) {
	db, err := st.DB()
	if err != nil {
		return application.K8sPodInfo{}, errors.Capture(err)
	}

	unitName := unitName{Name: name}
	infoQuery := `
SELECT    k.provider_id AS &unitK8sPodInfo.provider_id,
          ip.address_value AS &unitK8sPodInfo.address
FROM      unit AS u
LEFT JOIN k8s_pod AS k ON u.uuid = k.unit_uuid
LEFT JOIN link_layer_device lld ON lld.net_node_uuid = u.net_node_uuid
LEFT JOIN ip_address ip ON ip.device_uuid = lld.uuid
WHERE     u.name = $unitName.name`
	infoStmt, err := st.Prepare(infoQuery, unitK8sPodInfo{}, unitName)
	if err != nil {
		return application.K8sPodInfo{}, errors.Capture(err)
	}

	portsQuery := `
SELECT &k8sPodPort.*
FROM   unit AS u
JOIN   k8s_pod_port AS kp ON kp.unit_uuid = u.uuid
WHERE  u.name = $unitName.name`
	portsStmt, err := st.Prepare(portsQuery, k8sPodPort{}, unitName)
	if err != nil {
		return application.K8sPodInfo{}, errors.Capture(err)
	}

	var info unitK8sPodInfo
	var ports []k8sPodPort
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkUnitNotDeadByName(ctx, tx, name); err != nil {
			return errors.Capture(err)
		}

		err := tx.Query(ctx, infoStmt, unitName).Get(&info)
		if err != nil {
			return errors.Capture(err)
		}

		err = tx.Query(ctx, portsStmt, unitName).GetAll(&ports)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}

		return nil
	})
	if err != nil {
		return application.K8sPodInfo{}, errors.Capture(err)
	}
	return encodeK8sPodInfo(info, ports), nil
}

// GetUnitNetNodesByName returns the net node UUIDs associated with the
// specified unit. The net nodes are selected in the same way as in
// GetUnitAddresses, i.e. the union of the net nodes of the cloud service (if
// any) and the net node of the unit.
//
// The following errors may be returned:
// - [uniterrors.UnitNotFound] if the unit does not exist
func (st *State) GetUnitNetNodesByName(ctx context.Context, name coreunit.Name) ([]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	ident := unitName{Name: name}
	stmt, err := st.Prepare(`
SELECT &unitNetNodeUUID.*
FROM (
    SELECT s.net_node_uuid, u.name
    FROM unit u
    JOIN k8s_service s on s.application_uuid = u.application_uuid
    UNION
    SELECT net_node_uuid, name FROM unit
) AS n
WHERE n.name = $unitName.name
`, unitNetNodeUUID{}, ident)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var netNodeUUIDs []unitNetNodeUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, ident).GetAll(&netNodeUUIDs)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("%w: %s", applicationerrors.UnitNotFound, name)
		}
		return errors.Capture(err)
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	netNodeUUIDstrs := make([]string, len(netNodeUUIDs))
	for i, n := range netNodeUUIDs {
		netNodeUUIDstrs[i] = n.NetNodeUUID
	}

	return netNodeUUIDstrs, nil
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
	ctx context.Context,
	tx *sqlair.TX,
	unitName coreunit.Name,
	unitUUID coreunit.UUID,
	netNodeUUID string,
	cc *application.CloudContainer,
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
	ctx context.Context, tx *sqlair.TX, unitName coreunit.Name, netNodeUUID string, address application.ContainerAddress,
) error {
	// First ensure the address link layer device is upserted.
	// For cloud containers, the device is a placeholder without
	// a MAC address. It just exits to tie the address to the
	// net node corresponding to the cloud container.
	cloudContainerDeviceInfo := cloudContainerDevice{
		Name:              address.Device.Name,
		NetNodeID:         netNodeUUID,
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

	subnetUUIDs, err := st.k8sSubnetUUIDsByAddressType(ctx, tx)
	if err != nil {
		return errors.Capture(err)
	}
	subnetUUID, ok := subnetUUIDs[ipaddress.UnMarshallAddressType(address.AddressType)]
	if !ok {
		// Note: This is a programming error. Today the K8S subnets are
		// placeholders which should always be created when a model is
		// added.
		return errors.Errorf("subnet for address type %q not found", address.AddressType)
	}

	// Now process the address details.
	ipAddr := ipAddress{
		Value:        address.Value,
		SubnetUUID:   subnetUUID,
		NetNodeUUID:  netNodeUUID,
		ConfigTypeID: int(address.ConfigType),
		TypeID:       int(address.AddressType),
		OriginID:     int(address.Origin),
		ScopeID:      int(address.Scope),
		DeviceID:     deviceUUID,
	}

	selectAddressUUIDStmt, err := st.Prepare(`
SELECT &ipAddress.uuid
FROM   ip_address
WHERE  device_uuid = $ipAddress.device_uuid;
`, ipAddr)
	if err != nil {
		return errors.Capture(err)
	}

	upsertAddressStmt, err := sqlair.Prepare(`
INSERT INTO ip_address (*)
VALUES ($ipAddress.*)
ON CONFLICT(uuid) DO UPDATE SET
    address_value = excluded.address_value,
    subnet_uuid = excluded.subnet_uuid,
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

	ccPort := unitK8sPodPort{
		UnitUUID: unitUUID,
	}
	deleteStmt, err := st.Prepare(`
DELETE FROM k8s_pod_port
WHERE port NOT IN ($ports[:])
AND unit_uuid = $unitK8sPodPort.unit_uuid;
`, ports{}, ccPort)
	if err != nil {
		return errors.Capture(err)
	}

	upsertStmt, err := sqlair.Prepare(`
INSERT INTO k8s_pod_port (*)
VALUES ($unitK8sPodPort.*)
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

// setK8sPodStatus saves the given k8s pod status, overwriting
// any current status data. If returns an error satisfying
// [applicationerrors.UnitNotFound] if the unit doesn't exist.
func (st *State) setK8sPodStatus(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID coreunit.UUID,
	sts *status.StatusInfo[status.K8sPodStatusType],
) error {
	if sts == nil {
		return nil
	}

	statusID, err := status.EncodeK8sPodStatus(sts.Status)
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

func encodeK8sPodInfo(info unitK8sPodInfo, ports []k8sPodPort) application.K8sPodInfo {
	ret := application.K8sPodInfo{}
	if info.ProviderID.Valid {
		ret.ProviderID = info.ProviderID.V
	}
	if info.Address.Valid {
		ret.Address = info.Address.V
	}
	ret.Ports = make([]string, len(ports))
	for i, p := range ports {
		ret.Ports[i] = p.Port
	}
	return ret
}
