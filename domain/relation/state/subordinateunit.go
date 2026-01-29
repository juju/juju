// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"

	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
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
	) (unit.Name, unit.UUID, []machine.Name, error)
}

func (st *State) addSubordinateUnit(
	ctx context.Context,
	tx *sqlair.TX,
	relationUUID, relationUnitUUID, principalUnitUUID string,
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

	// Check if there is already a subordinate unit.
	if exists, err := st.subordinateUnitExists(ctx, tx, subAppUUID, principalUnitUUID); err != nil {
		return empty, errors.Errorf("checking if subordinate already exists: %w", err)
	} else if exists {
		return empty, nil
	}

	// get principal unit's net node uuid.
	principalNetNodeUUID, err := st.getNetNodeUUID(ctx, tx, principalUnitUUID)
	if err != nil {
		return empty, errors.Errorf("getting principal unit net node uuid: %w", err)
	}

	charmUUID, err := st.getCharmIDByApplicationUUID(ctx, tx, subAppUUID)
	if err != nil {
		return empty, errors.Errorf(
			"getting subordinate application %q charm uuid: %w",
			subAppUUID, err,
		)
	}

	// Place the subordinate on the same machine as the principal unit.
	machineIdentifiers, err := st.getUnitMachineIdentifier(
		ctx, tx, principalUnitUUID,
	)
	if err != nil {
		return empty, errors.Errorf("getting principal unit machine information: %w", err)
	}

	unitStatus := st.makeIAASUnitStatusArgs()
	addUnitArg := application.AddIAASUnitArg{
		MachineNetNodeUUID: network.NetNodeUUID(machineIdentifiers.NetNodeUUID),
		MachineUUID:        machine.UUID(machineIdentifiers.UUID),
		AddUnitArg: application.AddUnitArg{
			// TODO: storage for subordinate units.
			NetNodeUUID: principalNetNodeUUID,
			Placement: deployment.Placement{
				Type:      deployment.PlacementTypeMachine,
				Directive: machineIdentifiers.Name,
			},
			UnitStatusArg: unitStatus,
		},
	}

	unitName, unitUUID, _, err := st.unitState.InsertIAASUnit(
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
	now := ptr(st.clock.Now())
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

func ptr[T any](v T) *T {
	return &v
}
