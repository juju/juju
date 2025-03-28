// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	corerelation "github.com/juju/juju/core/relation"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

type State struct {
	*domain.StateBase
	clock  clock.Clock
	logger logger.Logger
}

// NewState returns a new state reference.
func NewState(factory database.TxnRunnerFactory, clock clock.Clock, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		clock:     clock,
		logger:    logger,
	}
}

// GetAllRelationDetails return all uuid of all relation for the current model.
func (st *State) GetAllRelationDetails(ctx context.Context) ([]relation.RelationDetailsResult, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	var relationsDetails []relation.RelationDetailsResult
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		relations, err := st.getAllRelations(ctx, tx)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting all relations: %w", err)
		}

		for _, rel := range relations {
			details, err := st.getRelationDetails(ctx, tx, rel.ID)
			if err != nil {
				return errors.Errorf("getting relation details: %w", err)
			}
			relationsDetails = append(relationsDetails, details)
		}
		return nil
	})
	return relationsDetails, errors.Capture(err)
}

// GetAllRelationStatuses returns all the relation statuses of the given model.
func (st *State) GetAllRelationStatuses(ctx context.Context) (map[corerelation.UUID]corestatus.StatusInfo, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	relationsStatuses := make(map[corerelation.UUID]corestatus.StatusInfo)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		relations, err := st.getAllRelations(ctx, tx)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting all relations: %w", err)
		}

		for _, rel := range relations {
			relationStatus, err := st.getRelationStatus(ctx, tx, rel.UUID)
			if err != nil {
				return errors.Errorf("getting relation status: %w", err)
			}
			relationsStatuses[rel.UUID] = relationStatus
		}
		return nil
	})
	return relationsStatuses, errors.Capture(err)
}

// GetRelationID returns the relation ID for the given relation UUID.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationNotFound] is returned if the relation UUID
//     is not found.
func (st *State) GetRelationID(ctx context.Context, relationUUID corerelation.UUID) (int, error) {
	db, err := st.DB()
	if err != nil {
		return 0, errors.Capture(err)
	}

	id := relationIDAndUUID{
		UUID: relationUUID,
	}
	stmt, err := st.Prepare(`
SELECT &relationIDAndUUID.relation_id
FROM   relation
WHERE  uuid = $relationIDAndUUID.uuid
`, id)
	if err != nil {
		return 0, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, id).Get(&id)
		if errors.Is(err, sqlair.ErrNoRows) {
			return relationerrors.RelationNotFound
		}
		return err
	})
	if err != nil {
		return 0, errors.Capture(err)
	}

	return id.ID, nil
}

// GetRelationUUIDByID returns the relation UUID based on the relation ID.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationNotFound] is returned if the relation UUID
//     relating to the relation ID cannot be found.
func (st *State) GetRelationUUIDByID(ctx context.Context, relationID int) (corerelation.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	id := relationIDAndUUID{
		ID: relationID,
	}
	stmt, err := st.Prepare(`
SELECT &relationIDAndUUID.uuid
FROM   relation
WHERE  relation_id = $relationIDAndUUID.relation_id
`, id)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, id).Get(&id)
		if errors.Is(err, sqlair.ErrNoRows) {
			return relationerrors.RelationNotFound
		}
		return err
	})
	if err != nil {
		return "", errors.Capture(err)
	}

	return corerelation.UUID(id.UUID), nil
}

// GetRelationEndpointUUID retrieves the endpoint UUID of a given relation
// for a specific application.
// It queries the database using the provided application ID and relation UUID
// arguments.
//
// The following error types can be expected to be returned:
//   - [relationerrors.ApplicationNotFound] is returned if the application
//     is not found.
//   - [relationerrors.RelationEndpointNotFound] is returned if the relation
//     Endpoint is not found.
//   - [relationerrors.RelationNotFound] is returned if the relation UUID
//     is not found.
func (st *State) GetRelationEndpointUUID(ctx context.Context, args relation.GetRelationEndpointUUIDArgs) (
	corerelation.EndpointUUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	type relationEndpointUUID struct {
		UUID string `db:"uuid"`
	}
	type relationEndpointArgs struct {
		AppID        string `db:"application_uuid"`
		RelationUUID string `db:"relation_uuid"`
	}
	dbArgs := relationEndpointArgs{
		AppID:        args.ApplicationID.String(),
		RelationUUID: args.RelationUUID.String(),
	}
	stmt, err := st.Prepare(`
SELECT re.uuid AS &relationEndpointUUID.uuid
FROM   relation_endpoint re
JOIN   application_endpoint ae ON re.endpoint_uuid = ae.uuid
WHERE  ae.application_uuid = $relationEndpointArgs.application_uuid
AND    re.relation_uuid = $relationEndpointArgs.relation_uuid
`, relationEndpointUUID{}, dbArgs)
	if err != nil {
		return "", errors.Capture(err)
	}
	var relationEndpoint relationEndpointUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, dbArgs).Get(&relationEndpoint)
		if errors.Is(err, sqlair.ErrNoRows) {
			// Check if it is a missing application.
			appFound, err := checkExistsByUUID(ctx, st, tx, "application", args.ApplicationID.String())
			if err != nil {
				return errors.Capture(err)
			}
			// Check if the relation exists.
			relationFound, err := checkExistsByUUID(ctx, st, tx, "relation", args.RelationUUID.String())
			if err != nil {
				return errors.Capture(err)
			}
			var errs []error
			if !appFound {
				errs = append(errs, errors.Errorf("%w: %s", relationerrors.ApplicationNotFound, args.ApplicationID))
			}
			if !relationFound {
				errs = append(errs, errors.Errorf("%w: %s", relationerrors.RelationNotFound, args.RelationUUID))
			}
			if len(errs) > 0 {
				return errors.Join(errs...)
			}
			return errors.Errorf("relationUUID %q with applicationID %q: %w",
				args.RelationUUID, args.ApplicationID, relationerrors.RelationEndpointNotFound)

		}
		return errors.Capture(err)
	})

	return corerelation.EndpointUUID(relationEndpoint.UUID), errors.Capture(err)
}

// GetRelationsStatusForUnit returns RelationUnitStatus for any relation the
// unit is part of.
func (st *State) GetRelationsStatusForUnit(
	ctx context.Context,
	unitUUID unit.UUID,
) ([]relation.RelationUnitStatusResult, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	type relationUnitStatus struct {
		RelationUUID string `db:"relation_uuid"`
		InScope      bool   `db:"in_scope"`
		Status       string `db:"status"`
	}
	type unitUUIDArg struct {
		UUID string `db:"unit_uuid"`
	}

	uuid := unitUUIDArg{
		UUID: unitUUID.String(),
	}

	stmt, err := st.Prepare(`
SELECT (ru.relation_uuid, ru.in_scope, vrs.status) AS (&relationUnitStatus.*)
FROM   relation_unit ru
JOIN   v_relation_status vrs ON ru.relation_uuid = vrs.relation_uuid
WHERE  ru.unit_uuid = $unitUUIDArg.unit_uuid
`, uuid, relationUnitStatus{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var relationUnitStatuses []relation.RelationUnitStatusResult
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var statuses []relationUnitStatus
		err := tx.Query(ctx, stmt, uuid).GetAll(&statuses)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}

		for _, status := range statuses {
			endpoints, err := st.getEndpoints(ctx, tx, corerelation.UUID(status.RelationUUID))
			if err != nil {
				return errors.Errorf("getting endpoints of relation %q: %w", status.RelationUUID, err)
			}

			relationUnitStatuses = append(relationUnitStatuses, relation.RelationUnitStatusResult{
				Endpoints: endpoints,
				InScope:   status.InScope,
				Suspended: status.Status == corestatus.Suspended.String(),
			})
		}

		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return relationUnitStatuses, nil
}

// GetRelationEndpoints retrieves the endpoints of a given relation specified via its UUID.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationNotFound] is returned if the relation UUID is not
//     found.
func (st *State) GetRelationEndpoints(ctx context.Context, uuid corerelation.UUID) ([]relation.Endpoint, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	var endpoints []relation.Endpoint
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		endpoints, err = st.getEndpoints(ctx, tx, uuid)
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return endpoints, nil
}

// getEndpoints retrieves the endpoints of the specified relation.
func (st *State) getEndpoints(
	ctx context.Context,
	tx *sqlair.TX,
	uuid corerelation.UUID,
) ([]relation.Endpoint, error) {
	id := relationUUID{
		UUID: uuid.String(),
	}
	stmt, err := st.Prepare(`
SELECT &endpoint.*
FROM   v_relation_endpoint
WHERE  relation_uuid = $relationUUID.uuid
`, id, endpoint{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var endpoints []endpoint
	err = tx.Query(ctx, stmt, id).GetAll(&endpoints)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, relationerrors.RelationNotFound
	}

	if length := len(endpoints); length > 2 {
		return nil, errors.Errorf("internal error: expected 1 or 2 endpoints in relation, got %d", length)
	}

	var relationEndpoints []relation.Endpoint
	for _, e := range endpoints {
		relationEndpoints = append(relationEndpoints, e.toRelationEndpoint())
	}

	return relationEndpoints, nil
}

// GetRegularRelationUUIDByEndpointIdentifiers gets the UUID of a regular
// relation specified by two endpoint identifiers.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationNotFound] is returned if endpoints cannot be
//     found.
func (st *State) GetRegularRelationUUIDByEndpointIdentifiers(
	ctx context.Context,
	endpoint1, endpoint2 relation.EndpointIdentifier,
) (corerelation.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	type endpointIdentifier1 endpointIdentifier
	type endpointIdentifier2 endpointIdentifier
	e1 := endpointIdentifier1{
		ApplicationName: endpoint1.ApplicationName,
		EndpointName:    endpoint1.EndpointName,
	}
	e2 := endpointIdentifier2{
		ApplicationName: endpoint2.ApplicationName,
		EndpointName:    endpoint2.EndpointName,
	}

	stmt, err := st.Prepare(`
SELECT &relationUUID.*
FROM   relation r
JOIN   v_relation_endpoint_identifier e1 ON r.uuid = e1.relation_uuid
JOIN   v_relation_endpoint_identifier e2 ON r.uuid = e2.relation_uuid
WHERE  e1.application_name = $endpointIdentifier1.application_name 
AND    e1.endpoint_name    = $endpointIdentifier1.endpoint_name
AND    e2.application_name = $endpointIdentifier2.application_name 
AND    e2.endpoint_name    = $endpointIdentifier2.endpoint_name
`, relationUUID{}, e1, e2)
	if err != nil {
		return "", errors.Capture(err)
	}

	var uuid []relationUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, e1, e2).GetAll(&uuid)
		if errors.Is(err, sqlair.ErrNoRows) {
			return relationerrors.RelationNotFound
		}
		return errors.Capture(err)
	})
	if err != nil {
		return "", errors.Capture(err)
	}

	if len(uuid) > 1 {
		return "", errors.Errorf("found multiple relations for endpoint pair")
	}

	return corerelation.UUID(uuid[0].UUID), nil
}

// GetPeerRelationUUIDByEndpointIdentifiers gets the UUID of a peer
// relation specified by a single endpoint identifier.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationNotFound] is returned if endpoint cannot be
//     found.
func (st *State) GetPeerRelationUUIDByEndpointIdentifiers(
	ctx context.Context,
	endpoint relation.EndpointIdentifier,
) (corerelation.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	e := endpointIdentifier{
		ApplicationName: endpoint.ApplicationName,
		EndpointName:    endpoint.EndpointName,
	}

	stmt, err := st.Prepare(`
SELECT &relationUUIDAndRole.*
FROM   relation r
JOIN   v_relation_endpoint e ON r.uuid = e.relation_uuid
WHERE  e.application_name = $endpointIdentifier.application_name 
AND    e.endpoint_name    = $endpointIdentifier.endpoint_name
`, relationUUIDAndRole{}, e)
	if err != nil {
		return "", errors.Capture(err)
	}

	var uuidAndRole []relationUUIDAndRole
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, e).GetAll(&uuidAndRole)
		if errors.Is(err, sqlair.ErrNoRows) {
			return relationerrors.RelationNotFound
		}
		return errors.Capture(err)
	})
	if err != nil {
		return "", errors.Capture(err)
	}

	if len(uuidAndRole) > 1 {
		return "", errors.Errorf("found multiple relations for peer application endpoint combination")
	}

	// Verify that the role is peer. Endpoint names are unique per charm, so if
	// the role is not peer the application does not have a peer relation with
	// the specified endpoint name, so return RelationNotFound.
	if uuidAndRole[0].Role != string(charm.RolePeer) {
		return "", relationerrors.RelationNotFound
	}

	return corerelation.UUID(uuidAndRole[0].UUID), nil
}

// GetRelationDetails returns relation details for the given relationID.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationNotFound] is returned if the relation UUID
//     is not found.
func (st *State) GetRelationDetails(ctx context.Context, relationID int) (relation.RelationDetailsResult, error) {
	db, err := st.DB()
	var result relation.RelationDetailsResult
	if err != nil {
		return result, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result, err = st.getRelationDetails(ctx, tx, relationID)
		return errors.Capture(err)
	})
	return result, errors.Capture(err)
}

// WatcherApplicationSettingsNamespace returns the namespace string used for
// tracking application settings in the database.
func (st *State) WatcherApplicationSettingsNamespace() string {
	return "relation_application_setting"
}

// checkExistsByUUID checks if a record with the specified UUID exists in the given
// table using a transaction and context.
func checkExistsByUUID(ctx context.Context, st *State, tx *sqlair.TX, table string, uuid string) (bool,
	error) {
	type search struct {
		UUID string `db:"uuid"`
	}

	searched := search{UUID: uuid}
	query := fmt.Sprintf(`
SELECT &search.* 
FROM   %s 
WHERE  uuid = $search.uuid
`, table)
	checkStmt, err := st.Prepare(query, searched)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, checkStmt, searched).Get(&searched)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Errorf("query %q: %w", query, err)
	}
	return true, nil
}

// getAllRelations retrieves all relations from the database, returning a slice of relationIDAndUUID or an error.
func (st *State) getAllRelations(ctx context.Context, tx *sqlair.TX) ([]relationIDAndUUID, error) {
	var result []relationIDAndUUID

	stmt, err := st.Prepare(`
SELECT &relationIDAndUUID.*
FROM   relation
`, relationIDAndUUID{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	err = tx.Query(ctx, stmt).GetAll(&result)

	return result, errors.Capture(err)
}

func (st *State) getRelationDetails(ctx context.Context, tx *sqlair.TX, relationID int) (relation.RelationDetailsResult, error) {
	rel := relationForDetails{
		ID: relationID,
	}
	stmt, err := st.Prepare(`
SELECT (r.uuid, r.relation_id, l.value) AS (&relationForDetails.*)
FROM   relation r
JOIN   life l ON r.life_id = l.id
WHERE  relation_id = $relationForDetails.relation_id
`, rel)
	if err != nil {
		return relation.RelationDetailsResult{}, errors.Capture(err)
	}

	var endpoints []relation.Endpoint
	err = tx.Query(ctx, stmt, rel).Get(&rel)
	if errors.Is(err, sqlair.ErrNoRows) {
		return relation.RelationDetailsResult{}, relationerrors.RelationNotFound
	} else if err != nil {
		return relation.RelationDetailsResult{}, errors.Capture(err)
	}

	endpoints, err = st.getEndpoints(ctx, tx, rel.UUID)
	if err != nil {
		return relation.RelationDetailsResult{}, errors.Errorf("getting relation endpoints: %w", err)
	}

	return relation.RelationDetailsResult{
		Life:      rel.Life,
		UUID:      rel.UUID,
		ID:        rel.ID,
		Endpoints: endpoints,
	}, nil
}

func (st *State) getRelationStatus(ctx context.Context, tx *sqlair.TX, uuid corerelation.UUID) (corestatus.StatusInfo,
	error) {
	relStatus := relationStatus{
		RelationUUID: uuid,
	}

	stmt, err := st.Prepare(`
SELECT &relationStatus.*
FROM v_relation_status
WHERE relation_uuid = $relationStatus.relation_uuid`, relStatus)
	if err != nil {
		return corestatus.StatusInfo{}, errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, relStatus).Get(&relStatus)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) { // avoid error if the status has not yet been inserted
		return corestatus.StatusInfo{}, errors.Capture(err)
	}
	if errors.Is(err, sqlair.ErrNoRows) {
		return corestatus.StatusInfo{
			Status: corestatus.Joining,
		}, nil
	}
	return corestatus.StatusInfo{
		Status:  corestatus.Status(relStatus.Status),
		Message: relStatus.Reason,
		Since:   &relStatus.Since,
	}, nil
}
