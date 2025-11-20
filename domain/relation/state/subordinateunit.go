// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/charm"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/errors"
)

// SubordinateUnitState represents the minium state required to insert a
// subordinate unit.
type SubordinateUnitState struct {
	*domain.StateBase
	clock  clock.Clock
	logger logger.Logger
	us     *applicationstate.InsertIAASUnitState
}

// AddIAASSubordinateUnit adds a unit to the specified subordinate application
// to the IAAS application on the same machine as the given principal unit and
// records the principal-subordinate relationship.
//
// The following error types can be expected:
//   - [applicationerrors.ApplicationNotFound] when the subordinate application
//     cannot be found.
//   - [applicationerrors.UnitNotFound] when the principal unit cannot be found.
//   - [machineerrors.MachineNotFound] when no machine is attached to the
//
// principal unit.
func (st *SubordinateUnitState) AddIAASSubordinateUnit(
	ctx context.Context,
	arg application.SubordinateUnitArg,
) (unit.Name, []machine.Name, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", nil, errors.Capture(err)
	}

	var (
		unitName     unit.Name
		unitUUID     unit.UUID
		machineNames []machine.Name
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Check the application is alive.
		if err := st.checkApplicationAlive(ctx, tx, arg.SubordinateAppID); err != nil {
			return errors.Capture(err)
		}
		if err := st.checkUnitNotDead(ctx, tx, arg.PrincipalUnitUUID); err != nil {
			return errors.Capture(err)
		}

		// Check this unit does not already have a subordinate unit from this
		// application.
		err := st.checkNoSubordinateExists(ctx, tx, arg.SubordinateAppID, arg.PrincipalUnitUUID)
		if err != nil {
			return errors.Errorf("checking if subordinate already exists: %w", err)
		}
		charmUUID, err := st.getCharmIDByApplicationUUID(ctx, tx, arg.SubordinateAppID)
		if err != nil {
			return errors.Errorf(
				"getting subordinate application %q charm uuid: %w",
				arg.SubordinateAppID, err,
			)
		}

		// Place the subordinate on the same machine as the principal unit.
		machineIdentifiers, err := st.getUnitMachineIdentifiers(
			ctx, tx, arg.PrincipalUnitUUID,
		)
		if err != nil {
			return errors.Errorf("getting principal unit machine information: %w", err)
		}

		addUnitArg := application.AddIAASUnitArg{
			MachineNetNodeUUID: network.NetNodeUUID(machineIdentifiers.NetNodeUUID),
			MachineUUID:        machine.UUID(machineIdentifiers.UUID),
			AddUnitArg: application.AddUnitArg{
				CreateUnitStorageArg: arg.CreateUnitStorageArg,
				NetNodeUUID:          arg.NetNodeUUID,
				Placement: deployment.Placement{
					Type:      deployment.PlacementTypeMachine,
					Directive: machineIdentifiers.Name,
				},
				UnitStatusArg: arg.UnitStatusArg,
			},
		}

		unitName, unitUUID, machineNames, err = st.us.InsertIAASUnit(
			ctx, tx, arg.SubordinateAppID, charmUUID, addUnitArg,
		)
		if err != nil {
			return errors.Errorf("inserting new IAAS subordinate unitq: %w", err)
		}

		// Record the principal/subordinate relationship.
		if err := st.recordUnitPrincipal(ctx, tx, arg.PrincipalUnitUUID, unitUUID); err != nil {
			return errors.Errorf("recording principal-subordinate relationship: %w", err)
		}

		return nil
	})
	if err != nil {
		return "", nil, errors.Capture(err)
	}

	return unitName, machineNames, nil
}

// getUnitMachineIdentifiers gets the identifiers of the machine that a unit is
// attached to.
//
// The following errors may be expected:
// - [applicationerrors.UnitNotFound] when the unit identified by the uuid no
// longer exists.
// - [applicationerrors.UnitMachineNotAssigned] when the unit is not assigned to
// a machine.
func (st *SubordinateUnitState) getUnitMachineIdentifiers(
	ctx context.Context, tx *sqlair.TX, unitUUID unit.UUID,
) (machineIdentifiers, error) {
	var (
		input = entityUUID{UUID: unitUUID.String()}
		dbVal machineIdentifiers
	)

	q := `
SELECT (m.uuid, m.net_node_uuid, m.name) AS (&machineIdentifiers.*)
FROM   unit u
JOIN   machine AS m ON u.net_node_uuid = m.net_node_uuid
WHERE  u.uuid = $entityUUID.uuid
`
	stmt, err := st.Prepare(q, input, dbVal)
	if err != nil {
		return machineIdentifiers{}, errors.Capture(err)
	}

	// Note: can be done via subordinateUnitExists in relation domain.
	var exists bool
	//exists, err := st.checkUnitExists(ctx, tx, unitUUID)
	//if err != nil {
	//	return internalapplication.machineIdentifiers{}, errors.Errorf(
	//		"checking unit %q exists", unitUUID,
	//	)
	//}

	if !exists {
		return machineIdentifiers{}, errors.Errorf(
			"unit %q does not exist", unitUUID,
		).Add(applicationerrors.UnitNotFound)
	}

	err = tx.Query(ctx, stmt, input).Get(&dbVal)
	if errors.Is(err, sqlair.ErrNoRows) {
		// While we expect the caller had validated this statement we can still
		// provide a more helpful error message than sql error no rows.
		return machineIdentifiers{}, errors.Errorf(
			"unit %q is not assigned to a machine in the model", unitUUID,
		).Add(applicationerrors.UnitMachineNotAssigned)
	} else if err != nil {
		return machineIdentifiers{}, errors.Capture(err)
	}

	return dbVal, nil
}

// checkNoSubordinateExists returns
// [applicationerrors.UnitAlreadyHasSubordinate] if the specified unit already
// has a subordinate for the given application.
func (st *SubordinateUnitState) checkNoSubordinateExists(
	ctx context.Context,
	tx *sqlair.TX,
	subordinateAppUUID coreapplication.UUID,
	principalUnitUUID unit.UUID,
) error {
	var (
		sAppUUID  = applicationUUID{UUID: subordinateAppUUID.String()}
		pUnitUUID = entityUUID{UUID: principalUnitUUID.String()}
	)

	stmt, err := st.Prepare(`
SELECT pu.uuid AS &entityUUID.uuid
FROM   unit pu
JOIN   unit_principal up ON up.principal_uuid = pu.uuid
JOIN   unit su ON su.uuid = up.unit_uuid
WHERE  pu.uuid = $entityUUID.uuid
AND    su.application_uuid = $applicationUUID.application_uuid
`,
		sAppUUID, pUnitUUID,
	)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, sAppUUID, pUnitUUID).Get(&pUnitUUID)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil
	} else if err != nil {
		return errors.Capture(err)
	}

	return applicationerrors.UnitAlreadyHasSubordinate
}

// checkUnitNotDead checks if the unit exists and is not dead. It's possible to
// access alive and dying units, but not dead ones:
// - If the unit is not found, [applicationerrors.UnitNotFound] is returned.
// - If the unit is dead, [applicationerrors.UnitIsDead] is returned.
func (st *SubordinateUnitState) checkUnitNotDead(ctx context.Context, tx *sqlair.TX, uuid unit.UUID) error {
	query := `
SELECT (uuid, value) AS (&getLife.*)
FROM unit
JOIN life ON unit.life_id = life.id
WHERE uuid = $getLife.uuid;
`
	input := entityUUID{UUID: uuid.String()}
	stmt, err := st.Prepare(query, input, getLife{})
	if err != nil {
		return errors.Errorf("preparing query for unit %q: %w", uuid, err)
	}

	var result getLife
	err = tx.Query(ctx, stmt, input).Get(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return applicationerrors.UnitNotFound
	} else if err != nil {
		return errors.Errorf("getting unit %q life: %w", uuid, err)
	}

	switch result.Life {
	case corelife.Dead:
		return applicationerrors.UnitIsDead
	default:
		return nil
	}
}

func (s *SubordinateUnitState) getCharmIDByApplicationUUID(ctx context.Context, tx *sqlair.TX, appID coreapplication.UUID) (charm.ID, error) {
	query := `
SELECT charm_uuid AS &charmUUID.*
FROM application
WHERE uuid = $entityUUID.uuid;
`
	stmt, err := s.Prepare(query, entityUUID{})
	if err != nil {
		return "", errors.Errorf("preparing query: %w", err)
	}
	var charmUUID entityUUID
	if err := tx.Query(ctx, stmt, entityUUID{UUID: appID.String()}).Get(&charmUUID); errors.Is(err, sqlair.ErrNoRows) {
		return "", applicationerrors.ApplicationNotFound
	} else if err != nil {
		return "", errors.Errorf("getting charm ID by application UUID: %w", err)
	}

	return charm.ParseID(charmUUID.UUID)
}

// recordUnitPrincipal records a subordinate-principal relationship between
// units.
//
// It is expected that the caller has already verified that both unit uuids
// exist in the model.
func (st *SubordinateUnitState) recordUnitPrincipal(
	ctx context.Context,
	tx *sqlair.TX,
	principalUnitUUID, subordinateUnitUUID unit.UUID,
) error {
	type unitPrincipal struct {
		PrincipalUUID   string `db:"principal_uuid"`
		SubordinateUUID string `db:"unit_uuid"`
	}
	arg := unitPrincipal{
		PrincipalUUID:   principalUnitUUID.String(),
		SubordinateUUID: subordinateUnitUUID.String(),
	}
	stmt, err := st.Prepare(`
INSERT INTO unit_principal (*)
VALUES ($unitPrincipal.*)
`, arg)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, arg).Run()
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// checkApplicationAlive checks if the application exists and it is alive.
func (st *SubordinateUnitState) checkApplicationAlive(ctx context.Context, tx *sqlair.TX, appUUID coreapplication.UUID) error {
	type life struct {
		LifeID corelife.Value `db:"value"`
	}

	ident := entityUUID{UUID: appUUID.String()}
	query := `
SELECT &life.*
FROM   application AS a
JOIN   life as l ON a.life_id = l.id
WHERE  a.uuid = $ident.uuid
`
	stmt, err := st.Prepare(query, ident, life{})
	if err != nil {
		return errors.Errorf("preparing query for application %q: %w", ident.UUID, err)
	}

	var result life
	err = tx.Query(ctx, stmt, ident).Get(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return applicationerrors.ApplicationNotFound
	} else if err != nil {
		return errors.Errorf("checking application %q exists: %w", ident.UUID, err)
	}

	switch result.LifeID {
	case corelife.Dead:
		return applicationerrors.ApplicationIsDead
	case corelife.Dying:
		return applicationerrors.ApplicationNotAlive
	default:
		return nil
	}
}

// checkUnitExists checks if the unit with the given UUID exists in the model.
// True is returned when the unit is found.
func (st *SubordinateUnitState) checkUnitExists(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID unit.UUID,
) (bool, error) {
	uuidInput := entityUUID{UUID: unitUUID.String()}

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
