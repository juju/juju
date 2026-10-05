// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"

	coreapplication "github.com/juju/juju/core/application"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	corerelation "github.com/juju/juju/core/relation"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/deployment"
	internalcharm "github.com/juju/juju/domain/deployment/charm"
	"github.com/juju/juju/domain/network"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/relation/internal"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/errors"
)

// InsertIAASUnitState represents the application domain method which
// inserts an IAAS unit. Used here to insert subordinate units only.
type InsertIAASUnitState interface {
	InsertIAASUnit(
		ctx context.Context,
		tx *sqlair.TX,
		appUUID, charmUUID string,
		args application.AddIAASUnitArg,
	) (unit.Name, []machine.Name, error)
}

func (st *State) addSubordinateUnit(
	ctx context.Context,
	tx *sqlair.TX,
	relationUUID, relationUnitUUID, enteringUnitUUID string,
	storageArgs internal.SubordinateUnitStorageArgs,
) (internal.SubordinateUnitStatusHistoryData, error) {
	var empty internal.SubordinateUnitStatusHistoryData
	// Check that we are in a container scoped relation.
	scope, err := st.getRelationScope(ctx, tx, relationUUID)
	if err != nil {
		return empty, errors.Errorf("getting relation scope: %w", err)
	} else if scope != string(internalcharm.ScopeContainer) {
		// No subordinate unit is required.
		return empty, nil
	}

	// Get the ID of the related subordinate application, if it exists.
	subAppUUID, relatedSubExists, err := st.findRelatedSubordinateApplication(ctx, tx, relationUnitUUID)
	if err != nil {
		return empty, errors.Errorf("getting related subordinate application: %w", err)
	} else if !relatedSubExists {
		return empty, nil
	}

	// Resolve the principal unit that will host the new subordinate unit,
	// the machine it is placed on, and whether a subordinate unit will be
	// created at all. No subordinate unit is created if one already exists
	// for the resolved principal.
	principalUnitUUID, machineIdentifiers, createSubordinate, err := st.subordinateCreationTarget(
		ctx, tx, subAppUUID, enteringUnitUUID,
	)
	if err != nil {
		return empty, errors.Capture(err)
	} else if !createSubordinate {
		return empty, nil
	}

	// get principal unit's net node uuid.
	principalNetNodeUUID, err := st.getNetNodeUUID(ctx, tx, principalUnitUUID)
	if err != nil {
		return empty, errors.Errorf("getting principal unit net node uuid: %w", err)
	}
	unitUUID, err := unit.NewUUID()
	if err != nil {
		return empty, errors.Errorf("generating subordinate unit uuid: %w", err)
	}

	charmUUID, err := st.getCharmIDByApplicationUUID(ctx, tx, subAppUUID)
	if err != nil {
		return empty, errors.Errorf(
			"getting subordinate application %q charm uuid: %w",
			subAppUUID, err,
		)
	}

	unitStatus := st.makeIAASUnitStatusArgs()
	addUnitArg := application.AddIAASUnitArg{
		MachineNetNodeUUID: network.NetNodeUUID(machineIdentifiers.NetNodeUUID),
		MachineUUID:        machine.UUID(machineIdentifiers.UUID),
		AddUnitArg: application.AddUnitArg{
			UnitUUID: unitUUID,
			// Subordinate storage is attached to the machine's net node of
			// the principal unit, the same as any other IAAS unit.
			NetNodeUUID: principalNetNodeUUID,
			Placement: deployment.Placement{
				Type:      deployment.PlacementTypeMachine,
				Directive: machineIdentifiers.Name,
			},
			UnitStatusArg:        unitStatus,
			CreateUnitStorageArg: storageArgs.UnitStorageArgs,
		},
		CreateIAASUnitStorageArg: storageArgs.IAASUnitStorageArgs,
	}

	unitName, _, err := st.unitState.InsertIAASUnit(
		ctx, tx, subAppUUID, charmUUID, addUnitArg,
	)
	if err != nil {
		return empty, errors.Errorf("inserting new IAAS subordinate unit: %w", err)
	}

	// Record the principal/subordinate relationship.
	if err := st.recordUnitPrincipal(ctx, tx, principalUnitUUID, unitUUID.String()); err != nil {
		return empty, errors.Errorf("recording principal-subordinate relationship: %w", err)
	}

	return internal.SubordinateUnitStatusHistoryData{
		UnitName:   unitName.String(),
		UnitStatus: unitStatus,
	}, nil
}

func (st *State) makeIAASUnitStatusArgs() application.UnitStatusArg {
	now := new(st.clock.Now())
	return application.UnitStatusArg{
		AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
			Status: status.UnitAgentStatusAllocating,
			Since:  now,
		},
		WorkloadStatus: &status.StatusInfo[status.WorkloadStatusType]{
			Status:  status.WorkloadStatusWaiting,
			Message: corestatus.MessageWaitForMachine,
			Since:   now,
		},
	}
}

// GetSubordinateUnitCreationInfo returns the information required to make
// the storage arguments for a subordinate unit, and true, if entering scope
// of the given relation with the given unit would create one. If no
// subordinate unit would be created, false is returned.
//
// The checks are shared with [State.addSubordinateUnit], the authoritative
// in-transaction implementation, via [State.subordinateCreationTarget], so
// that the storage arguments for a new subordinate unit can be made before
// the unit enters scope and the two implementations cannot drift apart.
// The in-transaction checks win: the storage arguments are only consumed
// when a subordinate unit is actually created.
//
// The following errors may be expected:
//   - [relationerrors.RelationNotFound] if the relation does not exist.
//   - [relationerrors.CannotEnterScopeSubordinateNotAlive] if a subordinate
//     unit already exists, but is not alive.
//   - [applicationerrors.UnitNotFound] if the unit entering scope does not
//     exist.
//   - [applicationerrors.ApplicationIsDead] if the related subordinate
//     application is dead.
//   - [applicationerrors.ApplicationNotAlive] if the related subordinate
//     application is dying.
//   - [applicationerrors.UnitMachineNotAssigned] if the principal unit is
//     not assigned to a machine.
func (st *State) GetSubordinateUnitCreationInfo(
	ctx context.Context,
	relationUUID corerelation.UUID,
	unitName unit.Name,
) (internal.SubordinateUnitCreationInfo, bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return internal.SubordinateUnitCreationInfo{}, false, errors.Capture(err)
	}

	var (
		info              internal.SubordinateUnitCreationInfo
		createSubordinate bool
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		info = internal.SubordinateUnitCreationInfo{}
		createSubordinate = false

		// Only container scoped relations create subordinate units. Map
		// a missing relation to its typed error so that callers do not
		// have to match on raw sqlair errors.
		scope, err := st.getRelationScope(ctx, tx, relationUUID.String())
		if errors.Is(err, sqlair.ErrNoRows) {
			return relationerrors.RelationNotFound
		} else if err != nil {
			return errors.Errorf("getting relation scope: %w", err)
		} else if scope != string(internalcharm.ScopeContainer) {
			return nil
		}

		// Get the UUID of the unit entering scope.
		unitArgs := getUnit{Name: unitName}
		getUnitStmt, err := st.Prepare(`
SELECT &getUnit.*
FROM   unit
WHERE  name = $getUnit.name
`, unitArgs)
		if err != nil {
			return errors.Capture(err)
		}
		err = tx.Query(ctx, getUnitStmt, unitArgs).Get(&unitArgs)
		if errors.Is(err, sqlair.ErrNoRows) {
			return applicationerrors.UnitNotFound
		} else if err != nil {
			return errors.Capture(err)
		}

		// Get the UUID of the related subordinate application, if it exists.
		subAppUUID, relatedSubExists, err := st.getRelatedSubordinateApplication(
			ctx, tx, relationUUID.String(), unitArgs.UUID.String(),
		)
		if err != nil {
			return errors.Errorf("getting related subordinate application: %w", err)
		} else if !relatedSubExists {
			return nil
		}

		// Resolve the principal unit that would host a new subordinate
		// unit, the machine it would be placed on, and whether one would
		// be created at all. The checks are shared with
		// [State.addSubordinateUnit] so the pre-read cannot drift from the
		// in-transaction implementation.
		_, machineIdentifiers, wouldCreateSubordinate, err := st.subordinateCreationTarget(
			ctx, tx, subAppUUID, unitArgs.UUID.String(),
		)
		if err != nil {
			return errors.Capture(err)
		} else if !wouldCreateSubordinate {
			return nil
		}

		info = internal.SubordinateUnitCreationInfo{
			SubordinateApplicationUUID: coreapplication.UUID(subAppUUID),
			MachineNetNodeUUID:         network.NetNodeUUID(machineIdentifiers.NetNodeUUID),
		}
		createSubordinate = true
		return nil
	})
	if err != nil {
		return internal.SubordinateUnitCreationInfo{}, false, errors.Capture(err)
	}
	return info, createSubordinate, nil
}

// subordinateCreationTarget returns the UUID of the principal unit that a
// new subordinate unit of the given application would be attached to, the
// identifiers of the machine it would be placed on, and true, if entering
// scope with the given unit would create the subordinate unit. False is
// returned if a subordinate unit of the application already exists for the
// resolved principal, in which case no new subordinate unit is created.
//
// The entering unit is the principal unit of the new subordinate unit,
// unless the entering unit is itself a subordinate unit. This happens
// when two subordinate applications are related to each other in a
// container scoped relation. In that case, the new subordinate unit must
// be keyed to the principal of the entering unit. Keying it to the
// entering unit instead would make the relation-joined hook of the newly
// created unit spawn another unit of the other application, and so on
// without ever terminating.
//
// The subordinate unit is placed on the same machine as the principal
// unit, and its storage is attached to the machine's net node.
//
// This check sequence is shared by [State.addSubordinateUnit], the
// authoritative in-transaction implementation, and
// [State.GetSubordinateUnitCreationInfo], the pre-read used to make the
// storage arguments before entering scope, so the two cannot drift apart.
//
// The following errors may be expected:
//   - [relationerrors.CannotEnterScopeSubordinateNotAlive] if a subordinate
//     unit already exists, but is not alive.
//   - [applicationerrors.UnitMachineNotAssigned] if the principal unit is
//     not assigned to a machine.
func (st *State) subordinateCreationTarget(
	ctx context.Context,
	tx *sqlair.TX,
	subAppUUID, enteringUnitUUID string,
) (string, machineIdentifier, bool, error) {
	principalUnitUUID := enteringUnitUUID
	if principalUUID, found, err := st.getUnitPrincipalUUID(ctx, tx, enteringUnitUUID); err != nil {
		return "", machineIdentifier{}, false, errors.Errorf("getting principal unit of entering unit: %w", err)
	} else if found {
		principalUnitUUID = principalUUID
	}

	// Check if there is already a subordinate unit for the principal unit.
	// If there is, no new subordinate unit will be created.
	if exists, err := st.subordinateUnitExists(ctx, tx, subAppUUID, principalUnitUUID); err != nil {
		return "", machineIdentifier{}, false, errors.Errorf("checking if subordinate already exists: %w", err)
	} else if exists {
		return "", machineIdentifier{}, false, nil
	}

	machineIdentifiers, err := st.getUnitMachineIdentifier(
		ctx, tx, principalUnitUUID,
	)
	if err != nil {
		return "", machineIdentifier{}, false, errors.Errorf("getting principal unit machine information: %w", err)
	}

	return principalUnitUUID, machineIdentifiers, true, nil
}

// getUnitMachineIdentifier gets the identifiers of the machine that a unit is
// attached to.
//
// The following errors may be expected:
// - [applicationerrors.UnitNotFound] when the unit identified by the uuid no
// longer exists.
// - [applicationerrors.UnitMachineNotAssigned] when the unit is not assigned to
// a machine.
func (st *State) getUnitMachineIdentifier(
	ctx context.Context, tx *sqlair.TX, unitUUID string,
) (machineIdentifier, error) {
	var (
		input = entityUUID{UUID: unitUUID}
		dbVal machineIdentifier
	)

	q := `
SELECT (m.uuid, m.net_node_uuid, m.name) AS (&machineIdentifier.*)
FROM   unit AS u
JOIN   machine AS m ON u.net_node_uuid = m.net_node_uuid
WHERE  u.uuid = $entityUUID.uuid
`
	stmt, err := st.Prepare(q, input, dbVal)
	if err != nil {
		return machineIdentifier{}, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, input).Get(&dbVal)
	if errors.Is(err, sqlair.ErrNoRows) {
		// While we expect the caller had validated this statement we can still
		// provide a more helpful error message than sql error no rows.
		return machineIdentifier{}, errors.Errorf(
			"unit %q is not assigned to a machine in the model", unitUUID,
		).Add(applicationerrors.UnitMachineNotAssigned)
	} else if err != nil {
		return machineIdentifier{}, errors.Capture(err)
	}

	return dbVal, nil
}

// getUnitPrincipalUUID returns the UUID of the principal unit recorded for
// the given unit, and true, if the unit is a subordinate unit. If the unit
// has no principal unit, false is returned.
func (st *State) getUnitPrincipalUUID(
	ctx context.Context, tx *sqlair.TX, unitUUID string,
) (string, bool, error) {
	// unitPrincipalRow describes the projected principal_uuid column. The
	// input unit UUID is bound from a separate entityUUID so this struct only
	// carries the data a query populates.
	type unitPrincipalRow struct {
		PrincipalUUID string `db:"principal_uuid"`
	}
	stmt, err := st.Prepare(`
SELECT principal_uuid AS &unitPrincipalRow.principal_uuid
FROM   unit_principal
WHERE  unit_uuid = $entityUUID.uuid
`, unitPrincipalRow{}, entityUUID{})
	if err != nil {
		return "", false, errors.Capture(err)
	}

	arg := entityUUID{UUID: unitUUID}
	var row unitPrincipalRow
	err = tx.Query(ctx, stmt, arg).Get(&row)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", false, nil
	} else if err != nil {
		return "", false, errors.Capture(err)
	}

	return row.PrincipalUUID, true, nil
}

func (s *State) getCharmIDByApplicationUUID(ctx context.Context, tx *sqlair.TX, appID string) (string, error) {
	query := `
SELECT charm_uuid AS &entityUUID.uuid
FROM application
WHERE uuid = $entityUUID.uuid;
`
	stmt, err := s.Prepare(query, entityUUID{})
	if err != nil {
		return "", errors.Errorf("preparing query: %w", err)
	}
	var charmUUID entityUUID
	if err := tx.Query(ctx, stmt, entityUUID{UUID: appID}).Get(&charmUUID); errors.Is(err, sqlair.ErrNoRows) {
		return "", applicationerrors.ApplicationNotFound
	} else if err != nil {
		return "", errors.Errorf("getting charm ID by application UUID: %w", err)
	}

	return charmUUID.UUID, nil
}

// recordUnitPrincipal records a subordinate-principal relationship between
// units.
//
// It is expected that the caller has already verified that both unit uuids
// exist in the model.
func (st *State) recordUnitPrincipal(
	ctx context.Context,
	tx *sqlair.TX,
	principalUnitUUID, subordinateUnitUUID string,
) error {
	type unitPrincipal struct {
		PrincipalUUID   string `db:"principal_uuid"`
		SubordinateUUID string `db:"unit_uuid"`
	}
	arg := unitPrincipal{
		PrincipalUUID:   principalUnitUUID,
		SubordinateUUID: subordinateUnitUUID,
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

// findRelatedSubordinateApplication returns the application UUID of the
// related subordinate application there is one and it is alive, if there
// is not, it returns false as the boolean argument.
func (st *State) findRelatedSubordinateApplication(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID string,
) (string, bool, error) {
	type getSub struct {
		UnitUUID      string `db:"unit_uuid"`
		Subordinate   bool   `db:"subordinate"`
		ApplicationID string `db:"application_uuid"`
		Life          string `db:"value"`
	}

	arg := getSub{
		UnitUUID: unitUUID,
	}
	stmt, err := st.Prepare(`
SELECT (cm.subordinate, ae.application_uuid, l.value) AS (&getSub.*)
FROM   relation_unit AS ru
JOIN   relation_endpoint AS re1 ON ru.relation_endpoint_uuid = re1.uuid
JOIN   relation_endpoint AS re2 ON re1.relation_uuid = re2.relation_uuid AND re1.uuid != re2.uuid 
JOIN   application_endpoint AS ae ON re2.endpoint_uuid = ae.uuid
JOIN   charm_relation AS cr ON ae.charm_relation_uuid = cr.uuid
JOIN   charm_metadata AS cm ON cr.charm_uuid = cm.charm_uuid
JOIN   application AS a ON ae.application_uuid = a.uuid
JOIN   life AS l ON a.life_id = l.id
WHERE  ru.uuid = $getSub.unit_uuid
`, arg)
	if err != nil {
		return "", false, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, arg).Get(&arg)
	if errors.Is(err, sqlair.ErrNoRows) {
		// Peer relations will return no rows, so will units not in relations.
		// Return false for these.
		return "", false, applicationerrors.ApplicationNotFound
	}
	if err != nil {
		return "", false, errors.Capture(err)
	}

	switch arg.Life {
	case string(corelife.Dead):
		return "", false, applicationerrors.ApplicationIsDead
	case string(corelife.Dying):
		return "", false, applicationerrors.ApplicationNotAlive
	}

	return arg.ApplicationID, arg.Subordinate, nil
}

// getRelatedSubordinateApplication returns the application UUID of the
// subordinate application related to the given relation, relative to the
// application of the given unit, if it exists and is alive. False is
// returned if there is no such application, or if a related application
// exists but is not a subordinate.
//
// Unlike [State.findRelatedSubordinateApplication], the related application
// is resolved via the application of the given unit itself, so the unit does
// not need to have entered scope of the relation yet. Also unlike the
// original, no error is returned when there is no related application: a
// nil error is returned with false, where the original returns
// [applicationerrors.ApplicationNotFound].
func (st *State) getRelatedSubordinateApplication(
	ctx context.Context,
	tx *sqlair.TX,
	relationUUID, unitUUID string,
) (string, bool, error) {
	type relatedSubordinateAppRow struct {
		Subordinate   bool   `db:"subordinate"`
		ApplicationID string `db:"application_uuid"`
		Life          string `db:"value"`
	}
	type relatedSubordinateAppQuery struct {
		RelationUUID string `db:"relation_uuid"`
		UnitUUID     string `db:"unit_uuid"`
	}

	arg := relatedSubordinateAppQuery{
		RelationUUID: relationUUID,
		UnitUUID:     unitUUID,
	}
	stmt, err := st.Prepare(`
SELECT (cm.subordinate, ae2.application_uuid, l.value) AS (&relatedSubordinateAppRow.*)
FROM   relation_endpoint AS re1
JOIN   relation_endpoint AS re2
       ON re1.relation_uuid = re2.relation_uuid AND re1.uuid != re2.uuid
JOIN   application_endpoint AS ae1 ON re1.endpoint_uuid = ae1.uuid
JOIN   application_endpoint AS ae2 ON re2.endpoint_uuid = ae2.uuid
JOIN   charm_relation AS cr ON ae2.charm_relation_uuid = cr.uuid
JOIN   charm_metadata AS cm ON cr.charm_uuid = cm.charm_uuid
JOIN   application AS a ON ae2.application_uuid = a.uuid
JOIN   life AS l ON a.life_id = l.id
JOIN   unit AS u ON u.application_uuid = ae1.application_uuid
WHERE  re1.relation_uuid = $relatedSubordinateAppQuery.relation_uuid
AND    u.uuid = $relatedSubordinateAppQuery.unit_uuid
`, relatedSubordinateAppRow{}, arg)
	if err != nil {
		return "", false, errors.Capture(err)
	}

	var sub relatedSubordinateAppRow
	err = tx.Query(ctx, stmt, arg).Get(&sub)
	if errors.Is(err, sqlair.ErrNoRows) {
		// Peer relations will return no rows, as will relations where the
		// unit's application has no endpoint.
		return "", false, nil
	} else if err != nil {
		return "", false, errors.Capture(err)
	}

	switch sub.Life {
	case string(corelife.Dead):
		return "", false, applicationerrors.ApplicationIsDead
	case string(corelife.Dying):
		return "", false, applicationerrors.ApplicationNotAlive
	}

	return sub.ApplicationID, sub.Subordinate, nil
}

// subordinateUnitExists checks if the principal unit already has a subordinate
// unit of the given application.
//
// If the subordinate unit exists but is not alive
// [relationerrors.CannotEnterScopeSubordinateNotAlive] is returned.
func (st *State) subordinateUnitExists(
	ctx context.Context,
	tx *sqlair.TX,
	subordinateAppID, principalUnit string,
) (bool, error) {
	type getSub struct {
		PrincipalUnitUUID        string `db:"unit_uuid"`
		SubordinateApplicationID string `db:"application_uuid"`
		SubordinateLife          string `db:"value"`
	}
	arg := getSub{
		PrincipalUnitUUID:        principalUnit,
		SubordinateApplicationID: subordinateAppID,
	}
	stmt, err := st.Prepare(`
SELECT (u.application_uuid, l.value) AS (&getSub.*)
FROM   unit_principal AS up
JOIN   unit AS u ON up.unit_uuid = u.uuid
JOIN   life AS l ON u.life_id = l.id
WHERE  u.application_uuid = $getSub.application_uuid
AND    up.principal_uuid  = $getSub.unit_uuid
`, arg)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, arg).Get(&arg)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}

	if arg.SubordinateLife != string(corelife.Alive) {
		return false, relationerrors.CannotEnterScopeSubordinateNotAlive
	}

	return true, nil
}
func (st *State) getNetNodeUUID(ctx context.Context, tx *sqlair.TX, unitUUID string) (network.NetNodeUUID, error) {
	var (
		input = entityUUID{UUID: unitUUID}
		dbVal netNodeUUID
	)

	stmt, err := st.Prepare(`
SELECT &netNodeUUID.*
FROM   unit
WHERE  uuid = $entityUUID.uuid
`, input, dbVal)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, input).Get(&dbVal)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf(
			"unit with uuid %q does not exist", unitUUID,
		).Add(applicationerrors.UnitNotFound)
	} else if err != nil {
		return "", errors.Capture(err)
	}

	return network.NetNodeUUID(dbVal.NetNodeUUID), nil
}
