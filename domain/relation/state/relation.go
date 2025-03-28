// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"strings"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/life"
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

// AddRelation establishes a new relation between two endpoints identified by epIdentifier1 and epIdentifier2.
// Returns the two endpoints involved in the relation and any error encountered during the operation.
//
// The following error types can be expected to be returned:
//   - [relationerrors.AmbiguousRelation] is returned if the endpoint
//     identifiers can refer to several possible relations.
//   - [relationerrors.NoRelationFound] is returned  if the endpoint identifiers
//     relates to correct endpoints yet not compatible (ex provider with provider)
//   - [relationerrors.RelationAlreadyExists] is returned  if the inferred
//     relation already exists.
//   - [relationerrors.RelationEndpointNotFound] is returned if no endpoint can be
//     inferred from one of the identifier.
func (st *State) AddRelation(ctx context.Context, epIdentifier1, epIdentifier2 relation.EndpointIdentifier) (
	relation.Endpoint,
	relation.Endpoint,
	error) {
	db, err := st.DB()
	if err != nil {
		return relation.Endpoint{}, relation.Endpoint{}, errors.Capture(err)
	}

	var endpoint1, endpoint2 relation.Endpoint

	return endpoint1, endpoint2, db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Infers endpoint, ie get both application_endpoint_uuid.
		ep1, ep2, err := st.inferEndpoints(ctx, tx, epIdentifier1, epIdentifier2)
		if err != nil {
			return errors.Errorf("cannot relate endpoints %q and %q: %w",
				epIdentifier1,
				epIdentifier2, err)
		}

		// Check the relation doesn't already exist.
		if err := st.relationAlreadyExists(ctx, tx, ep1, ep2); err != nil {
			return errors.Errorf("relation %q: %w", fmt.Sprintf("%s:%s %s:%s",
				ep1.ApplicationName, ep1.EndpointName, ep2.ApplicationName, ep2.EndpointName), err)
		}

		// Insert a new relation with a new relation UUID.
		relUUID, err := st.insertNewRelation(ctx, tx)
		if err != nil {
			return errors.Errorf("inserting new relation: %w", err)
		}

		// Insert both relation_endpoint from application_endpoint_uuid and relation
		// uuid.
		if err := st.insertNewRelationEndpoint(ctx, tx, relUUID, ep1.EndpointUUID); err != nil {
			return errors.Errorf("inserting new relation endpoint for %q: %w", epIdentifier1.String(), err)
		}
		if err := st.insertNewRelationEndpoint(ctx, tx, relUUID, ep2.EndpointUUID); err != nil {
			return errors.Errorf("inserting new relation endpoint for %q: %w", epIdentifier2.String(), err)
		}

		// Get endpoints from UUID.
		endpoints, err := st.getEndpoints(ctx, tx, relUUID)
		if err != nil {
			return errors.Errorf("getting endpoints of relation %q: %w", relUUID, err)
		}
		if l := len(endpoints); l != 2 {
			return errors.Errorf("internal error: expected 2 endpoints in relation, got %d", l)
		}
		endpoint1 = endpoints[0]
		endpoint2 = endpoints[1]

		return nil
	})
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
		UUID: relationUUID.String(),
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
		UUID: uuid,
	}
	stmt, err := st.Prepare(`
SELECT &endpoint.*
FROM   v_relation_endpoint
WHERE  relation_uuid = $relationUUID.uuid
ORDER  BY application_name, endpoint_name, role
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

	return uuid[0].UUID, nil
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
	if err != nil {
		return relation.RelationDetailsResult{}, errors.Capture(err)
	}

	type getRelation struct {
		UUID corerelation.UUID `db:"uuid"`
		ID   int               `db:"relation_id"`
		Life life.Value        `db:"value"`
	}
	rel := getRelation{
		ID: relationID,
	}
	stmt, err := st.Prepare(`
SELECT (r.uuid, r.relation_id, l.value) AS (&getRelation.*)
FROM   relation r
JOIN   life l ON r.life_id = l.id
WHERE  relation_id = $getRelation.relation_id
`, rel)
	if err != nil {
		return relation.RelationDetailsResult{}, errors.Capture(err)
	}

	var endpoints []relation.Endpoint
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, rel).Get(&rel)
		if errors.Is(err, sqlair.ErrNoRows) {
			return relationerrors.RelationNotFound
		} else if err != nil {
			return errors.Capture(err)
		}

		endpoints, err = st.getEndpoints(ctx, tx, rel.UUID)
		if err != nil {
			return errors.Errorf("getting relation endpoints: %w", err)
		}
		return errors.Capture(err)
	})
	if err != nil {
		return relation.RelationDetailsResult{}, errors.Capture(err)
	}

	return relation.RelationDetailsResult{
		Life:      rel.Life,
		UUID:      rel.UUID,
		ID:        rel.ID,
		Endpoints: endpoints,
	}, nil
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

// getCandidateEndpoints retrieves a list of candidate endpoints from the
// database matching the given identifier parameters.
// It queries based on application name and optionally endpoint name,
// returning all matching endpoints or an error.
func (st *State) getCandidateEndpoints(ctx context.Context, tx *sqlair.TX,
	identifier relation.EndpointIdentifier) ([]endpoint, error) {

	epIdentifier := endpointIdentifier{
		ApplicationName: identifier.ApplicationName,
		EndpointName:    identifier.EndpointName,
	}

	stmt, err := st.Prepare(`
SELECT &endpoint.*
FROM   v_application_endpoint
WHERE  application_name = $endpointIdentifier.application_name
AND    (
    endpoint_name    = $endpointIdentifier.endpoint_name
    OR $endpointIdentifier.endpoint_name = ''
)
`, endpoint{}, epIdentifier)
	if err != nil {
		return nil, errors.Capture(err)
	}
	var endpoints []endpoint
	err = tx.Query(ctx, stmt, epIdentifier).GetAll(&endpoints)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("getting candidate endpoints for %q: %w", identifier, err)
	}

	return endpoints, nil
}

// inferEndpoints determines and validates the endpoints of a given relation
// based on the provided identifiers. It tries to find matching endpoint for both
// application, regarding the name of endpoint if provided, their roles
// and their scopes. If there is several matches or none, it will return an error.
func (st *State) inferEndpoints(
	ctx context.Context,
	tx *sqlair.TX,
	identifier1, identifier2 relation.EndpointIdentifier) (endpoint, endpoint, error) {

	// Get candidate endpoints.
	endpoints1, err := st.getCandidateEndpoints(ctx, tx, identifier1)
	if err != nil {
		return endpoint{}, endpoint{}, errors.Capture(err)
	}
	endpoints2, err := st.getCandidateEndpoints(ctx, tx, identifier2)
	if err != nil {
		return endpoint{}, endpoint{}, errors.Capture(err)
	}

	var noCandidates []string
	if len(endpoints1) == 0 {
		noCandidates = append(noCandidates, identifier1.String())
	}
	if len(endpoints2) == 0 {
		noCandidates = append(noCandidates, identifier2.String())
	}
	if len(noCandidates) > 0 {
		return endpoint{}, endpoint{}, errors.Errorf("no candidates for %s: %w",
			strings.Join(noCandidates, " and "),
			relationerrors.RelationEndpointNotFound)
	}

	// Check if applications are subordinates.
	isSubordinate1, err := st.isSubordinate(ctx, tx, identifier1.ApplicationName)
	if err != nil {
		return endpoint{}, endpoint{}, errors.Capture(err)
	}
	isSubordinate2, err := st.isSubordinate(ctx, tx, identifier2.ApplicationName)
	if err != nil {
		return endpoint{}, endpoint{}, errors.Capture(err)
	}

	// Compute matches.
	type match struct {
		ep1 *endpoint
		ep2 *endpoint
	}
	var matches []match
	for _, e1 := range endpoints1 {
		for _, e2 := range endpoints2 {
			ep1 := e1.toRelationEndpoint()
			ep2 := e2.toRelationEndpoint()
			if (ep1.Scope == charm.ScopeContainer || ep2.Scope == charm.ScopeContainer) &&
				!isSubordinate1 && !isSubordinate2 {
				continue
			}
			if ep1.CanRelateTo(ep2) {
				matches = append(matches, match{ep1: &e1, ep2: &e2})
			}
		}
	}
	// todo(gfouillet) - in 3.6, implicit relation are discarded (ie juju-info)

	if matchCount := len(matches); matchCount == 0 {
		return endpoint{}, endpoint{}, relationerrors.NoRelationFound
	} else if matchCount > 1 {
		return endpoint{}, endpoint{}, relationerrors.AmbiguousRelation
	}

	return *matches[0].ep1, *matches[0].ep2, nil
}

// insertNewRelation creates a new relation entry in the database and returns its UUID or an error if the operation fails.
func (st *State) insertNewRelation(ctx context.Context, tx *sqlair.TX) (corerelation.UUID, error) {
	relUUID, err := corerelation.NewUUID()
	if err != nil {
		return relUUID, errors.Errorf("generating new relation UUID: %w", err)
	}

	relUUIDArg := relationUUID{
		UUID: relUUID,
	}
	stmt, err := st.Prepare(`
INSERT INTO relation (uuid, life_id, relation_id)
SELECT $relationUUID.uuid, 0, count(*)+1
FROM relation
`, relUUIDArg)
	if err != nil {
		return relUUID, errors.Capture(err)
	}

	if err := tx.Query(ctx, stmt, relUUIDArg).Run(); err != nil {
		return relUUID, errors.Capture(err)
	}

	return relUUID, nil
}

// insertNewRelationEndpoint inserts a new relation endpoint into the database
// using the provided context and transaction.
func (st *State) insertNewRelationEndpoint(ctx context.Context, tx *sqlair.TX, relUUID corerelation.UUID,
	endpointUUID corerelation.EndpointUUID) error {
	uuid, err := corerelation.NewEndpointUUID()
	if err != nil {
		return errors.Errorf("generating new relation endpoint UUID: %w", err)
	}

	endpoint := setRelationEndpoint{
		UUID:         uuid,
		RelationUUID: relUUID,
		EndpointUUID: endpointUUID,
	}
	stmt, err := st.Prepare(`
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES($setRelationEndpoint.*)`, endpoint)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, stmt, endpoint).Run(); err != nil {
		return errors.Capture(err)
	}

	return nil
}

// isSubordinate determines if the specified application is a subordinate based
// on its metadata in the database.
func (st *State) isSubordinate(ctx context.Context, tx *sqlair.TX, applicationName string) (bool, error) {
	appName := endpointIdentifier{
		ApplicationName: applicationName,
	}
	stmt, err := st.Prepare(`
SELECT a.name AS &endpointIdentifier.application_name
FROM application a
JOIN v_charm_metadata vcm ON a.charm_uuid = vcm.uuid
WHERE a.name = $endpointIdentifier.application_name
AND vcm.subordinate = true
`, appName)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, appName).Get(&appName)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return false, errors.Errorf("getting subordinate status for %q: %w", appName, err)
	}

	return err == nil, nil
}

// relationAlreadyExists checks if a relation already exists between two
// endpoints in the database and returns an error if it does.
func (st *State) relationAlreadyExists(ctx context.Context, tx *sqlair.TX, ep1 endpoint, ep2 endpoint) error {
	type (
		endpoint1 endpoint
		endpoint2 endpoint
	)
	e1 := endpoint1(ep1)
	e2 := endpoint2(ep2)

	stmt, err := st.Prepare(`
SELECT r.uuid as &relationUUID.uuid
FROM relation r
JOIN relation_endpoint e1 ON r.uuid = e1.relation_uuid
JOIN relation_endpoint e2 ON r.uuid = e2.relation_uuid
WHERE e1.endpoint_uuid = $endpoint1.endpoint_uuid                          
AND e2.endpoint_uuid = $endpoint2.endpoint_uuid
`, relationUUID{}, e1, e2)
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, e1, e2).Get(&relationUUID{})
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil
	} else if err == nil {
		return relationerrors.RelationAlreadyExists
	}
	return errors.Capture(err)
}
