// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	corestatus "github.com/juju/juju/core/status"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	domainapplicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/internal"
	applicationinternal "github.com/juju/juju/domain/application/internal"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/ipaddress"
	"github.com/juju/juju/domain/life"
	domainlife "github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/port"
	"github.com/juju/juju/domain/status"
	domainstorage "github.com/juju/juju/domain/storage"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// checkUnitExists checks if the unit with the given UUID exists in the model.
// True is returned when the unit is found.
func (st *State) checkUnitExists(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID string,
) (bool, error) {
	uuidInput := entityUUID{UUID: unitUUID}

	checkStmt, err := st.Prepare(`
SELECT &entityUUID.*
FROM   unit
WHERE  uuid = $entityUUID.uuid
	`,
		uuidInput,
	)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, checkStmt, uuidInput).Get(&uuidInput)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}
	return true, nil
}

func (st *State) getUnitLifeAndNetNode(ctx context.Context, tx *sqlair.TX, uuid string) (life.Life, string, error) {
	unitUUID := unitUUID{UnitUUID: uuid}
	queryUnit := `
SELECT &unitLifeAndNetNode.*
FROM unit
WHERE uuid = $unitUUID.uuid
`
	queryUnitStmt, err := st.Prepare(queryUnit, unitUUID, unitLifeAndNetNode{})
	if err != nil {
		return 0, "", errors.Capture(err)
	}

	var lifeAndNetNode unitLifeAndNetNode
	err = tx.Query(ctx, queryUnitStmt, unitUUID).Get(&lifeAndNetNode)
	if errors.Is(err, sqlair.ErrNoRows) {
		return 0, "", errors.Errorf("%w: %s", applicationerrors.UnitNotFound, unitUUID)
	} else if err != nil {
		return 0, "", errors.Errorf("querying unit %q life: %w", unitUUID, err)
	}

	return life.Life(lifeAndNetNode.LifeID), lifeAndNetNode.NetNodeID, nil
}

// GetCAASUnitRegistered checks if a caas unit by the provided name is already
// registered in the model. False is returned when no unit exists, otherwise
// the units existing uuid and netnode uuid is returned.
func (st *State) GetCAASUnitRegistered(
	ctx context.Context,
	uName coreunit.Name,
) (bool, coreunit.UUID, domainnetwork.NetNodeUUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, "", "", errors.Capture(err)
	}

	var (
		unitNameInput = unitName{Name: uName.String()}
		dbVal         unitUUIDAndNetNode
	)

	q := "SELECT &unitUUIDAndNetNode.* FROM unit WHERE name = $unitName.name"
	stmt, err := st.Prepare(q, unitNameInput, dbVal)
	if err != nil {
		return false, "", "", errors.Capture(err)
	}

	var registered bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, unitNameInput).Get(&dbVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err == nil {
			registered = true
		}
		return err
	})

	if err != nil {
		return false, "", "", errors.Capture(err)
	}

	if !registered {
		return false, "", "", nil
	}

	return true,
		coreunit.UUID(dbVal.UUID),
		domainnetwork.NetNodeUUID(dbVal.NetNodeUUID),
		nil
}

// InitialWatchStatementUnitAddressesHash returns the initial namespace query
// for the unit addresses hash watcher as well as the tables to be watched
// (ip_address and application_endpoint)
func (st *State) InitialWatchStatementUnitAddressesHash(
	appUUID coreapplication.UUID,
	netNodeUUID string,
) (string, string, eventsource.NamespaceQuery) {
	queryFunc := func(ctx context.Context, runner database.TxnRunner) ([]string, error) {
		var (
			spaceAddresses   []spaceAddress
			endpointBindings map[string]string
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
	return "custom_unit_name_lifecycle", func(ctx context.Context, runner database.TxnRunner) ([]string, error) {
		var unitNames []string
		err := runner.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			var err error
			unitNames, err = st.getUnitNamesForNetNode(ctx, tx, netNodeUUID)
			return err
		})
		if err != nil {
			return nil, errors.Errorf("querying unit names for net node %q: %w", netNodeUUID, err)
		}
		return unitNames, nil
	}
}

// InitialWatchStatementUnitLife returns the initial namespace query for the
// application unit life watcher.
func (st *State) InitialWatchStatementUnitLife(appName string) (string, eventsource.NamespaceQuery) {
	queryFunc := func(ctx context.Context, runner database.TxnRunner) ([]string, error) {
		app := applicationName{Name: appName}
		stmt, err := st.Prepare(`
SELECT u.uuid AS &unitUUID.uuid
FROM unit u
JOIN application a ON a.uuid = u.application_uuid
WHERE a.name = $applicationName.name
`, app, unitUUID{})
		if err != nil {
			return nil, errors.Capture(err)
		}
		var result []unitUUID
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
			uuids[i] = r.UnitUUID
		}
		return uuids, nil
	}
	return "unit", queryFunc
}

// GetApplicationUnitLife returns the life values for the specified units of the
// given application. The supplied ids may belong to a different application;
// the application name is used to filter.
func (st *State) GetApplicationUnitLife(ctx context.Context, appName string, ids ...coreunit.UUID) (map[string]int, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	unitUUIDs := unitUUIDs(ids)

	lifeQuery := `
SELECT (u.uuid, u.life_id) AS (&unitUUIDLife.*)
FROM unit u
JOIN application a ON a.uuid = u.application_uuid
WHERE u.uuid IN ($unitUUIDs[:])
AND a.name = $applicationName.name
`

	app := applicationName{Name: appName}
	lifeStmt, err := st.Prepare(lifeQuery, app, unitUUIDLife{}, unitUUIDs)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var lifes []unitUUIDLife
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
	result := make(map[string]int)
	for _, u := range lifes {
		result[u.UnitUUID] = u.LifeID
	}
	return result, nil
}

// GetAllUnitLifeForApplication returns a map of the unit names and their lives
// for the given application.
//   - If the application is not found, [applicationerrors.ApplicationNotFound]
//     is returned.
func (st *State) GetAllUnitLifeForApplication(ctx context.Context, appID coreapplication.UUID) (map[string]int, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	ident := entityUUID{UUID: appID.String()}
	appExistsQuery := `
SELECT &entityUUID.*
FROM application
WHERE uuid = $entityUUID.uuid;
`
	appExistsStmt, err := st.Prepare(appExistsQuery, ident)
	if err != nil {
		return nil, errors.Errorf("preparing query for application %q: %w", ident.UUID, err)
	}

	lifeQuery := `
SELECT (u.name, u.life_id) AS (&unitNameLife.*)
FROM unit u
WHERE u.application_uuid = $entityUUID.uuid
`

	app := entityUUID{UUID: appID.String()}
	lifeStmt, err := st.Prepare(lifeQuery, app, unitNameLife{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var lifes []unitNameLife
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, appExistsStmt, ident).Get(&ident)
		if errors.Is(err, sql.ErrNoRows) {
			return applicationerrors.ApplicationNotFound
		} else if err != nil {
			return errors.Errorf("checking application %q exists: %w", ident.UUID, err)
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
	result := make(map[string]int)
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
func (st *State) GetUnitMachineName(ctx context.Context, unitUUID string) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}
	arg := unitMachineName{
		UnitUUID: unitUUID,
	}
	stmt, err := st.Prepare(`
SELECT (m.name) AS (&unitMachineName.*)
FROM   unit AS u
JOIN   machine AS m ON u.net_node_uuid = m.net_node_uuid
WHERE  u.uuid = $unitMachineName.unit_uuid
`, arg)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkUnitNotDead(ctx, tx, unitUUID); err != nil {
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
func (st *State) GetUnitMachineUUID(ctx context.Context, unitUUID string) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var machineUUID string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkUnitNotDead(ctx, tx, unitUUID); err != nil {
			return errors.Capture(err)
		}

		machineUUID, err = st.getUnitMachineUUID(ctx, tx, unitUUID)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return "", errors.Capture(err)
	}

	return machineUUID, nil
}

func (st *State) getUnitMachineUUID(ctx context.Context, tx *sqlair.TX, unitUUID string) (string, error) {
	arg := unitMachineUUID{
		UnitUUID: unitUUID,
	}
	stmt, err := st.Prepare(`
SELECT (m.uuid) AS (&unitMachineUUID.*)
FROM   unit AS u
JOIN   machine AS m ON u.net_node_uuid = m.net_node_uuid
WHERE  u.uuid = $unitMachineUUID.unit_uuid
`, arg)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, arg).Get(&arg)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", applicationerrors.UnitMachineNotAssigned
	} else if err != nil {
		return "", errors.Capture(err)
	}

	return arg.MachineUUID, nil
}

func (st *State) getNonDeadUnitNetNodeByUnitName(ctx context.Context, tx *sqlair.TX, unitName string) (string, error) {
	val := nameWithNetNodeAndLife{
		Name: unitName,
	}
	stmt, err := st.Prepare(`
SELECT &nameWithNetNodeAndLife.*
FROM   unit
WHERE  name = $nameWithNetNodeAndLife.name
`, val)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, val).Get(&val)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("unit %q not found", unitName).Add(applicationerrors.UnitNotFound)
	} else if err != nil {
		return "", errors.Capture(err)
	}

	switch val.LifeID {
	case life.Dead:
		return "", applicationerrors.UnitIsDead
	default:
		return val.NetNodeUUID, nil
	}
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
	ctx context.Context,
	appUUID coreapplication.UUID,
	args ...application.AddIAASUnitArg,
) ([]coreunit.Name, []coremachine.Name, error) {
	if len(args) == 0 {
		return nil, nil, nil
	}

	db, err := st.DB(ctx)
	if err != nil {
		return nil, nil, errors.Capture(err)
	}

	var (
		unitNames    []coreunit.Name
		machineNames []coremachine.Name
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkApplicationAlive(ctx, tx, appUUID.String()); err != nil {
			return errors.Capture(err)
		}

		charmUUID, err := st.getCharmIDByApplicationUUID(ctx, tx, appUUID.String())
		if err != nil {
			return errors.Errorf("getting application %q charm uuid: %w", appUUID, err)
		}

		for i, arg := range args {
			uName, mNames, err := st.InsertIAASUnit(ctx, tx, appUUID.String(), charmUUID, arg)
			if err != nil {
				return errors.Errorf("inserting unit %d: %w ", i, err)
			}
			machineNames = append(machineNames, mNames...)
			unitNames = append(unitNames, uName)
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
	ctx context.Context,
	appUUID coreapplication.UUID,
	args ...application.AddCAASUnitArg,
) ([]coreunit.Name, error) {
	if len(args) == 0 {
		return nil, nil
	}

	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var unitNames []coreunit.Name
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		charmUUID, err := st.getCharmIDByApplicationUUID(ctx, tx, appUUID.String())
		if err != nil {
			return errors.Errorf("getting application %q charm uuid: %w", appUUID, err)
		}

		for _, arg := range args {
			unitName, err := st.insertCAASUnit(ctx, tx, appUUID.String(), charmUUID, arg)
			if err != nil {
				return errors.Errorf("inserting unit %q: %w ", unitName, err)
			}

			unitNames = append(unitNames, coreunit.Name(unitName))
		}
		return nil
	})
	return unitNames, errors.Capture(err)
}

// GetUnitPrincipal gets the subordinates principal unit. If no principal unit
// is found, for example, when the unit is not a subordinate, then false is
// returned.
func (st *State) GetUnitPrincipal(
	ctx context.Context,
	unitName coreunit.Name,
) (coreunit.Name, bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", false, errors.Capture(err)
	}

	arg := principal{
		SubordinateUnitName: unitName,
	}

	stmt, err := st.Prepare(`
SELECT principal.name AS &principal.principal_unit_name
FROM   unit AS principal
JOIN   unit_principal AS up ON principal.uuid = up.principal_uuid
JOIN   unit AS sub ON up.unit_uuid = sub.uuid
WHERE  sub.name = $principal.subordinate_unit_name
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

// IsSubordinateApplication returns true if the application is a subordinate
// application.
// The following errors may be returned:
// - [appliationerrors.ApplicationNotFound] if the application does not exist
func (st *State) IsSubordinateApplication(
	ctx context.Context,
	applicationUUID coreapplication.UUID,
) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	type getSubordinate struct {
		ApplicationUUID coreapplication.UUID `db:"application_uuid"`
		Subordinate     bool                 `db:"subordinate"`
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
	db, err := st.DB(ctx)
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
		if err := st.checkUnitExistsByName(ctx, tx, unitName.String()); err != nil {
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

// GetUnitNameForUUID returns the name of the unit with the given UUID.
//
// The following errors can be expected:
// - [applicationerrors.UnitNotFound] if the unit does not exist.
func (st *State) GetUnitNameForUUID(
	ctx context.Context,
	uuid coreunit.UUID,
) (coreunit.Name, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var (
		unitUUIDInput = unitUUID{UnitUUID: uuid.String()}
		dbVal         unitName
	)

	stmt, err := st.Prepare(
		"SELECT &unitName.* FROM unit WHERE uuid = $unitUUID.uuid",
		unitUUIDInput, dbVal,
	)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, unitUUIDInput).Get(&dbVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.New("unit does not exist").Add(
				applicationerrors.UnitNotFound,
			)
		}
		return err
	})

	if err != nil {
		return "", errors.Capture(err)
	}

	return coreunit.Name(dbVal.Name), nil
}

// GetUnitUUIDByName returns the UUID for the named unit, returning an error
// satisfying [applicationerrors.UnitNotFound] if the unit doesn't exist.
func (st *State) GetUnitUUIDByName(ctx context.Context, name coreunit.Name) (coreunit.UUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var uuid string
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

	return coreunit.UUID(uuid), nil
}

func (st *State) getUnitUUIDByName(
	ctx context.Context,
	tx *sqlair.TX,
	name coreunit.Name,
) (string, error) {
	unitName := unitName{Name: name.String()}

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

func (st *State) getUnitDetails(ctx context.Context, tx *sqlair.TX, unitName string) (*unitDetails, error) {
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
	db, err := st.DB(ctx)
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

	now := new(st.clock.Now().UTC())
	addUnitArg := application.AddCAASUnitArg{
		AddUnitArg: application.AddUnitArg{
			CreateUnitStorageArg: arg.CreateUnitStorageArg,
			UnitUUID:             arg.UnitUUID,
			NetNodeUUID:          arg.NetNodeUUID,
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
		},
		CloudContainer: cloudContainer,
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		appDetails, err := st.getApplicationDetails(ctx, tx, appName)
		if err != nil {
			return errors.Errorf("querying life for application %q: %w", appName, err)
		} else if appDetails.LifeID != life.Alive {
			return errors.Errorf("registering application %q: %w", appName, applicationerrors.ApplicationNotAlive)
		} else if appDetails.IsApplicationSynthetic {
			return errors.Errorf("registering unit for synthetic application %q", appName)
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

			uuid, err := st.insertCAASUnitWithName(
				ctx, tx, appUUID, appDetails.CharmUUID, arg.UnitName.String(), addUnitArg,
			)
			if err != nil {
				return errors.Errorf("inserting new caas application %s: %w", arg.UnitName, err)
			}

			err = st.setUnitPassword(ctx, tx, uuid, application.PasswordInfo{
				PasswordHash:  arg.PasswordHash,
				HashAlgorithm: application.HashAlgorithmSHA256,
			})
			if err != nil {
				return errors.Errorf("setting password for unit %q: %w", arg.UnitName, err)
			}

		} else if err != nil {
			return errors.Errorf("checking unit life %q: %w", arg.UnitName, err)
		}
		if unitLife == life.Dead {
			return errors.Errorf("dead unit %q already exists", arg.UnitName).Add(applicationerrors.UnitAlreadyExists)
		}

		// Unit already exists and is not dead. Update the cloud container.
		toUpdate, err := st.getUnitDetails(ctx, tx, arg.UnitName.String())
		if err != nil {
			return errors.Capture(err)
		}

		err = st.upsertUnitCloudContainer(ctx, tx, toUpdate.Name, toUpdate.UnitUUID, toUpdate.NetNodeID, cloudContainer)
		if err != nil {
			return errors.Errorf("updating cloud container for unit %q: %w", arg.UnitName, err)
		}

		err = st.setFilesystemProviderIDs(ctx, tx, arg.FilesystemProviderIDs)
		if err != nil {
			return errors.Errorf(
				"setting filesystem provider IDs for unit %q: %w",
				arg.UnitName, err,
			)
		}

		err = st.setFilesystemAttachmentProviderIDs(ctx, tx, arg.FilesystemAttachmentProviderIDs)
		if err != nil {
			return errors.Errorf(
				"setting filesystem attachment provider IDs for unit %q: %w",
				arg.UnitName, err,
			)
		}

		// TODO(storage): set volume and volume attachment provider IDs.

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

func (st *State) setUnitPassword(ctx context.Context, tx *sqlair.TX, unitUUID string, password application.PasswordInfo) error {
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
	appUUID, charmUUID string,
	args application.AddCAASUnitArg,
) (string, error) {
	unitName, err := st.newUnitName(ctx, tx, appUUID)
	if err != nil {
		return "", errors.Errorf("getting new unit name for application %q: %w", appUUID, err)
	}

	_, err = st.insertCAASUnitWithName(ctx, tx, appUUID, charmUUID, unitName, args)
	if err != nil {
		return "", errors.Capture(err)
	}

	return unitName, nil
}

// insertCAASUnitWithName inserts a new CAAS unit into the model using the
// supplied unit name. Returned is the uuid for the new unit.
//
// The following errors can be expected:
//   - [storageerrors.StorageInstanceNotFound] when any storage instance
//     in [application.AddUnitArg.CreateUnitStorageArg.ExistingStorageInstanceUUIDsToCheck]
//     does not exist.
//   - [storageerrors.StorageInstanceNotAlive] when any storage instance
//     in [application.AddUnitArg.CreateUnitStorageArg.ExistingStorageInstanceUUIDsToCheck]
//     is not alive.
//   - [applicationerrors.StorageInstanceUnexpectedAttachments] when a storage
//     instance has attachments outside
//     [internal.StorageInstanceAttachmentCheckArgs.ExpectedAttachments] or is
//     missing expected attachments.
func (st *State) insertCAASUnitWithName(
	ctx context.Context,
	tx *sqlair.TX,
	appUUID, charmUUID, unitName string,
	args application.AddCAASUnitArg,
) (string, error) {
	unitUUID := args.UnitUUID.String()

	err := st.insertUnit(ctx, tx, appUUID, unitUUID, args.NetNodeUUID.String(), insertUnitArg{
		CharmUUID:      charmUUID,
		UnitName:       unitName,
		CloudContainer: args.CloudContainer,
		Constraints:    args.Constraints,
		UnitStatusArg:  args.UnitStatusArg,
	})
	if err != nil {
		return "", errors.Errorf("inserting unit for CAAS application %q: %w", appUUID, err)
	}

	// This checks that any existing Storage Instances being used as part of
	// creating this new unit exist and are alive.
	err = st.checkStorageInstancesExistAndAlive(
		ctx, tx, args.AddUnitArg.CreateUnitStorageArg.ExistingStorageInstanceUUIDsToCheck,
	)
	if err != nil {
		return "", errors.Errorf(
			"checking existing Storage Instances exist and are alive: %w", err,
		)
	}

	// This checks that any existing Storage Instances being used as part of
	// creating this new unit have the expected attachments on which the
	// information was calculated.
	err = st.checkStorageInstancesAttachmentExpectations(
		ctx,
		tx,
		args.AddUnitArg.CreateUnitStorageArg.StorageInstanceAttachmentCheckArgs,
	)
	if err != nil {
		return "", errors.Errorf(
			"checking pre condition for existing storage instance attachments: %w",
			err,
		)
	}

	err = st.insertUnitStorageDirectives(
		ctx, tx, unitUUID, charmUUID, args.StorageDirectives,
	)
	if err != nil {
		return "", errors.Errorf(
			"inserting storage directives for unit %q: %w", unitName, err,
		)
	}

	_, err = st.insertUnitStorageInstances(
		ctx, tx, args.StorageInstances,
	)
	if err != nil {
		return "", errors.Errorf(
			"inserting storage instances for unit %q: %w", unitName, err,
		)
	}

	err = st.insertUnitStorageAttachments(
		ctx,
		tx,
		unitUUID,
		args.NewStorageToAttach,
	)
	if err != nil {
		return "", errors.Errorf(
			"inserting storage attachments for unit %q: %w", unitName, err,
		)
	}

	err = st.insertUnitStorageOwnership(ctx, tx, unitUUID, args.StorageToOwn)
	if err != nil {
		return "", errors.Errorf(
			"inserting storage ownership for unit %q: %w", unitName, err,
		)
	}

	// If we are using any existing Storage Instances we need to ensure that the
	// charm name column has been updated.
	err = st.setStorageInstancesCharmName(
		ctx, tx, args.StorageInstanceCharmNameSetArgs,
	)
	if err != nil {
		return "", errors.Errorf(
			"updating storage instance charm name for new unit %q: %w",
			unitName, err,
		)
	}

	return unitUUID, nil
}

// UpdateCAASUnit updates the cloud container for specified unit,
// returning an error satisfying [applicationerrors.UnitNotFoundError]
// if the unit doesn't exist.
func (st *State) UpdateCAASUnit(ctx context.Context, unitName coreunit.Name, params application.UpdateCAASUnitParams) error {
	db, err := st.DB(ctx)
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
		toUpdate, err := st.getUnitDetails(ctx, tx, unitName.String())
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

// GetUnitStorageDirectivesCurrentNext returns the current and the next storage
// directives for this unit, if the unit was to switch to the given charm.
//
// The following errors can be expected:
// - [applicationerrors.UnitNotFound] when the unit does not exist.
// - [applicationerrors.CharmNotFound] when the charm does not exist.
func (st *State) GetUnitStorageRefreshArgs(
	ctx context.Context, unit coreunit.UUID, next corecharm.ID,
) (applicationinternal.UnitStorageRefreshArgs, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return applicationinternal.UnitStorageRefreshArgs{}, errors.Capture(err)
	}

	charmUUID := charmUUID{UUID: next.String()}
	unitUUID := unitUUID{UnitUUID: unit.String()}

	unitStmt, err := st.Prepare(`
SELECT &unitNetNodeWithCharmAndMachine.* FROM (
	SELECT    u.uuid,
	          u.net_node_uuid,
	          u.charm_uuid, 
	          m.uuid AS machine_uuid
	FROM      unit u
	LEFT JOIN machine m ON u.net_node_uuid = m.net_node_uuid
	WHERE     u.uuid = $unitUUID.uuid
)
`, unitUUID, unitNetNodeWithCharmAndMachine{})
	if err != nil {
		return applicationinternal.UnitStorageRefreshArgs{}, errors.Capture(err)
	}

	sdStmt, err := st.Prepare(`
SELECT &storageDirective.* FROM (
    SELECT usd.count,
           usd.size_mib,
           usd.storage_name,
           usd.storage_pool_uuid,
           cm.name AS charm_metadata_name,
           csk.kind AS charm_storage_kind,
           cs.count_max AS count_max
    FROM   unit_storage_directive usd
    JOIN   charm_storage cs ON cs.charm_uuid = usd.charm_uuid AND
                               cs.name = usd.storage_name
    JOIN   charm_metadata cm ON cm.charm_uuid = usd.charm_uuid
    JOIN   charm_storage_kind csk ON csk.id = cs.storage_kind_id
    WHERE  usd.unit_uuid = $unitUUID.uuid AND
           usd.charm_uuid = $charmUUID.charm_uuid
)`, unitUUID, charmUUID, storageDirective{})
	if err != nil {
		return applicationinternal.UnitStorageRefreshArgs{}, errors.Capture(err)
	}

	unitVal := unitNetNodeWithCharmAndMachine{}
	sdVals := []storageDirective{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, unitStmt, unitUUID).Get(&unitVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"unit %q does not exist", unit,
			).Add(applicationerrors.UnitNotFound)
		} else if err != nil {
			return errors.Errorf(
				"getting unit %q details: %w", unit, err,
			)
		}

		err = st.checkCharmExists(ctx, tx, charmUUID.UUID)
		if err != nil {
			return errors.Errorf(
				"checking charm %q exists: %w", charmUUID.UUID, err,
			)
		}

		err = tx.Query(ctx, sdStmt, unitUUID, charmUUID).GetAll(&sdVals)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return applicationinternal.UnitStorageRefreshArgs{}, errors.Capture(err)
	}

	retVal := applicationinternal.UnitStorageRefreshArgs{
		NetNodeUUID:      domainnetwork.NetNodeUUID(unitVal.NetNodeUUID),
		CurrentCharmUUID: corecharm.ID(unitVal.CharmUUID),
		RefreshCharmUUID: next,
		RefreshStorageDirectives: make(
			[]applicationinternal.StorageDirective, 0, len(sdVals)),
	}
	if unitVal.MachineUUID.Valid {
		retVal.MachineUUID = new(coremachine.UUID(unitVal.MachineUUID.V))
	}
	for _, v := range sdVals {
		sd := applicationinternal.StorageDirective{
			CharmMetadataName: v.CharmMetadataName,
			CharmStorageType:  charm.StorageType(v.CharmStorageKind),
			Count:             v.Count,
			MaxCount:          v.CountMax,
			Name:              domainstorage.Name(v.StorageName),
			PoolUUID:          domainstorage.StoragePoolUUID(v.StoragePoolUUID),
			Size:              v.SizeMiB,
		}
		retVal.RefreshStorageDirectives = append(
			retVal.RefreshStorageDirectives, sd)
	}

	return retVal, nil
}

// UpdateUnitCharm updates the currently running charm marker for the given
// unit, creates new storage instances required, and deletes unit storage
// directives for the old charm.
// The following errors may be returned:
// - [applicationerrors.UnitNotFound] if the unit does not exist.
// - [applicationerrors.UnitIsDead] if the unit is dead.
// - [applicationerrors.CharmNotFound] if the charm does not exist.
func (st *State) UpdateUnitCharm(
	ctx context.Context, arg applicationinternal.UpdateUnitCharmArg,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	unitUUID := unitUUID{UnitUUID: arg.UUID.String()}
	targetCharmUUID := charmUUID{UUID: arg.CharmUUID.String()}

	unitStmt, err := st.Prepare(`
SELECT &unitLifeWithCharm.*
FROM   unit u
WHERE  u.uuid = $unitUUID.uuid
`, unitUUID, unitLifeWithCharm{})
	if err != nil {
		return errors.Capture(err)
	}

	updateUnitCharmStmt, err := st.Prepare(`
UPDATE unit
SET    charm_uuid = $charmUUID.charm_uuid
WHERE  uuid = $unitUUID.uuid
`, unitUUID, targetCharmUUID)
	if err != nil {
		return errors.Capture(err)
	}

	deleteStorageDirectiveStmt, err := st.Prepare(`
DELETE FROM unit_storage_directive
WHERE       unit_uuid = $unitUUID.uuid AND
            charm_uuid = $charmUUID.charm_uuid
`, unitUUID, charmUUID{})
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		unitLifeCharm := unitLifeWithCharm{}
		err := tx.Query(ctx, unitStmt, unitUUID).Get(&unitLifeCharm)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"unit %q not found", arg.UUID,
			).Add(applicationerrors.UnitNotFound)
		} else if err != nil {
			return errors.Errorf(
				"getting unit %q charm and life: %w", arg.UUID, err,
			)
		}
		// Ensure unit is alive for update.
		if unitLifeCharm.LifeID == int(life.Dead) {
			return errors.Errorf(
				"unit %q is dead", arg.UUID,
			).Add(applicationerrors.UnitIsDead)
		}
		// Ensure the target charm exists.
		err = st.checkCharmExists(ctx, tx, targetCharmUUID.UUID)
		if err != nil {
			return errors.Capture(err)
		}

		// Update unit charm UUID.
		err = tx.Query(ctx, updateUnitCharmStmt, targetCharmUUID, unitUUID).Run()
		if err != nil {
			return errors.Errorf(
				"updating unit %q charm to %q: %w",
				arg.UUID, arg.CharmUUID, err,
			)
		}

		// Insert new storage instances.
		_, err = st.unitState.insertUnitStorageInstances(
			ctx, tx, arg.UnitStorage.StorageInstances)
		if err != nil {
			return errors.Capture(err)
		}
		err = st.unitState.insertUnitStorageOwnership(
			ctx, tx, arg.UUID.String(), arg.UnitStorage.StorageToOwn)
		if err != nil {
			return errors.Capture(err)
		}
		err = st.unitState.insertUnitStorageAttachments(
			ctx, tx, arg.UUID.String(), arg.UnitStorage.StorageToAttach)
		if err != nil {
			return errors.Capture(err)
		}
		if arg.MachineUUID != nil && arg.IAASUnitStorage != nil {
			err = st.unitState.insertMachineFilesystemOwnership(
				ctx, tx, *arg.MachineUUID, arg.IAASUnitStorage.FilesystemsToOwn)
			if err != nil {
				return errors.Capture(err)
			}
			err = st.unitState.insertMachineVolumeOwnership(
				ctx, tx, *arg.MachineUUID, arg.IAASUnitStorage.VolumesToOwn)
			if err != nil {
				return errors.Capture(err)
			}
		}

		// Delete old unit storage directives.
		oldCharmUUID := charmUUID{
			UUID: unitLifeCharm.CharmUUID,
		}
		err = tx.Query(
			ctx, deleteStorageDirectiveStmt, unitUUID, oldCharmUUID,
		).Run()
		if err != nil {
			return errors.Errorf(
				"deleting previous unit %q storage directives: %w",
				arg.UUID, err,
			)
		}

		return nil
	})
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// GetUnitRefreshAttributes returns the unit refresh attributes for the
// specified unit. If the unit is not found, an error satisfying
// [applicationerrors.UnitNotFound] is returned. This doesn't take into account
// life, so it can return the attributes of a unit even if it's dead.
func (st *State) GetUnitRefreshAttributes(ctx context.Context, unitName coreunit.Name) (application.UnitAttributes, error) {
	db, err := st.DB(ctx)
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

// GetMachineUUIDAndNetNodeForName is responsible for identifying the uuid
// and net node for a machine by it's name.
//
// The following errors may be expected:
// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
// machine exists for the supplied machine name.
func (st *State) GetMachineUUIDAndNetNodeForName(
	ctx context.Context, mName string,
) (coremachine.UUID, domainnetwork.NetNodeUUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", "", errors.Capture(err)
	}

	var (
		nameInput = machineName{Name: mName}
		dbVal     machineUUIDWithNetNode
	)

	q := `
SELECT &machineUUIDWithNetNode.*
FROM   machine
WHERE  name = $machineName.name
`

	stmt, err := st.Prepare(q, nameInput, dbVal)
	if err != nil {
		return "", "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, nameInput).Get(&dbVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"machine with name %q does not exist", mName,
			).Add(machineerrors.MachineNotFound)
		}

		return err
	})

	if err != nil {
		return "", "", errors.Capture(err)
	}

	return coremachine.UUID(dbVal.UUID),
		domainnetwork.NetNodeUUID(dbVal.NetNodeUUID),
		nil
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
	db, err := st.DB(ctx)
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

// GetAllUnitNames returns a slice of all unit names in the model.
func (st *State) GetAllUnitNames(ctx context.Context) ([]coreunit.Name, error) {
	db, err := st.DB(ctx)
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
		return coreunit.Name(r.Name)
	}), nil
}

// GetUnitNamesForApplication returns a slice of the unit names for the given application
// The following errors may be returned:
// - [applicationerrors.ApplicationIsDead] if the application is dead
// - [applicationerrors.ApplicationNotFound] if the application does not exist
func (st *State) GetUnitNamesForApplication(ctx context.Context, uuid coreapplication.UUID) ([]coreunit.Name, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	appUUID := entityUUID{UUID: uuid.String()}
	query := `
SELECT &unitName.*
FROM unit
JOIN charm AS c ON unit.charm_uuid = c.uuid
WHERE application_uuid = $entityUUID.uuid AND c.source_id < 2`
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
		return coreunit.Name(r.Name)
	}), nil
}

// GetUnitUUIDAndNetNodeForName returns the unit uuid and net node uuid for a
// unit matching the supplied name.
//
// The following errors may be expected:
// - [applicationerrors.UnitNotFound] if no unit exists for the supplied
// name.
func (st *State) GetUnitUUIDAndNetNodeForName(
	ctx context.Context, name coreunit.Name,
) (coreunit.UUID, domainnetwork.NetNodeUUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", "", errors.Capture(err)
	}

	var (
		input = unitName{Name: name.String()}
		dbVal unitUUIDAndNetNode
	)

	stmt, err := st.Prepare(`
SELECT &unitUUIDAndNetNode.*
FROM   unit
WHERE  name = $unitName.name
`,
		input, dbVal,
	)
	if err != nil {
		return "", "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, input).Get(&dbVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"unit with name %q does not exist", name,
			).Add(applicationerrors.UnitNotFound)
		}
		return err
	})
	if err != nil {
		return "", "", errors.Capture(err)
	}

	return coreunit.UUID(dbVal.UUID),
		domainnetwork.NetNodeUUID(dbVal.NetNodeUUID),
		nil
}

// GetUnitNamesForNetNode returns a slice of the unit names for the given net node
// The following errors may be returned:
func (st *State) GetUnitNamesForNetNode(ctx context.Context, uuid string) ([]coreunit.Name, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var unitNames []string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		unitNames, err = st.getUnitNamesForNetNode(ctx, tx, uuid)
		return err
	})
	if err != nil {
		return nil, errors.Errorf("querying unit names for net node %q: %w", uuid, err)
	}
	return transform.Slice(unitNames, func(n string) coreunit.Name {
		return coreunit.Name(n)
	}), nil
}

func (st *State) getUnitNamesForNetNode(ctx context.Context, tx *sqlair.TX, uuid string) ([]string, error) {
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
		return []string{}, nil
	} else if err != nil {
		return nil, errors.Capture(err)
	}

	return transform.Slice(result, func(r unitName) string {
		return r.Name
	}), nil
}

// SetUnitWorkloadVersion sets the workload version for the given unit.
func (st *State) SetUnitWorkloadVersion(ctx context.Context, unitName coreunit.Name, version string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		unitUUID, err := st.getUnitUUIDByName(ctx, tx, unitName)
		if err != nil {
			return errors.Errorf("getting uuid for unit %q: %w", unitName, err)
		}
		return st.setUnitWorkloadVersion(ctx, tx, unitUUID, version)
	})
	if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// GetWorkloadVersion returns the workload version for the given unit.
func (st *State) GetUnitWorkloadVersion(ctx context.Context, unitName coreunit.Name) (string, error) {
	db, err := st.DB(ctx)
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

// GetAllUnitCloudContainerIDsForApplication returns a map of the unit names
// and their cloud container provider IDs for the given application.
//   - If the application is dead, [applicationerrors.ApplicationIsDead] is returned.
//   - If the application is not found, [applicationerrors.ApplicationNotFound]
//     is returned.
func (st *State) GetAllUnitCloudContainerIDsForApplication(
	ctx context.Context,
	appUUID coreapplication.UUID,
) (map[coreunit.Name]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	input := entityUUID{UUID: appUUID.String()}
	query := `
SELECT (u.name, kp.provider_id) AS (&unitNameCloudContainer.*)
FROM unit u
JOIN k8s_pod kp ON u.uuid = kp.unit_uuid
WHERE u.application_uuid = $entityUUID.uuid
`
	stmt, err := st.Prepare(query, unitNameCloudContainer{}, input)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var result []unitNameCloudContainer
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.checkApplicationNotDead(ctx, tx, appUUID)
		if err != nil {
			return errors.Capture(err)
		}
		err = tx.Query(ctx, stmt, input).GetAll(&result)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	res := make(map[coreunit.Name]string, len(result))
	for _, v := range result {
		res[coreunit.Name(v.Name)] = v.ProviderID
	}
	return res, nil
}

// GetUnitsK8sPodInfo returns information about the k8s pods for all alive units.
// If any of the requested pieces of data are not present yet, zero values will
// be returned in their place.
func (st *State) GetUnitsK8sPodInfo(ctx context.Context) (map[coreunit.Name]application.K8sPodInfo, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	deadLife := entityLife{
		LifeID: int(life.Dead),
	}

	infoQuery := `
SELECT
	u.name AS &unitK8sPodInfoWithName.name,
	k.provider_id AS &unitK8sPodInfoWithName.provider_id,
	ip.address_value AS &unitK8sPodInfoWithName.address,
	COALESCE(
		GROUP_CONCAT(kpp.port, ','),
		''
	) AS &unitK8sPodInfoWithName.ports
FROM
	unit AS u
LEFT JOIN k8s_pod AS k ON u.uuid = k.unit_uuid
LEFT JOIN link_layer_device lld ON lld.net_node_uuid = u.net_node_uuid
LEFT JOIN ip_address ip ON ip.device_uuid = lld.uuid
LEFT JOIN k8s_pod_port kpp ON kpp.unit_uuid = u.uuid
WHERE
	u.life_id != $entityLife.life_id
GROUP BY
	u.name
`
	stmt, err := st.Prepare(infoQuery, unitK8sPodInfoWithName{}, deadLife)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var infos []unitK8sPodInfoWithName

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, deadLife).GetAll(&infos); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	result := make(map[coreunit.Name]application.K8sPodInfo)
	for _, info := range infos {
		ports := make([]k8sPodPort, 0)
		for p := range strings.SplitSeq(info.Ports, ",") {
			ports = append(ports, k8sPodPort{Port: p})
		}
		result[coreunit.Name(info.UnitName)] = encodeK8sPodInfo(
			unitK8sPodInfo{
				ProviderID: info.ProviderID,
				Address:    info.Address,
			},
			ports,
		)
	}
	return result, nil
}

// GetUnitK8sPodInfo returns information about the k8s pod for the given unit.
// If any of the requested pieces of data are not present yet, zero values will
// be returned in their place.
// The following errors may be returned:
// - [applicationerrors.UnitNotFound] if the unit does not exist
// - [applicationerrors.UnitIsDead] if the unit is dead
func (st *State) GetUnitK8sPodInfo(ctx context.Context, name coreunit.Name) (application.K8sPodInfo, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return application.K8sPodInfo{}, errors.Capture(err)
	}

	unitName := unitName{Name: name.String()}
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
		if err := st.checkUnitNotDeadByName(ctx, tx, name.String()); err != nil {
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
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	ident := unitName{Name: name.String()}
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

// GetUnitNetNodeUUID returns the net node UUID for the specified unit.
// The following error types can be expected:
// - [applicationerrors.UnitNotFound]: when the unit is not found.
func (st *State) GetUnitNetNodeUUID(ctx context.Context, uuid coreunit.UUID) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	ident := unitUUID{UnitUUID: uuid.String()}
	stmt, err := st.Prepare(`
SELECT &unitNetNodeUUID.*
FROM   unit u
WHERE  u.uuid = $unitUUID.uuid
`, unitNetNodeUUID{}, ident)
	if err != nil {
		return "", errors.Capture(err)
	}

	var netNodeUUID unitNetNodeUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, ident).Get(&netNodeUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("%w: %s", applicationerrors.UnitNotFound, uuid)
		}
		return errors.Capture(err)
	})
	if err != nil {
		return "", errors.Capture(err)
	}

	return netNodeUUID.NetNodeUUID, nil
}

// GetStorageAddInfoByUnitUUID returns the deploy metadata and how many
// storage instances exist for the named storage on the specified unit.
//
// The following error types can be expected:
// - [applicationerrors.StorageNameNotSupported]: when storage name is not defined in charm metadata.
func (st *State) GetStorageAddInfoByUnitUUID(
	ctx context.Context, unitUUID coreunit.UUID, storageName corestorage.Name,
) (internal.StorageInfoForAdd, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return internal.StorageInfoForAdd{}, errors.Capture(err)
	}

	var (
		addInfo storageInfoForAdd
		count   uint32
	)
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		addInfo, err = st.getStorageInstanceInfoForAdd(ctx, tx, unitUUID, storageName)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("storage %q is not found", storageName).Add(applicationerrors.StorageNameNotSupported)
		}
		if err != nil {
			return errors.Errorf("getting charm storage metadata for unit %q: %w", unitUUID, err)
		}
		count, err = st.getUnitStorageCount(ctx, tx, unitUUID, storageName)
		if err != nil {
			return errors.Errorf("getting storage count for unit %q storage %s: %w", unitUUID, storageName, err)
		}
		return nil
	}); err != nil {
		return internal.StorageInfoForAdd{}, errors.Capture(err)
	}
	return internal.StorageInfoForAdd{
		CharmStorageDefinitionForValidation: internal.CharmStorageDefinitionForValidation{
			Name:        addInfo.Name,
			Type:        domainapplicationcharm.StorageType(addInfo.Kind),
			CountMin:    addInfo.CountMin,
			CountMax:    addInfo.CountMax,
			MinimumSize: addInfo.MinimumSize,
		},
		AlreadyAttachedCount: count,
	}, nil
}

// GetIAASUnitContext returns IAAS context information required for the
// construction of a context factory.
//
// The following errors may be returned:
// - [applicationerrors.UnitNotFound] if the unit does not exist.
// - [applicationerrors.UnitIsDead] if the unit is dead.
func (st *State) GetIAASUnitContext(ctx context.Context, unitName string) (applicationinternal.IAASUnitContext, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return applicationinternal.IAASUnitContext{}, errors.Capture(err)
	}

	var (
		legacyProxySettings, jujuProxySettings applicationinternal.ProxySettings
		machineOpenedPortRanges                []unitEndpointOpenedPortRange
		unitAddress                            *string
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		netNodeUUID, err := st.getNonDeadUnitNetNodeByUnitName(ctx, tx, unitName)
		if err != nil {
			return errors.Errorf("getting net node for unit: %w", err)
		}

		legacyProxySettings, err = st.getLegacyProxySettings(ctx, tx)
		if err != nil {
			return errors.Errorf("getting legacy proxy settings: %w", err)
		}

		jujuProxySettings, err = st.getJujuProxySettings(ctx, tx)
		if err != nil {
			return errors.Errorf("getting proxy settings: %w", err)
		}

		machineOpenedPortRanges, err = st.getMachineOpenedPortRanges(ctx, tx, netNodeUUID)
		if err != nil {
			return errors.Errorf("getting machine opened port ranges: %w", err)
		}

		unitAddress, err = st.getUnitPrivateAddress(ctx, tx, netNodeUUID)
		if err != nil {
			return errors.Errorf("getting private address for unit: %w", err)
		}

		return nil
	})
	if err != nil {
		return applicationinternal.IAASUnitContext{}, errors.Capture(err)
	}

	decoded := port.UnitEndpointPortRanges(
		transform.Slice(machineOpenedPortRanges,
			func(p unitEndpointOpenedPortRange) port.UnitEndpointPortRange {
				return p.decodeToUnitEndpointPortRange()
			},
		),
	)

	return applicationinternal.IAASUnitContext{
		LegacyProxySettings:               legacyProxySettings,
		JujuProxySettings:                 jujuProxySettings,
		OpenedMachinePortRangesByEndpoint: decoded.ByUnitByEndpoint(),
		PrivateAddress:                    unitAddress,
	}, nil
}

// GetCAASUnitContext returns CAAS context information required for the
// construction of a context factory.
// The following errors may be returned:
// - [applicationerrors.UnitNotFound] if the unit does not exist.
// - [applicationerrors.UnitIsDead] if the unit is dead.
func (st *State) GetCAASUnitContext(ctx context.Context, unitName string) (applicationinternal.CAASUnitContext, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return applicationinternal.CAASUnitContext{}, errors.Capture(err)
	}

	var (
		legacyProxySettings, jujuProxySettings applicationinternal.ProxySettings
		unitOpenedPortRanges                   []unitEndpointOpenedPortRange
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkUnitNotDeadByName(ctx, tx, unitName); err != nil {
			return errors.Capture(err)
		}

		legacyProxySettings, err = st.getLegacyProxySettings(ctx, tx)
		if err != nil {
			return errors.Errorf("getting legacy proxy settings: %w", err)
		}

		jujuProxySettings, err = st.getJujuProxySettings(ctx, tx)
		if err != nil {
			return errors.Errorf("getting proxy settings: %w", err)
		}

		unitOpenedPortRanges, err = st.getUnitOpenedPortRanges(ctx, tx, unitName)
		if err != nil {
			return errors.Errorf("getting machine opened port ranges: %w", err)
		}

		return nil
	})
	if err != nil {
		return applicationinternal.CAASUnitContext{}, errors.Capture(err)
	}

	decoded := port.UnitEndpointPortRanges(
		transform.Slice(unitOpenedPortRanges,
			func(p unitEndpointOpenedPortRange) port.UnitEndpointPortRange {
				return p.decodeToUnitEndpointPortRange()
			}),
	)

	return applicationinternal.CAASUnitContext{
		LegacyProxySettings:        legacyProxySettings,
		JujuProxySettings:          jujuProxySettings,
		OpenedPortRangesByEndpoint: decoded.ByUnitByEndpoint(),
	}, nil
}

func (st *State) getLegacyProxySettings(ctx context.Context, tx *sqlair.TX) (applicationinternal.ProxySettings, error) {
	type modelConfig struct {
		Key   string `db:"key"`
		Value string `db:"value"`
	}

	stmt, err := st.Prepare(`
SELECT &modelConfig.*
FROM model_config
WHERE key IN ('http-proxy', 'https-proxy', 'ftp-proxy', 'no-proxy')
`, modelConfig{})
	if err != nil {
		return applicationinternal.ProxySettings{}, errors.Capture(err)
	}

	var configs []modelConfig
	err = tx.Query(ctx, stmt).GetAll(&configs)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return applicationinternal.ProxySettings{}, err
	}

	proxySettings := applicationinternal.ProxySettings{}
	for _, config := range configs {
		switch config.Key {
		case "http-proxy":
			proxySettings.HTTP = config.Value
		case "https-proxy":
			proxySettings.HTTPS = config.Value
		case "ftp-proxy":
			proxySettings.FTP = config.Value
		case "no-proxy":
			proxySettings.NoProxy = config.Value
		}
	}
	return proxySettings, nil
}

func (st *State) getJujuProxySettings(ctx context.Context, tx *sqlair.TX) (applicationinternal.ProxySettings, error) {
	type modelConfig struct {
		Key   string `db:"key"`
		Value string `db:"value"`
	}

	stmt, err := st.Prepare(`
SELECT &modelConfig.*
FROM model_config
WHERE key IN ('juju-http-proxy', 'juju-https-proxy', 'juju-ftp-proxy', 'juju-no-proxy')
`, modelConfig{})
	if err != nil {
		return applicationinternal.ProxySettings{}, errors.Capture(err)
	}

	var configs []modelConfig
	err = tx.Query(ctx, stmt).GetAll(&configs)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return applicationinternal.ProxySettings{}, err
	}

	proxySettings := applicationinternal.ProxySettings{}
	for _, config := range configs {
		switch config.Key {
		case "juju-http-proxy":
			proxySettings.HTTP = config.Value
		case "juju-https-proxy":
			proxySettings.HTTPS = config.Value
		case "juju-ftp-proxy":
			proxySettings.FTP = config.Value
		case "juju-no-proxy":
			proxySettings.NoProxy = config.Value
		}
	}
	return proxySettings, nil
}

type unitEndpointOpenedPortRange struct {
	UnitName coreunit.Name `db:"unit_name"`
	Protocol string        `db:"protocol"`
	FromPort int           `db:"from_port"`
	ToPort   int           `db:"to_port"`
	Endpoint string        `db:"endpoint"`
}

func (p unitEndpointOpenedPortRange) decodeToUnitEndpointPortRange() port.UnitEndpointPortRange {
	return port.UnitEndpointPortRange{
		UnitName:  p.UnitName,
		Endpoint:  p.Endpoint,
		PortRange: p.decodeToPortRange(),
	}
}

func (p unitEndpointOpenedPortRange) decodeToPortRange() network.PortRange {
	return network.PortRange{
		Protocol: p.Protocol,
		FromPort: p.FromPort,
		ToPort:   p.ToPort,
	}
}

func (st *State) getMachineOpenedPortRanges(
	ctx context.Context,
	tx *sqlair.TX,
	netNodeUUID string,
) ([]unitEndpointOpenedPortRange, error) {
	nUUID := entityUUID{UUID: netNodeUUID}

	query, err := st.Prepare(`
SELECT &unitEndpointOpenedPortRange.*
FROM v_port_range
JOIN unit ON unit_uuid = unit.uuid
WHERE unit.net_node_uuid = $entityUUID.uuid
`, unitEndpointOpenedPortRange{}, nUUID)
	if err != nil {
		return nil, errors.Errorf("preparing get machine opened ports statement: %w", err)
	}

	var results []unitEndpointOpenedPortRange
	err = tx.Query(ctx, query, nUUID).GetAll(&results)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, nil
	}
	return results, nil
}

func (st *State) getUnitOpenedPortRanges(
	ctx context.Context,
	tx *sqlair.TX,
	unitName string,
) ([]unitEndpointOpenedPortRange, error) {
	uName := unitEndpointOpenedPortRange{UnitName: coreunit.Name(unitName)}

	query, err := st.Prepare(`
SELECT &unitEndpointOpenedPortRange.*
FROM v_port_range
WHERE unit_name = $unitEndpointOpenedPortRange.unit_name
`, uName)
	if err != nil {
		return nil, errors.Errorf("preparing get unit opened ports statement: %w", err)
	}

	var results []unitEndpointOpenedPortRange
	err = tx.Query(ctx, query, uName).GetAll(&results)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, nil
	}
	return results, nil
}

func (st *State) getUnitPrivateAddress(ctx context.Context, tx *sqlair.TX, netNodeUUID string) (*string, error) {
	entityUUID := entityUUID{UUID: netNodeUUID}

	// A unit private address is determined by looking for IP addresses
	// associated with the unit's net node, and prioritising them as follows:
	//
	//  - Local cloud scoped IPv4 addresses
	//  - Local cloud scoped IPv6 addresses
	//  - Public or unknown scoped IPv4 addresses
	//  - Public or unknown scoped IPv6 addresses
	//  - Origin either from machine or provider
	//  - Real ethernet devices over virtual ethernet devices
	//
	// Loopback addresses are excluded.
	//
	// Note: unknown scope is included as a fallback for compatibility with the
	// openstack provider, though in practice we would expect these to be public
	// addresses and shouldn't be used.

	query, err := st.Prepare(`
SELECT
    a.address_value AS &unitAddress.value,
	d.device_type_id,
    CASE
        WHEN a.scope_id = 2 THEN 0
        WHEN a.scope_id IN (1, 0) THEN 1
        ELSE 2
    END AS scope_rank,
    CASE
        WHEN a.type_id = 0 THEN 0
        WHEN a.type_id = 1 THEN 1
        ELSE 2
    END AS type_rank,
	CASE
		WHEN a.origin_id = 0 THEN 1
		WHEN a.origin_id = 1 THEN 0
		ELSE 2
	END AS origin_rank
FROM net_node n
JOIN link_layer_device d ON n.uuid = d.net_node_uuid
JOIN ip_address a ON d.uuid = a.device_uuid
WHERE
    a.scope_id IN (0, 1, 2)
    AND a.config_type_id != 6
    AND n.uuid = $entityUUID.uuid
ORDER BY
    scope_rank,
    type_rank,
	origin_rank,
	d.device_type_id,
    a.address_value
LIMIT 1;
`, unitAddress{}, entityUUID)
	if err != nil {
		return nil, errors.Errorf("preparing get unit private address statement: %w", err)
	}

	var address unitAddress
	err = tx.Query(ctx, query, entityUUID).Get(&address)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Errorf("querying unit private address: %w", err)
	}
	return new(address.Value), nil
}

// GetCharmStorageAndInstanceInfoByUnitUUIDAndStorageUUID returns the metadata
// GetStorageAttachInfoByUnitUUIDAndStorageUUID returns the metadata
// and select details for the storage instance on the specified unit.
// The details include how many existing instances of the same named storage
// already exist, the requested size, and the instance's storage pool.
//
// The following errors can be expected:
// - [applicationerrors.UnitNotFound] when the unit does not exist.
// - [applicationerrors.StorageNameNotSupported] when the unit's charm does not
// support the storage name in use by the storage instance.
// - [storageerrors.StorageInstanceNotFound] when the storage instance does not
// exist.
func (st *State) GetStorageAttachInfoByUnitUUIDAndStorageUUID(
	ctx context.Context,
	unitUUID coreunit.UUID,
	storageUUID domainstorage.StorageInstanceUUID,
) (internal.StorageInstanceInfoForUnitAttach, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return internal.StorageInstanceInfoForUnitAttach{}, errors.Capture(err)
	}

	var (
		storageInstInfo        storageInstanceInfoForAttach
		storageInstAttachments []storageInstanceUnitAttachment
		unitStorageNameInfo    unitStorageNameInfo
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// err is defined locally so as not to be captured due to retries of
		// the transaction.
		var err error

		unitExists, err := st.checkUnitExists(ctx, tx, unitUUID.String())
		if err != nil {
			return errors.Errorf("check if unit exists: %w", err)
		}
		if !unitExists {
			return errors.Errorf(
				"unit %q does not exist", unitUUID,
			).Add(applicationerrors.UnitNotFound)
		}

		storageInstInfo, err = st.getStorageInstanceInfoForAttach(ctx, tx, storageUUID)
		if err != nil {
			return errors.Errorf(
				"getting storage instance information for attachment: %w", err,
			)
		}

		storageInstAttachments, err = st.getStorageInstanceUnitAttachments(ctx, tx, storageUUID)
		if err != nil {
			return errors.Errorf(
				"getting storage instance unit attachments: %w", err,
			)
		}

		// We use the name of the Storage Instance to lookup and find the
		// storage definition information for the unit.
		unitStorageNameInfo, err = st.getUnitStorageNameInfo(
			ctx, tx, unitUUID, storageInstInfo.StorageName,
		)
		if err != nil {
			return errors.Errorf(
				"getting unit storage name %q info: %w",
				storageInstInfo.StorageName,
				err,
			)
		}

		return nil
	})
	if err != nil {
		return internal.StorageInstanceInfoForUnitAttach{}, err
	}

	retVal := internal.StorageInstanceInfoForUnitAttach{
		StorageInstanceInfo: internal.StorageInstanceInfo{
			UUID:             domainstorage.StorageInstanceUUID(storageInstInfo.UUID),
			Life:             domainlife.Life(storageInstInfo.Life),
			Kind:             domainstorage.StorageKind(storageInstInfo.StorageKindID),
			RequestedSizeMIB: storageInstInfo.RequestedSizeMIB,
			StorageName:      storageInstInfo.StorageName,
		},

		UnitNamedStorageInfo: internal.UnitNamedStorageInfo{
			AlreadyAttachedCount: unitStorageNameInfo.AlreadyAttachedCount,
			CharmStorageDefinitionForValidation: internal.CharmStorageDefinitionForValidation{
				CountMin:    unitStorageNameInfo.StorageDefinitionCountMin,
				CountMax:    unitStorageNameInfo.StorageDefinitionCountMax,
				MinimumSize: unitStorageNameInfo.StorageDefinitionMinimumSize,
				Name:        unitStorageNameInfo.StorageDefinitionName,
				Type:        domainapplicationcharm.StorageType(unitStorageNameInfo.StorageDefinitionKind),
			},
			Name: coreunit.Name(unitStorageNameInfo.UnitName),
			UUID: coreunit.UUID(unitStorageNameInfo.UnitUUID),
		},
	}

	if unitStorageNameInfo.MachineUUID.Valid {
		retVal.UnitNamedStorageInfo.MachineUUID = new(
			coremachine.UUID(unitStorageNameInfo.MachineUUID.V))
	}

	if storageInstInfo.CharmName.Valid {
		retVal.StorageInstanceInfo.CharmName = new(storageInstInfo.CharmName.V)
	}

	if storageInstInfo.FilesystemUUID.Valid {
		retVal.StorageInstanceInfo.Filesystem = &internal.StorageInstanceFilesystemInfo{
			UUID: domainstorage.FilesystemUUID(storageInstInfo.FilesystemUUID.V),
			Size: storageInstInfo.FilesystemSizeMIB.V,
		}
	}
	if storageInstInfo.FilesystemOwnedMachineUUID.Valid {
		retVal.StorageInstanceInfo.Filesystem.OwningMachineUUID =
			new(coremachine.UUID(storageInstInfo.FilesystemOwnedMachineUUID.V))
	}
	if storageInstInfo.VolumeUUID.Valid {
		retVal.StorageInstanceInfo.Volume = &internal.StorageInstanceVolumeInfo{
			UUID: domainstorage.VolumeUUID(storageInstInfo.VolumeUUID.V),
			Size: storageInstInfo.VolumeSizeMIB.V,
		}
	}
	if storageInstInfo.VolumeOwnedMachineUUID.Valid {
		retVal.StorageInstanceInfo.Volume.OwningMachineUUID =
			new(coremachine.UUID(storageInstInfo.VolumeOwnedMachineUUID.V))
	}

	retVal.StorageInstanceAttachments = slices.Grow(
		retVal.StorageInstanceAttachments, len(storageInstAttachments))
	for _, unitAttachment := range storageInstAttachments {
		retVal.StorageInstanceAttachments = append(
			retVal.StorageInstanceAttachments,
			internal.StorageInstanceUnitAttachment{
				UnitUUID: coreunit.UUID(unitAttachment.UnitUUID),
				UUID:     domainstorage.StorageAttachmentUUID(unitAttachment.UUID),
			},
		)
	}

	return retVal, nil
}

// setK8sPodStatus saves the given k8s pod status, overwriting
// any current status data. If returns an error satisfying
// [applicationerrors.UnitNotFound] if the unit doesn't exist.
func (st *State) setK8sPodStatus(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID string,
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
