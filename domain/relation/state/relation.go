// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/application"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	corerelation "github.com/juju/juju/core/relation"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/eventsource"
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

// AddRelation establishes a new relation between two endpoints identified by
// epIdentifier1 and epIdentifier2.
// Returns the two endpoints involved in the relation and any error encountered
// during the operation.
//
// The following error types can be expected to be returned:
//   - [relationerrors.AmbiguousRelation] is returned if the endpoint
//     identifiers can refer to several possible relations.
//   - [relationerrors.ApplicationNotAlive] is returned if one of the application
//     is not alive.
//   - [relationerrors.CompatibleEndpointsNotFound] is returned  if the endpoint
//     identifiers relates to correct endpoints yet not compatible (ex provider
//     with provider)
//   - [relationerrors.RelationAlreadyExists] is returned  if the inferred
//     relation already exists.
//   - [relationerrors.RelationEndpointNotFound] is returned if no endpoint can be
//     inferred from one of the identifier.
func (st *State) AddRelation(ctx context.Context, epIdentifier1, epIdentifier2 relation.CandidateEndpointIdentifier) (
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
			return errors.Errorf("relation %s %s: %w", ep1, ep2, err)
		}

		// Check both application are alive
		if alive, err := st.checkLife(ctx, tx, "application", ep1.ApplicationUUID.String(), life.IsAlive); err != nil {
			return errors.Errorf("relation %s %s: cannot check application life: %w", ep1, ep2, err)
		} else if !alive {
			return errors.Errorf("relation %s %s: application %s is not alive", ep1, ep2, ep1.ApplicationName).Add(relationerrors.ApplicationNotAlive)
		}
		if alive, err := st.checkLife(ctx, tx, "application", ep2.ApplicationUUID.String(), life.IsAlive); err != nil {
			return errors.Errorf("relation %s %s: cannot check application life: %w", ep1, ep2, err)
		} else if !alive {
			return errors.Errorf("relation %s %s: application %s is not alive", ep1, ep2, ep2.ApplicationName).Add(relationerrors.ApplicationNotAlive)
		}

		// Check the application bases are compatible, if required
		if err := st.checkCompatibleBases(ctx, tx, ep1, ep2); err != nil {
			return errors.Errorf("relation %s %s: %w", ep1, ep2, err)
		}

		// Check that adding a relation won't exceed any endpoint limit
		if err := st.checkEndpointCapacity(ctx, tx, ep1); err != nil {
			return errors.Errorf("relation %s %s: %w", ep1, ep2, err)
		}
		if err := st.checkEndpointCapacity(ctx, tx, ep2); err != nil {
			return errors.Errorf("relation %s %s: %w", ep1, ep2, err)
		}

		// Insert a new relation with a new relation UUID.
		relUUID, err := st.insertNewRelation(ctx, tx)
		if err != nil {
			return errors.Errorf("inserting new relation: %w", err)
		}

		// Insert relation status.
		if err := st.insertNewRelationStatus(ctx, tx, relUUID); err != nil {
			return errors.Errorf("inserting new relation %s %s: %w", ep1, ep2, err)
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
		endpoints, err := st.getRelationEndpoints(ctx, tx, relUUID)
		if err != nil {
			return errors.Errorf("getting endpoints of relation %q: %w", relUUID, err)
		}
		if l := len(endpoints); l != 2 {
			return errors.Errorf("internal error: expected 2 endpoints in relation, got %d", l)
		}

		// order results to have the same order between input candidate and output result.
		for _, e := range endpoints {
			if e.ApplicationName == ep1.ApplicationName && e.Name == ep1.EndpointName {
				endpoint1 = e
			} else {
				endpoint2 = e
			}
		}
		if len(endpoint1.Name) == 0 || len(endpoint2.Name) == 0 {
			// should not happen, unless above resolution loop is broken or
			// db corrupted.
			return errors.Errorf("unexpected empty endpoint name")
		}

		return nil
	})
}

// GetAllRelationDetails retrieves the details of all relations
// from the database. It returns the list of endpoints, the life status and
// identification data (UUID and ID) for each relation.
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
			details, err := st.getRelationDetails(ctx, tx, rel.UUID)
			if err != nil {
				return errors.Errorf("getting relation details: %w", err)
			}
			relationsDetails = append(relationsDetails, details)
		}
		return nil
	})
	return relationsDetails, errors.Capture(err)
}

// GetApplicationRelations retrieves a list of relation UUIDs for a given
// application ID within the specified context.
//
// The following error types can be expected to be returned:
//   - [relationerrors.ApplicationNotFound] is returned if application ID
//     doesn't refer an existing application.
func (st *State) GetApplicationRelations(ctx context.Context, id application.ID) ([]corerelation.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	var relations []relationUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Check if the application exists
		appFound, err := st.checkExistsByUUID(ctx, tx, "application", id.String())
		if err != nil {
			return errors.Capture(err)
		}
		if !appFound {
			return errors.Errorf("%w: %s", relationerrors.ApplicationNotFound, id)
		}

		appID := applicationID{ID: id}

		stmt, err := st.Prepare(`
SELECT re.relation_uuid AS &relationUUID.uuid
FROM   relation_endpoint AS re
JOIN   application_endpoint AS ae ON re.endpoint_uuid = ae.uuid
WHERE  ae.application_uuid = $applicationID.uuid
`, relationUUID{}, appID)
		if err != nil {
			return errors.Capture(err)
		}
		err = tx.Query(ctx, stmt, appID).GetAll(&relations)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})

	var results []corerelation.UUID
	for _, rel := range relations {
		results = append(results, rel.UUID)
	}
	return results, err
}

// GetOtherRelatedEndpointApplicationData returns an OtherApplicationForWatcher struct
// for the other Endpoint in a relation with the given application ID.
//
// The following error types can be expected to be returned:
//   - [relationerrors.ApplicationNotFound] is returned if application ID
//     is not used in any relations or if the other relation applications
//     are not found.
func (st *State) GetOtherRelatedEndpointApplicationData(
	ctx context.Context,
	relUUID corerelation.UUID,
	applicationID application.ID,
) (relation.OtherApplicationForWatcher, error) {
	db, err := st.DB()
	if err != nil {
		return relation.OtherApplicationForWatcher{}, errors.Capture(err)
	}

	otherAppSub := otherApplicationsForWatcher{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Find all applications in a relation with the given application.
		otherApp, err := st.getOtherApplicationInRelations(ctx, tx, relUUID, applicationID)
		if err != nil {
			return errors.Capture(err)
		}

		// For all applications, determine if it is a subordinate.
		otherAppSub, err = st.getApplicationSubordinate(ctx, tx, otherApp)
		if err != nil {
			return errors.Capture(err)
		}

		return nil
	})
	if err != nil {
		return relation.OtherApplicationForWatcher{}, errors.Capture(err)
	}

	return relation.OtherApplicationForWatcher{
		ApplicationID: otherAppSub.AppID,
		Subordinate:   otherAppSub.Subordinate,
	}, nil
}

// getOtherApplicationInRelations returns the applications ID
// at the other end of the relation from the given application ID.
func (st *State) getOtherApplicationInRelations(
	ctx context.Context,
	tx *sqlair.TX,
	relUUID corerelation.UUID,
	appID application.ID,
) (application.ID, error) {

	findOtherEndsStmt, err := st.Prepare(`
SELECT other.application_uuid AS &relationAndApplicationUUID.application_uuid
FROM   v_application_subordinate AS app
JOIN   v_application_subordinate AS other ON app.relation_uuid = other.relation_uuid
WHERE  app.application_uuid = $relationAndApplicationUUID.application_uuid
AND    other.application_uuid != $relationAndApplicationUUID.application_uuid
AND    app.relation_uuid = $relationAndApplicationUUID.relation_uuid
`, relationAndApplicationUUID{})
	if err != nil {
		return "", errors.Errorf("preparing other endpoint query: %w", err)
	}

	app := relationAndApplicationUUID{
		ApplicationID: appID,
		RelationUUID:  relUUID,
	}
	otherApp := relationAndApplicationUUID{}

	err = tx.Query(ctx, findOtherEndsStmt, app).Get(&otherApp)
	if err != nil {
		return "", errors.Capture(err)
	}
	return otherApp.ApplicationID, nil
}

// getApplicationSubordinate returns a otherApplicationsForWatcher structure
// for each given application ID.
func (st *State) getApplicationSubordinate(
	ctx context.Context,
	tx *sqlair.TX,
	app application.ID,
) (otherApplicationsForWatcher, error) {

	appSubordinateStmt, err := st.Prepare(`
SELECT application_uuid AS &otherApplicationsForWatcher.application_uuid,
       subordinate AS &otherApplicationsForWatcher.subordinate
FROM   v_application_subordinate
WHERE  application_uuid = $applicationID.uuid
`, otherApplicationsForWatcher{}, applicationID{})
	if err != nil {
		return otherApplicationsForWatcher{}, errors.Errorf("preparing other application query: %w", err)
	}

	appID := applicationID{ID: app}
	otherApp := otherApplicationsForWatcher{}
	err = tx.Query(ctx, appSubordinateStmt, appID).Get(&otherApp)
	if err != nil {
		return otherApplicationsForWatcher{}, errors.Capture(err)
	}

	return otherApp, nil
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

	return id.UUID, nil
}

// GetRelationEndpointScope returns the scope of the relation endpoint
// at the intersection of the relationUUID and applicationID.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationNotFound] is returned if the relation UUID
//     relating to the relation ID cannot be found.
func (st *State) GetRelationEndpointScope(
	ctx context.Context,
	relUUID corerelation.UUID,
	appID application.ID,
) (charm.RelationScope, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	type scope struct {
		Scope charm.RelationScope `db:"name"`
	}

	stmt, err := st.Prepare(`
SELECT  scope AS &scope.name
FROM    v_relation_endpoint
WHERE   relation_uuid = $relationUUID.uuid
AND     application_uuid = $applicationID.uuid
`, scope{}, applicationID{}, relationUUID{})
	if err != nil {
		return "", errors.Capture(err)
	}

	rel := relationUUID{
		UUID: relUUID,
	}
	app := applicationID{ID: appID}
	var output scope
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Check if the relation exists.
		relationFound, err := st.checkExistsByUUID(ctx, tx, "relation", relUUID.String())
		if err != nil {
			return errors.Capture(err)
		} else if !relationFound {
			return relationerrors.RelationNotFound
		}

		return tx.Query(ctx, stmt, rel, app).Get(&output)
	})
	if err != nil {
		return "", errors.Capture(err)
	}

	return output.Scope, nil
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
			appFound, err := st.checkExistsByUUID(ctx, tx, "application", args.ApplicationID.String())
			if err != nil {
				return errors.Capture(err)
			}
			// Check if the relation exists.
			relationFound, err := st.checkExistsByUUID(ctx, tx, "relation", args.RelationUUID.String())
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
SELECT
    re.relation_uuid AS &relationUnitStatus.relation_uuid,
    ru.uuid IS NOT NULL AS &relationUnitStatus.in_scope,
    vrs.status AS &relationUnitStatus.status
FROM relation_endpoint AS re
JOIN application_endpoint AS ae ON re.endpoint_uuid = ae.uuid
JOIN unit AS u ON ae.application_uuid = u.application_uuid
JOIN v_relation_status AS vrs ON re.relation_uuid = vrs.relation_uuid
LEFT JOIN relation_unit AS ru ON re.uuid = ru.relation_endpoint_uuid
WHERE  u.uuid = $unitUUIDArg.unit_uuid
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
			endpoints, err := st.getRelationEndpoints(ctx, tx, corerelation.UUID(status.RelationUUID))
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
		endpoints, err = st.getRelationEndpoints(ctx, tx, uuid)
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return endpoints, nil
}

// getRelationEndpoints retrieves the endpoints of the specified relation.
func (st *State) getRelationEndpoints(
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
`, id, endpoint{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var endpoints []endpoint
	err = tx.Query(ctx, stmt, id).GetAll(&endpoints)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, relationerrors.RelationNotFound
	} else if err != nil {
		return nil, errors.Capture(err)
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

// GetApplicationEndpoints returns all endpoints for the given application
// identifier.
func (st *State) GetApplicationEndpoints(
	ctx context.Context,
	applicationID application.ID,
) ([]relation.Endpoint, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	id := applicationUUID{
		UUID: applicationID,
	}
	stmt, err := st.Prepare(`
SELECT &endpoint.*
FROM   v_application_endpoint
WHERE  application_uuid = $applicationUUID.application_uuid
`, id, endpoint{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var eps []endpoint
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, id).GetAll(&eps)
		if errors.Is(err, sqlair.ErrNoRows) {
			// If there are no endpoints we return an empty list.
			return nil
		} else if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var endpoints []relation.Endpoint
	for _, ep := range eps {
		endpoints = append(endpoints, ep.toRelationEndpoint())
	}

	return endpoints, nil
}

// GetRegularRelationUUIDByEndpointIdentifiers gets the UUID of a regular
// relation specified by two endpoint identifiers.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationNotFound] is returned if endpoints cannot be
//     found.
func (st *State) GetRegularRelationUUIDByEndpointIdentifiers(
	ctx context.Context,
	endpoint1, endpoint2 corerelation.EndpointIdentifier,
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
	endpoint corerelation.EndpointIdentifier,
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
func (st *State) GetRelationDetails(ctx context.Context, relationUUID corerelation.UUID) (relation.RelationDetailsResult, error) {
	db, err := st.DB()
	var result relation.RelationDetailsResult
	if err != nil {
		return result, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result, err = st.getRelationDetails(ctx, tx, relationUUID)
		return errors.Capture(err)
	})
	return result, errors.Capture(err)
}

// GetRelationUnitEndpointName returns the name of the endpoint for the given
// relation unit.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationUnitNotFound] if the relation unit cannot be found.
func (st *State) GetRelationUnitEndpointName(
	ctx context.Context,
	relationUnitUUID corerelation.UnitUUID,
) (string, error) {
	db, err := st.DB()
	var result string
	if err != nil {
		return result, errors.Capture(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		args := getRelationUnitEndpointName{
			RelationUnitUUID: relationUnitUUID,
		}
		stmt, err := st.Prepare(`
SELECT &getRelationUnitEndpointName.*
FROM v_relation_unit_endpoint
WHERE relation_unit_uuid = $getRelationUnitEndpointName.relation_unit_uuid`, args)
		if err != nil {
			return errors.Capture(err)
		}

		err = tx.Query(ctx, stmt, args).Get(&args)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("relation unit %q not found", relationUnitUUID).Add(
				relationerrors.RelationUnitNotFound)
		}
		if err != nil {
			return errors.Capture(err)
		}
		result = args.EndpointName
		return nil
	})
	return result, errors.Capture(err)
}

// GetRelationUnit retrieves the UUID of a relation unit based on the given
// relation UUID and unit name.
func (st *State) GetRelationUnit(
	ctx context.Context,
	relationUUID corerelation.UUID,
	unitName unit.Name,
) (corerelation.UnitUUID, error) {
	db, err := st.DB()
	var result corerelation.UnitUUID
	if err != nil {
		return result, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		args := getRelationUnit{
			RelationUUID: relationUUID,
			Name:         unitName,
		}
		stmt, err := st.Prepare(`
SELECT ru.uuid AS &getRelationUnit.unit_uuid
FROM   relation_unit ru
JOIN   unit AS u ON ru.unit_uuid = u.uuid 
JOIN   relation_endpoint AS re ON ru.relation_endpoint_uuid = re.uuid
WHERE  u.name = $getRelationUnit.name
AND    re.relation_uuid = $getRelationUnit.relation_uuid`, args)
		if err != nil {
			return errors.Capture(err)
		}

		err = tx.Query(ctx, stmt, args).Get(&args)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("unit %q: %w", unitName, relationerrors.UnitNotFound)
		}
		if err != nil {
			return errors.Errorf("getting unit: %w", err)
		}
		result = args.UnitUUID
		return nil
	})
	return result, errors.Capture(err)
}

// EnterScope indicates that the provided unit has joined the relation.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationNotFound] if the relation cannot be found.
//   - [relationerrors.UnitNotFound] if no unit by the given name can be found
//   - [relationerrors.RelationNotAlive] if the relation is not alive.
//   - [relationerrors.UnitNotAlive] if the  is not alive.
//   - [relationerrors.PotentialRelationUnitNotValid] if the unit entering scope
//     is a subordinate and its endpoint has scope charm.ScopeContainer, but the
//     principal application of the unit is not the application in the relation.
func (st *State) EnterScope(
	ctx context.Context,
	relationUUID corerelation.UUID,
	unitName unit.Name,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	unitArgs := getUnit{
		Name: unitName,
	}
	getUnitStmt, err := st.Prepare(`
SELECT &getUnit.*
FROM   unit
WHERE  name = $getUnit.name
`, unitArgs)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Get the UUID of the unit entering scope.
		err = tx.Query(ctx, getUnitStmt, unitArgs).Get(&unitArgs)
		if errors.Is(err, sqlair.ErrNoRows) {
			return relationerrors.UnitNotFound
		} else if err != nil {
			return errors.Capture(err)
		}

		// Check the unit can enter scope in this relation.
		err := st.checkUnitCanEnterScope(ctx, tx, relationUUID, unitArgs.UUID)
		if err != nil {
			return errors.Errorf("checking unit valid in relation: %w", err)
		}

		// Upsert the row recording that the unit has entered scope.
		err = st.insertRelationUnit(ctx, tx, relationUUID, unitArgs.UUID)
		if err != nil {
			return errors.Capture(err)
		}
		return err
	})
	return errors.Capture(err)
}

// checkUnitCanEnterScope checks that the unit can enter scope in the given
// relation.
func (st *State) checkUnitCanEnterScope(
	ctx context.Context,
	tx *sqlair.TX,
	relationUUID corerelation.UUID,
	unitUUID unit.UUID,
) error {
	// Check relation is alive.
	relationLife, err := st.getLife(ctx, tx, "relation", relationUUID.String())
	if errors.Is(err, coreerrors.NotFound) {
		return relationerrors.RelationNotFound
	} else if err != nil {
		return errors.Errorf("getting relation life: %w", err)
	}
	if relationLife != life.Alive {
		return relationerrors.RelationNotAlive
	}

	// Check unit is alive.
	unitLife, err := st.getLife(ctx, tx, "unit", unitUUID.String())
	if errors.Is(err, coreerrors.NotFound) {
		return relationerrors.UnitNotFound
	} else if err != nil {
		return errors.Errorf("getting unit life: %w", err)
	}
	if unitLife != life.Alive {
		return relationerrors.UnitNotAlive
	}

	// Get the IDs of the applications in the relation.
	appIDs, err := st.getApplicationsInRelation(ctx, tx, relationUUID)
	if err != nil {
		return errors.Errorf("getting applications in relation: %w", err)
	}

	// Get the ID of the application of the unit trying to enter scope.
	unitsAppID, err := st.getApplicationOfUnit(ctx, tx, unitUUID)
	if err != nil {
		return errors.Errorf("getting application of unit: %w", err)
	}

	// Check that the application of the unit is in the relation.
	found := false
	switch len(appIDs) {
	case 1: // Peer relation.
		if appIDs[0] == unitsAppID {
			found = true
		}
	case 2: // Regular relation.
		var otherAppID application.ID
		if appIDs[0] == unitsAppID {
			found = true
			otherAppID = appIDs[1]
		} else if appIDs[1] == unitsAppID {
			found = true
			otherAppID = appIDs[0]
		}

		// If the unit is a subordinate, check that it can enter scope in this
		// relation.
		if subordinate, err := st.isSubordinate(ctx, tx, unitsAppID); err != nil {
			return errors.Errorf("checking if application is subordinate: %w", err)
		} else if subordinate {
			err := st.checkSubordinateUnitCanEnterScope(ctx, tx, relationUUID, unitUUID, otherAppID)
			if err != nil {
				return errors.Errorf("checking subordinate unit can enter scope %w", err)
			}
		}
	}
	if !found {
		return relationerrors.UnitNotInRelation
	}

	return nil
}

// getLife takes a table and a UUID, it joins the table to life on life_id. If
// the UUID cannot be found [coreerrors.NotFound] is returned.
func (st *State) getLife(
	ctx context.Context,
	tx *sqlair.TX,
	table, uuid string,
) (life.Value, error) {
	args := getLife{
		UUID: uuid,
	}
	stmt, err := st.Prepare(fmt.Sprintf(`
SELECT &getLife.*
FROM   %s t
JOIN   life l ON t.life_id = l.id
WHERE  t.uuid = $getLife.uuid
`, table), args)
	if err != nil {
		return "", errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, args).Get(&args)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", coreerrors.NotFound
	} else if err != nil {
		return "", errors.Capture(err)
	}

	return args.Life, nil
}

// getApplicationOfUnit returns the ID of the application associated with the
// unit.
func (st *State) getApplicationOfUnit(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID unit.UUID,
) (application.ID, error) {
	args := getUnitApp{
		UnitUUID: unitUUID,
	}
	stmt, err := st.Prepare(`
SELECT &getUnitApp.*
FROM   unit
WHERE  uuid = $getUnitApp.uuid
`, args)
	if err != nil {
		return "", errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, args).Get(&args)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", relationerrors.UnitNotFound
	} else if err != nil {
		return "", errors.Capture(err)
	}

	return args.ApplicationUUID, nil
}

// getApplicationsInRelation gets all the applications that are in the given
// relation.
func (st *State) getApplicationsInRelation(
	ctx context.Context,
	tx *sqlair.TX,
	uuid corerelation.UUID,
) ([]application.ID, error) {
	relUUID := relationUUID{
		UUID: uuid,
	}
	stmt, err := st.Prepare(`
SELECT &applicationUUID.application_uuid
FROM   relation_endpoint re 
JOIN   application_endpoint ae ON re.endpoint_uuid = ae.uuid
WHERE  re.relation_uuid = $relationUUID.uuid
`, relUUID, applicationUUID{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	var apps []applicationUUID
	err = tx.Query(ctx, stmt, relUUID).GetAll(&apps)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, relationerrors.RelationNotFound
	} else if err != nil {
		return nil, errors.Capture(err)
	}

	var ids []application.ID
	for _, app := range apps {
		ids = append(ids, app.UUID)
	}

	return ids, nil
}

// checkSubordinateUnitCanEnterScope checks that subordinate units can enter
// scope.
//
// When the all following three conditions are true:
// 1. The relation has scope container.
// 2. The unit entering scope is a subordinate.
// 3. The other application is not a subordinate.
// Then the subordinate unit cannot enter a relation unless its principle
// application is the one in the relation. In this case, the error
// [relationerrors.PotentialRelationUnitNotValid] is returned.
//
// The above scenario can happen when a subordinate application is deployed then
// related to multiple principle applications. The units of the single
// subordinate application can have different principle applications depending
// on which machine they are on. When the subordinate application is related to
// a new principle application, watchers will trigger for all of its units, and
// they will all try to enter the relation scope. They should only succeed if
// they are the units of the new principle application, otherwise the error is
// returned.
func (st *State) checkSubordinateUnitCanEnterScope(
	ctx context.Context,
	tx *sqlair.TX,
	relUUID corerelation.UUID,
	unitUUID unit.UUID,
	otherApplication application.ID,
) error {
	// Check that the other application in the relation is not a subordinate
	// application, if it is, we have a relation between two subordinates, which
	// is OK.
	if subordinate, err := st.isSubordinate(ctx, tx, otherApplication); err != nil {
		return errors.Errorf("checking if application is subordinate: %w", err)
	} else if subordinate {
		return nil
	}

	// Check that the relation is a container scoped relation.
	scope, err := st.getRelationScope(ctx, tx, relUUID)
	if err != nil {
		return errors.Errorf("getting relation scope: %w", err)
	}
	if scope != charm.ScopeContainer {
		return nil
	}

	// Check that the principle application of the unit is the other application
	// in the relation the unit is trying to enter scope in.
	principleAppID, err := st.getPrincipalApplicationOfUnit(ctx, tx, unitUUID)
	if err != nil {
		return errors.Errorf("getting principal application of unit: %w", err)
	}
	if principleAppID != otherApplication {
		return errors.Errorf(
			"unit cannot enter scope: principle application not in relation: %w",
			relationerrors.PotentialRelationUnitNotValid,
		)
	}

	return nil
}

// getRelationScope returns the scope of the given relation. Relation scope is
// defined by the scope of the endpoints in the relation. If it is a peer
// relation, then it is the scope of the single endpoint. If it is a regular
// relation, then it is Container if either/both of the endpoints have Container
// scope, and Global if both have Global scope.
func (st *State) getRelationScope(
	ctx context.Context,
	tx *sqlair.TX,
	uuid corerelation.UUID,
) (charm.RelationScope, error) {
	relUUID := relationUUID{
		UUID: uuid,
	}
	getScopeStmt, err := st.Prepare(`
SELECT &getScope.*
FROM   v_relation_endpoint
WHERE  relation_uuid = $relationUUID.uuid
`, relUUID, getScope{})
	if err != nil {
		return "", errors.Capture(err)
	}
	var scopes []getScope
	err = tx.Query(ctx, getScopeStmt, relUUID).GetAll(&scopes)
	if err != nil {
		return "", errors.Capture(err)
	}

	// Scope can either be Global or Container. It is container if any of the
	// endpoints are Container scoped.
	scope := charm.ScopeGlobal
	for _, s := range scopes {
		if s.Scope == charm.ScopeContainer {
			scope = charm.ScopeContainer
		}
	}

	return scope, nil
}

// isSubordinate returns true if the application is a subordinate application.
func (st *State) isSubordinate(
	ctx context.Context,
	tx *sqlair.TX,
	applicationUUID application.ID,
) (bool, error) {
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

	err = tx.Query(ctx, stmt, subordinate).Get(&subordinate)
	if err != nil {
		return false, errors.Capture(err)
	}

	return subordinate.Subordinate, nil
}

// getPrincipalApplicationOfUnit returns the UUID of the principle application
// of a unit. If the unit has no principle application, then the error
// [relationerrors.UnitPrincipalNotFound] is returned (this means the error is
// always returned when the unit is not a subordinate).
func (st *State) getPrincipalApplicationOfUnit(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID unit.UUID,
) (application.ID, error) {
	principal := getPrincipal{
		UnitUUID: unitUUID,
	}

	stmt, err := st.Prepare(`
SELECT &getPrincipal.application_uuid
FROM   unit u
JOIN   unit_principal up ON up.principal_uuid = u.uuid
WHERE  up.unit_uuid = $getPrincipal.unit_uuid
`, principal)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, principal).Get(&principal)
	if errors.Is(sql.ErrNoRows, err) {
		return "", relationerrors.UnitPrincipalNotFound
	} else if err != nil {
		return "", errors.Capture(err)
	}

	return principal.ApplicationUUID, nil
}

// insertRelationUnit inserts a relation unit record, if it doesn't exist.
func (st *State) insertRelationUnit(
	ctx context.Context,
	tx *sqlair.TX,
	relationUUID corerelation.UUID,
	unitUUID unit.UUID,
) error {
	// Check if a relation_unit record already exists for this unit.
	getRelationUnit := relationUnit{
		RelationUUID: relationUUID,
		UnitUUID:     unitUUID,
	}
	getRelationUnitStmt, err := st.Prepare(`
SELECT   
	ru.uuid AS &relationUnit.uuid
FROM    relation_unit AS ru
JOIN    relation_endpoint AS re ON ru.relation_endpoint_uuid = re.uuid
WHERE   re.relation_uuid = $relationUnit.relation_uuid
AND     ru.unit_uuid = $relationUnit.unit_uuid
`, getRelationUnit)
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, getRelationUnitStmt, getRelationUnit).Get(&getRelationUnit)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Capture(err)
	}
	if err == nil {
		// If there is already a relation unit in the table,
		// it means it is in scope
		return nil
	}

	// Insert a new relation unit
	uuid, err := corerelation.NewUnitUUID()
	if err != nil {
		return errors.Capture(err)
	}
	insertRelationUnit := relationUnit{
		RelationUnitUUID: uuid,
		RelationUUID:     relationUUID,
		UnitUUID:         unitUUID,
	}

	insertStmt, err := st.Prepare(`
INSERT INTO relation_unit (uuid, relation_endpoint_uuid, unit_uuid) 
SELECT $relationUnit.uuid, re.uuid, $relationUnit.unit_uuid
FROM   relation_endpoint AS re
JOIN   application_endpoint AS ae ON re.endpoint_uuid = ae.uuid
JOIN   unit AS u ON ae.application_uuid = u.application_uuid
WHERE  re.relation_uuid = $relationUnit.relation_uuid
AND    u.uuid = $relationUnit.unit_uuid
`, insertRelationUnit)
	if err != nil {
		return errors.Capture(err)
	}

	return tx.Query(ctx, insertStmt, insertRelationUnit).Run()
}

// LeaveScope updates the given relation to indicate it is not in scope.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationUnitNotFound] if the relation unit cannot be
//     found.
func (st *State) LeaveScope(ctx context.Context, relationUnitUUID corerelation.UnitUUID) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Check the relation unit exists.
		exists, err := st.checkExistsByUUID(ctx, tx, "relation_unit", relationUnitUUID.String())
		if err != nil {
			return errors.Errorf("checking relation unit exists: %w", err)
		} else if !exists {
			return relationerrors.RelationUnitNotFound
		}

		// Leave scope by deleting the relation unit.
		err = st.deleteRelationUnit(ctx, tx, relationUnitUUID)
		if err != nil {
			return errors.Capture(err)
		}

		return nil
	})
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (st *State) deleteRelationUnit(
	ctx context.Context,
	tx *sqlair.TX,
	uuid corerelation.UnitUUID,
) error {
	id := relationUnitUUID{
		RelationUnitUUID: uuid,
	}
	deleteSettingsStmt, err := st.Prepare(`
DELETE FROM relation_unit_setting
WHERE relation_unit_uuid = $relationUnitUUID.uuid
`, id)
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, deleteSettingsStmt, id).Run()
	if err != nil {
		return errors.Capture(err)
	}

	deleteRelationUnitStmt, err := st.Prepare(`
DELETE FROM relation_unit 
WHERE uuid = $relationUnitUUID.uuid
`, id)
	if err != nil {
		return errors.Capture(err)
	}

	var outcome sqlair.Outcome
	err = tx.Query(ctx, deleteRelationUnitStmt, id).Get(&outcome)
	if err != nil {
		return errors.Capture(err)
	}

	rows, err := outcome.Result().RowsAffected()
	if err != nil {
		return errors.Capture(err)
	} else if rows != 1 {
		return errors.Errorf("deleting relation unit: expected 1 row affected, got %d", rows)
	}

	return nil
}

// GetRelationApplicationSettings returns the application settings
// for the given application and relation identifier combination.
//
// The following error types can be expected to be returned:
//   - [relationerrors.ApplicationNotFoundForRelation] is returned if the
//     application is not part of the relation.
//   - [relationerrors.RelationNotFound] is returned if the relation UUID
//     is not found.
func (st *State) GetRelationApplicationSettings(
	ctx context.Context,
	relationUUID corerelation.UUID,
	applicationID application.ID,
) (map[string]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	var settings []relationSetting
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		endpointUUID, err := st.getRelationEndpointUUID(ctx, tx, relationUUID, applicationID)
		if err != nil {
			return errors.Errorf("getting relation endpoint UUID: %w", err)
		}

		settings, err = st.getApplicationSettings(ctx, tx, endpointUUID)
		if err != nil {
			return errors.Capture(err)
		}

		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	relationSettings := make(map[string]string, len(settings))
	for _, setting := range settings {
		relationSettings[setting.Key] = setting.Value
	}
	return relationSettings, nil
}

// SetRelationApplicationSettings records settings for a specific application
// relation combination.
//
// The following error types can be expected to be returned:
//   - [relationerrors.ApplicationNotFoundForRelation] is returned if the
//     application is not part of the relation.
//   - [relationerrors.RelationNotFound] is returned if the relation UUID
//     is not found.
func (st *State) SetRelationApplicationSettings(
	ctx context.Context,
	relationUUID corerelation.UUID,
	applicationID application.ID,
	settings map[string]string,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Get the relation endpoint UUID.
		endpointUUID, err := st.getRelationEndpointUUID(ctx, tx, relationUUID, applicationID)
		if err != nil {
			return errors.Errorf("getting relation endpoint uuid: %w", err)
		}

		// Update the application settings specified in the settings argument.
		err = st.updateApplicationSettings(ctx, tx, endpointUUID, settings)
		if err != nil {
			return errors.Errorf("updating relation application settings: %w", err)
		}

		// Fetch all the new settings in the relation for this application.
		newSettings, err := st.getApplicationSettings(ctx, tx, endpointUUID)
		if err != nil {
			return errors.Errorf("getting new relation application settings: %w", err)
		}

		// Hash the new settings.
		hash, err := hashSettings(newSettings)
		if err != nil {
			return errors.Errorf("generating hash of relation application settings: %w", err)
		}

		// Update the hash in the database.
		err = st.updateApplicationSettingsHash(ctx, tx, endpointUUID, hash)
		if err != nil {
			return errors.Errorf("updating relation application settings hash: %w", err)
		}

		return nil
	})
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// GetRelationUnitSettings returns the relation unit settings for the given
// relation unit.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationUnitNotFound] is returned if the
//     unit is not part of the relation.
func (st *State) GetRelationUnitSettings(
	ctx context.Context,
	relationUnitUUID corerelation.UnitUUID,
) (map[string]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	var settings []relationSetting
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		relUnitExists, err := st.checkExistsByUUID(ctx, tx, "relation_unit", relationUnitUUID.String())
		if err != nil {
			return errors.Capture(err)
		} else if !relUnitExists {
			return relationerrors.RelationUnitNotFound
		}

		settings, err = st.getRelationUnitSettings(ctx, tx, relationUnitUUID)
		if err != nil {
			return errors.Capture(err)
		}

		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	relationSettings := make(map[string]string, len(settings))
	for _, setting := range settings {
		relationSettings[setting.Key] = setting.Value
	}
	return relationSettings, nil
}

// SetRelationUnitSettings records settings for a specific relation unit.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationUnitNotFound] is returned if relation unit does
//     not exist.
func (st *State) SetRelationUnitSettings(
	ctx context.Context,
	relationUnitUUID corerelation.UnitUUID,
	settings map[string]string,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Get the relation endpoint UUID.
		exists, err := st.checkExistsByUUID(ctx, tx, "relation_unit", relationUnitUUID.String())
		if err != nil {
			return errors.Errorf("checking relation unit exists: %w", err)
		} else if !exists {
			return relationerrors.RelationUnitNotFound
		}

		// Update the unit settings specified in the settings argument.
		err = st.updateUnitSettings(ctx, tx, relationUnitUUID, settings)
		if err != nil {
			return errors.Errorf("updating relation unit settings: %w", err)
		}

		// Fetch all the new settings in the relation for this unit.
		newSettings, err := st.getRelationUnitSettings(ctx, tx, relationUnitUUID)
		if err != nil {
			return errors.Errorf("getting new relation unit settings: %w", err)
		}

		// Hash the new settings.
		hash, err := hashSettings(newSettings)
		if err != nil {
			return errors.Errorf("generating hash of relation unit settings: %w", err)
		}

		// Update the hash in the database.
		err = st.updateUnitSettingsHash(ctx, tx, relationUnitUUID, hash)
		if err != nil {
			return errors.Errorf("updating relation unit settings hash: %w", err)
		}

		return nil
	})
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (st *State) updateApplicationSettingsHash(
	ctx context.Context,
	tx *sqlair.TX,
	endpointUUID, hash string,
) error {
	arg := applicationSettingsHash{
		RelationEndpointUUID: endpointUUID,
		Hash:                 hash,
	}
	stmt, err := st.Prepare(`
INSERT INTO relation_application_settings_hash (*) 
VALUES ($applicationSettingsHash.*) 
ON CONFLICT (relation_endpoint_uuid) DO UPDATE SET sha256 = excluded.sha256
`, applicationSettingsHash{})
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, arg).Run()
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

func hashSettings(settings []relationSetting) (string, error) {
	h := sha256.New()

	// Ensure we have a stable order for the keys.
	sort.Slice(settings, func(i, j int) bool {
		return settings[i].Key < settings[j].Key
	})

	for _, s := range settings {
		if _, err := h.Write([]byte(s.Key + " " + s.Value + " ")); err != nil {
			return "", errors.Errorf("writing relation setting: %w", err)
		}
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func (st *State) getApplicationSettings(
	ctx context.Context,
	tx *sqlair.TX,
	endpointUUID string,
) ([]relationSetting, error) {
	id := relationEndpointUUID{UUID: endpointUUID}
	stmt, err := st.Prepare(`
SELECT &relationSetting.*
FROM   relation_application_setting
WHERE  relation_endpoint_uuid = $relationEndpointUUID.uuid
`, id, relationSetting{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var settings []relationSetting
	err = tx.Query(ctx, stmt, id).GetAll(&settings)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Capture(err)
	}

	return settings, nil
}

// updateApplicationSettings updates the settings for a relation endpoint
// according to the provided settings map. If the value of a setting is empty
// then the setting is deleted, otherwise it is inserted/updated.
func (st *State) updateApplicationSettings(
	ctx context.Context,
	tx *sqlair.TX,
	endpointUUID string,
	settings map[string]string,
) error {
	if len(settings) == 0 {
		return nil
	}

	// Determine the keys to set and unset.
	var set []relationApplicationSetting
	var unset keys
	for k, v := range settings {
		if v == "" {
			unset = append(unset, k)
		} else {
			set = append(set, relationApplicationSetting{
				UUID:  endpointUUID,
				Key:   k,
				Value: v,
			})
		}
	}

	// Update the keys to set.
	updateStmt, err := st.Prepare(`
INSERT INTO relation_application_setting (*) 
VALUES ($relationApplicationSetting.*) 
ON CONFLICT (relation_endpoint_uuid, key) DO UPDATE SET value = excluded.value
`, relationApplicationSetting{})
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, updateStmt, set).Run()
	if err != nil {
		return errors.Capture(err)
	}

	// Delete the keys to unset.
	id := relationEndpointUUID{UUID: endpointUUID}
	deleteStmt, err := st.Prepare(`
DELETE FROM relation_application_setting
WHERE       relation_endpoint_uuid = $relationEndpointUUID.uuid
AND         key IN ($keys[:])
`, id, unset)
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, deleteStmt, id, unset).Run()
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (st *State) getRelationUnitSettings(
	ctx context.Context,
	tx *sqlair.TX,
	relUnitUUID corerelation.UnitUUID,
) ([]relationSetting, error) {
	id := relationUnitUUID{RelationUnitUUID: relUnitUUID}
	stmt, err := st.Prepare(`
SELECT &relationSetting.*
FROM   relation_unit_setting
WHERE  relation_unit_uuid = $relationUnitUUID.uuid
`, id, relationSetting{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var settings []relationSetting
	err = tx.Query(ctx, stmt, id).GetAll(&settings)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Capture(err)
	}

	return settings, nil
}

// updateUnitSettings updates the settings for a relation unit according to the
// provided settings map. If the value of a setting is empty then the setting is
// deleted, otherwise it is inserted/updated.
func (st *State) updateUnitSettings(
	ctx context.Context,
	tx *sqlair.TX,
	relUnitUUID corerelation.UnitUUID,
	settings map[string]string,
) error {
	if len(settings) == 0 {
		return nil
	}

	// Determine the keys to set and unset.
	var set []relationUnitSetting
	var unset keys
	for k, v := range settings {
		if v == "" {
			unset = append(unset, k)
		} else {
			set = append(set, relationUnitSetting{
				UUID:  relUnitUUID,
				Key:   k,
				Value: v,
			})
		}
	}

	// Update the keys to set.
	updateStmt, err := st.Prepare(`
INSERT INTO relation_unit_setting (*) 
VALUES ($relationUnitSetting.*) 
ON CONFLICT (relation_unit_uuid, key) DO UPDATE SET value = excluded.value
`, relationUnitSetting{})
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, updateStmt, set).Run()
	if err != nil {
		return errors.Capture(err)
	}

	// Delete the keys to unset.
	id := relationUnitUUID{RelationUnitUUID: relUnitUUID}
	deleteStmt, err := st.Prepare(`
DELETE FROM relation_unit_setting
WHERE       relation_unit_uuid = $relationUnitUUID.uuid
AND         key IN ($keys[:])
`, id, unset)
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, deleteStmt, id, unset).Run()
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (st *State) updateUnitSettingsHash(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID corerelation.UnitUUID,
	hash string,
) error {
	arg := unitSettingsHash{
		RelationUnitUUID: unitUUID,
		Hash:             hash,
	}
	stmt, err := st.Prepare(`
INSERT INTO relation_unit_settings_hash (*) 
VALUES ($unitSettingsHash.*) 
ON CONFLICT (relation_unit_uuid) DO UPDATE SET sha256 = excluded.sha256
`, unitSettingsHash{})
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, arg).Run()
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (st *State) getRelationEndpointUUID(
	ctx context.Context,
	tx *sqlair.TX,
	relationUUID corerelation.UUID,
	applicationID application.ID,
) (string, error) {
	id := relationAndApplicationUUID{
		RelationUUID:  relationUUID,
		ApplicationID: applicationID,
	}
	var endpointUUID relationEndpointUUID
	stmt, err := st.Prepare(`
SELECT re.uuid AS &relationEndpointUUID.uuid
FROM   application_endpoint ae
JOIN   relation_endpoint re ON re.endpoint_uuid = ae.uuid
WHERE  ae.application_uuid = $relationAndApplicationUUID.application_uuid
AND    re.relation_uuid = $relationAndApplicationUUID.relation_uuid
`, id, endpointUUID)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, id).Get(&endpointUUID)
	if errors.Is(err, sqlair.ErrNoRows) {
		// Check if we got no rows because the relation does not exist.
		relationExists, err := st.checkExistsByUUID(ctx, tx, "relation", relationUUID.String())
		if err != nil {
			return "", errors.Capture(err)
		} else if !relationExists {
			return "", relationerrors.RelationNotFound
		}
		// We got no rows because the application was not in the relation.
		return "", relationerrors.ApplicationNotFoundForRelation
	} else if err != nil {
		return "", errors.Capture(err)
	}

	return endpointUUID.UUID, nil
}

// GetPrincipalSubordinateApplicationIDs returns the Principal and
// Subordinate application IDs for the given unit. The principal will
// be the first ID returned and the subordinate will be the second. If
// the unit is not a subordinate, the second application ID will be
// empty.
//
// The following error types can be expected to be returned:
//   - [relationerrors.UnitNotAlive] if the unit is dead or dying, or
//     not found.
func (st *State) GetPrincipalSubordinateApplicationIDs(
	ctx context.Context,
	unitUUID unit.UUID,
) (application.ID, application.ID, error) {
	db, err := st.DB()
	if err != nil {
		return "", "", errors.Capture(err)
	}

	var principleAppID, subordinateAppID application.ID

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if alive, err := st.checkLife(ctx, tx, "unit", unitUUID.String(), life.IsAlive); err != nil {
			return errors.Errorf("cannot check unit %q life: %w", unitUUID, err)
		} else if !alive {
			return errors.Errorf("unit %s is not alive", unitUUID).Add(relationerrors.UnitNotAlive)
		}
		var principalUnit bool
		principleAppID, err = st.getPrincipalApplicationOfUnit(ctx, tx, unitUUID)
		if errors.Is(err, relationerrors.UnitPrincipalNotFound) {
			principalUnit = true
		} else if err != nil {
			return errors.Errorf("getting principal application of unit: %w", err)
		}

		unitApplicationID, err := st.getApplicationIDByUnitUUID(ctx, tx, unitUUID)
		if err != nil {
			return errors.Errorf("getting application of unit: %w", err)
		}

		if principalUnit {
			principleAppID = unitApplicationID
		} else {
			subordinateAppID = unitApplicationID
		}
		return err
	})
	return principleAppID, subordinateAppID, errors.Capture(err)
}

// InitialWatchLifeSuspendedStatus returns the two tables to watch for
// a relation's Life and Suspended status when the relation contains
// the provided application and the initial namespace query.
func (st *State) InitialWatchLifeSuspendedStatus(id application.ID) (string, string, eventsource.NamespaceQuery) {
	queryFunc := func(ctx context.Context, runner database.TxnRunner) ([]string, error) {
		stmt, err := st.Prepare(`
SELECT  re.relation_uuid AS &relationUUID.uuid
FROM    relation_endpoint re
JOIN    application_endpoint ae ON ae.uuid = re.endpoint_uuid
WHERE   ae.application_uuid = $applicationID.uuid
`, applicationID{}, relationUUID{})
		if err != nil {
			return nil, errors.Capture(err)
		}

		appID := applicationID{ID: id}

		var results []relationUUID
		err = runner.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			err := tx.Query(ctx, stmt, appID).GetAll(&results)
			if errors.Is(err, sqlair.ErrNoRows) {
				return nil
			}
			return errors.Capture(err)
		})
		if err != nil {
			return nil, errors.Errorf("querying requested applications that have pending charms: %w", err)
		}

		return transform.Slice(results, func(r relationUUID) string { return r.UUID.String() }), nil
	}

	return "relation", "relation_status", queryFunc
}

// WatcherApplicationSettingsNamespace returns the namespace string used for
// tracking application settings in the database.
func (st *State) WatcherApplicationSettingsNamespace() string {
	return "relation_application_setting"
}

// GetMapperDataForWatchLifeSuspendedStatus returns data needed to evaluate a relation
// uuid as part of WatchLifeSuspendedStatus eventmapper.
//
// The following error types can be expected to be returned:
//   - [relationerrors.ApplicationNotFoundForRelation] is returned if the
//     application is not part of the relation.
//   - [relationerrors.RelationNotFound] is returned if the relation UUID
//     is not found.
func (st *State) GetMapperDataForWatchLifeSuspendedStatus(
	ctx context.Context,
	relUUID corerelation.UUID,
	appID application.ID,
) (relation.RelationLifeSuspendedData, error) {
	db, err := st.DB()
	if err != nil {
		return relation.RelationLifeSuspendedData{}, errors.Capture(err)
	}

	data := watcherMapperData{
		RelationUUID: relUUID.String(),
		AppUUID:      appID.String(),
	}

	relAppStmt, err := st.Prepare(`
SELECT  re.relation_uuid AS &watcherMapperData.uuid
FROM    relation_endpoint re
JOIN    application_endpoint ae ON ae.uuid = re.endpoint_uuid
WHERE   ae.application_uuid = $watcherMapperData.application_uuid
AND     re.relation_uuid = $watcherMapperData.uuid
`, watcherMapperData{})
	if err != nil {
		return relation.RelationLifeSuspendedData{}, errors.Capture(err)
	}

	lifeStatusStmt, err := st.Prepare(`
SELECT (rst.name, l.value) AS (&watcherMapperData.*)
FROM   relation r
JOIN   life l ON r.life_id = l.id
JOIN   relation_status rs ON rs.relation_uuid = r.uuid
JOIN   relation_status_type rst ON rst.id = rs.relation_status_type_id
WHERE  r.uuid = $watcherMapperData.uuid
`, watcherMapperData{})
	if err != nil {
		return relation.RelationLifeSuspendedData{}, errors.Capture(err)
	}

	var endpoints []relation.Endpoint
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, relAppStmt, data).Get(&data)
		if errors.Is(err, sqlair.ErrNoRows) {
			return relationerrors.ApplicationNotFoundForRelation
		} else if err != nil {
			return errors.Errorf("verifying relation application intersection: %w", err)
		}

		err = tx.Query(ctx, lifeStatusStmt, data).Get(&data)
		if errors.Is(err, sqlair.ErrNoRows) {
			return relationerrors.RelationNotFound
		} else if err != nil {
			return errors.Errorf("getting relation life and status: %w", err)
		}

		endpoints, err = st.getRelationEndpoints(ctx, tx, corerelation.UUID(data.RelationUUID))
		if err != nil {
			return errors.Errorf("getting relation endpoints: %w", err)
		}
		return errors.Capture(err)
	})
	if err != nil {
		return relation.RelationLifeSuspendedData{}, errors.Capture(err)
	}

	endpointIdentifiers := make([]corerelation.EndpointIdentifier, len(endpoints))
	for i, endpoint := range endpoints {
		endpointIdentifiers[i] = endpoint.EndpointIdentifier()
	}

	return relation.RelationLifeSuspendedData{
		Life:                life.Value(data.Life),
		Suspended:           data.Suspended == corestatus.Suspended.String(),
		EndpointIdentifiers: endpointIdentifiers,
	}, nil
}

// checkCompatibleBases determines if the bases of two application endpoints
// are compatible for a relation.
// It compares the OS and channel of the base configurations for both endpoints.
// Returns an error if no compatible bases are found or if fetching bases fails.
func (st *State) checkCompatibleBases(ctx context.Context, tx *sqlair.TX, ep1 endpoint, ep2 endpoint) error {
	if ep1.Scope != charm.ScopeContainer && ep2.Scope != charm.ScopeContainer {
		return nil
	}

	app1Bases, err := st.getBases(ctx, tx, ep1)
	if err != nil {
		return errors.Errorf("getting bases of application %q: %w", ep1.ApplicationName, err)
	}
	app2Bases, err := st.getBases(ctx, tx, ep2)
	if err != nil {
		return errors.Errorf("getting bases of application %q: %w", ep2.ApplicationName, err)
	}

	for _, base1 := range app1Bases {
		for _, base2 := range app2Bases {
			if base1.IsCompatible(base2) {
				return nil
			}
		}
	}

	return errors.Errorf("no compatible bases found for application %q and %q", ep1.ApplicationName,
		ep2.ApplicationName).Add(relationerrors.CompatibleEndpointsNotFound)
}

// checkExistsByUUID checks if a record with the specified UUID exists in the given
// table using a transaction and context.
func (st *State) checkExistsByUUID(
	ctx context.Context,
	tx *sqlair.TX,
	table, uuid string,
) (bool, error) {
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

// checkEndpointCapacity validates whether adding a new relation to the given
// endpoint exceeds its defined capacity limit.
func (st *State) checkEndpointCapacity(ctx context.Context, tx *sqlair.TX, ep endpoint) error {
	type related struct {
		Count int `db:"count"`
	}
	countStmt, err := st.Prepare(`
SELECT count(*) AS &related.count
FROM   relation_endpoint
WHERE  endpoint_uuid = $endpoint.endpoint_uuid`, related{}, ep)
	if err != nil {
		return errors.Capture(err)
	}
	var found related
	if err = tx.Query(ctx, countStmt, ep).Get(&found); err != nil {
		return errors.Errorf("querying relation linked to endpoint %q: %w", ep, err)
	}
	if ep.Capacity > 0 && found.Count >= ep.Capacity {
		return errors.Errorf("establishing a new relation for %s would exceed its maximum relation limit of %d", ep,
			ep.Capacity).Add(relationerrors.EndpointQuotaLimitExceeded)
	}

	return nil
}

// checkLife retrieves the life state of an entity by its UUID from the specified table and evaluates it using a predicate.
func (st *State) checkLife(ctx context.Context, tx *sqlair.TX, table, uuid string, check life.Predicate) (bool, error) {
	type search struct {
		UUID string     `db:"uuid"`
		Life life.Value `db:"life"`
	}

	searched := search{UUID: uuid}
	query := fmt.Sprintf(`
SELECT 
    main.uuid AS &search.uuid,
    l.value AS &search.life
FROM   %s main
JOIN   life l ON main.life_id = l.id
WHERE  main.uuid = $search.uuid
`, table)
	checkStmt, err := st.Prepare(query, searched)
	if err != nil {
		return false, errors.Capture(err)
	}

	if err = tx.Query(ctx, checkStmt, searched).Get(&searched); err != nil {
		return false, errors.Errorf("query %q: %w", query, err)
	}
	return check(searched.Life), nil
}

// getAllRelations retrieves all relations from the database, returning a slice
// of relationUUID or an error.
func (st *State) getAllRelations(ctx context.Context, tx *sqlair.TX) ([]relationUUID, error) {
	var result []relationUUID

	stmt, err := st.Prepare(`
SELECT &relationUUID.*
FROM   relation
`, relationUUID{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	err = tx.Query(ctx, stmt).GetAll(&result)

	return result, errors.Capture(err)
}

// getCandidateEndpoints retrieves a list of candidate endpoints from the
// database matching the given identifier parameters.
// It queries based on application name and optionally endpoint name,
// returning all matching endpoints or an error.
func (st *State) getCandidateEndpoints(ctx context.Context, tx *sqlair.TX,
	identifier relation.CandidateEndpointIdentifier) ([]endpoint, error) {

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

// getBases retrieves a list of OS and channel information for a specific
// application, given an endpoint and transaction.
func (st *State) getBases(ctx context.Context, tx *sqlair.TX, ep1 endpoint) ([]corebase.Base, error) {
	stmt, err := st.Prepare(`
SELECT 
    ap.channel AS &applicationPlatform.channel,
    os.name AS &applicationPlatform.os
FROM application_platform ap
JOIN os ON ap.os_id = os.id
WHERE ap.application_uuid = $endpoint.application_uuid`, ep1, applicationPlatform{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	var appPlatforms []applicationPlatform
	err = tx.Query(ctx, stmt, ep1).GetAll(&appPlatforms)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("getting application platforms for %q: %w", ep1, err)
	}

	var result []corebase.Base
	for _, appPlatform := range appPlatforms {
		channel, err := corebase.ParseChannel(appPlatform.Channel)
		if err != nil {
			return nil, errors.Errorf("parsing channel %q: %w", appPlatform.Channel, err)
		}
		result = append(result, corebase.Base{
			OS:      appPlatform.OS,
			Channel: channel,
		})
	}

	return result, nil
}

func (st *State) getRelationDetails(ctx context.Context, tx *sqlair.TX, relationUUID corerelation.UUID) (relation.RelationDetailsResult, error) {
	type getRelation struct {
		UUID corerelation.UUID `db:"uuid"`
		ID   int               `db:"relation_id"`
		Life life.Value        `db:"value"`
	}
	rel := getRelation{
		UUID: relationUUID,
	}
	stmt, err := st.Prepare(`
SELECT (r.uuid, r.relation_id, l.value) AS (&getRelation.*)
FROM   relation r
JOIN   life l ON r.life_id = l.id
WHERE  r.uuid = $getRelation.uuid
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

	endpoints, err = st.getRelationEndpoints(ctx, tx, rel.UUID)
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

// inferEndpoints determines and validates the endpoints of a given relation
// based on the provided identifiers. It tries to find matching endpoint for both
// application, regarding the name of endpoint if provided, their interfaces,
// their roles and their scopes.
// If there is several matches or none, it will return an error.
func (st *State) inferEndpoints(
	ctx context.Context,
	tx *sqlair.TX,
	identifier1, identifier2 relation.CandidateEndpointIdentifier) (endpoint, endpoint, error) {

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

	// It is ok since we checked that both sides have candidates,
	// and all candidates from one side should have the same application
	app1UUID := endpoints1[0].ApplicationUUID
	app2UUID := endpoints2[0].ApplicationUUID

	// Check if applications are subordinates.
	isSubordinate1, err := st.isSubordinate(ctx, tx, app1UUID)
	if err != nil {
		return endpoint{}, endpoint{}, errors.Capture(err)
	}
	isSubordinate2, err := st.isSubordinate(ctx, tx, app2UUID)
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
		ep1 := e1.toRelationEndpoint()
		for _, e2 := range endpoints2 {
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

	if matchCount := len(matches); matchCount == 0 {
		return endpoint{}, endpoint{}, relationerrors.CompatibleEndpointsNotFound
	} else if matchCount > 1 {
		possibleMatches := make([]string, 0, matchCount)
		for _, match := range matches {
			possibleMatches = append(possibleMatches, fmt.Sprintf("\"%s %s\"", match.ep1, match.ep2))
		}
		return endpoint{}, endpoint{}, errors.Errorf("%w: %q could refer to %s",
			relationerrors.AmbiguousRelation, fmt.Sprintf("%s %s", identifier1, identifier2),
			strings.Join(possibleMatches, "; "))
	}

	return *matches[0].ep1, *matches[0].ep2, nil
}

// insertNewRelation creates a new relation entry in the database and returns its UUID or an error if the operation fails.
func (st *State) insertNewRelation(ctx context.Context, tx *sqlair.TX) (corerelation.UUID, error) {
	relUUID, err := corerelation.NewUUID()
	if err != nil {
		return relUUID, errors.Errorf("generating new relation UUID: %w", err)
	}

	relUUIDArg := relationIDAndUUID{
		UUID: relUUID,
	}

	stmtGetID, err := st.Prepare(`
SELECT sequence AS &relationIDAndUUID.relation_id 
FROM relation_sequence`, relUUIDArg)
	if err != nil {
		return relUUID, errors.Capture(err)
	}

	stmtUpdateID, err := st.Prepare(`
UPDATE relation_sequence SET sequence = sequence + 1`)
	if err != nil {
		return relUUID, errors.Capture(err)
	}

	stmtInsert, err := st.Prepare(`
INSERT INTO relation (uuid, life_id, relation_id)
VALUES ($relationIDAndUUID.uuid, 0, $relationIDAndUUID.relation_id)
`, relUUIDArg)
	if err != nil {
		return relUUID, errors.Capture(err)
	}

	if err := tx.Query(ctx, stmtGetID).Get(&relUUIDArg); err != nil {
		return relUUID, errors.Capture(err)
	}

	if err := tx.Query(ctx, stmtUpdateID).Run(); err != nil {
		return relUUID, errors.Capture(err)
	}

	if err := tx.Query(ctx, stmtInsert, relUUIDArg).Run(); err != nil {
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
VALUES ($setRelationEndpoint.*)`, endpoint)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, stmt, endpoint).Run(); err != nil {
		return errors.Capture(err)
	}

	return nil
}

// insertNewRelationStatus inserts a new relation status into the
// relation_status table in the database.
// It uses the provided context, transaction, and relation UUID to create a
// record with a status of 'joining'.
func (st *State) insertNewRelationStatus(ctx context.Context, tx *sqlair.TX, uuid corerelation.UUID) error {
	status := setRelationStatus{
		RelationUUID: uuid,
		Status:       corestatus.Joining,
		UpdatedAt:    st.clock.Now(),
	}

	stmt, err := st.Prepare(`
INSERT INTO relation_status (relation_uuid, relation_status_type_id, updated_at)
SELECT $setRelationStatus.relation_uuid, status.id, $setRelationStatus.updated_at
FROM   relation_status_type status
WHERE  status.name = $setRelationStatus.status`, status)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, stmt, status).Run(); err != nil {
		return errors.Capture(err)
	}
	return nil
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
FROM   relation r
JOIN   relation_endpoint e1 ON r.uuid = e1.relation_uuid
JOIN   relation_endpoint e2 ON r.uuid = e2.relation_uuid
WHERE  e1.endpoint_uuid = $endpoint1.endpoint_uuid                          
AND    e2.endpoint_uuid = $endpoint2.endpoint_uuid
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

func (st *State) getApplicationIDByUnitUUID(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID unit.UUID,
) (application.ID, error) {
	getApplication := getPrincipal{
		UnitUUID: unitUUID,
	}

	stmt, err := st.Prepare(`
SELECT &getPrincipal.application_uuid
FROM   unit u
WHERE  u.uuid = $getPrincipal.unit_uuid
`, getPrincipal{})
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, getApplication).Get(&getApplication)
	if errors.Is(sql.ErrNoRows, err) {
		return "", relationerrors.UnitNotFound
	} else if err != nil {
		return "", errors.Capture(err)
	}

	return getApplication.ApplicationUUID, nil
}
