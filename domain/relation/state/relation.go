// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/collections/set"
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
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainlife "github.com/juju/juju/domain/life"
	domainrelation "github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/relation/internal"
	sequencestate "github.com/juju/juju/domain/sequence/state"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

// State represents the relation domain state.
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
//   - [applicationerrors.ApplicationNotAlive] is returned if one of the
//     application is not alive.
//   - [relationerrors.BothEndpointsRemote] is returned if both endpoints are
//     remote applications.
//   - [relationerrors.CompatibleEndpointsNotFound] is returned if the endpoint
//     identifiers relates to correct endpoints yet not compatible (ex provider
//     with provider)
//   - [relationerrors.RelationAlreadyExists] is returned  if the inferred
//     relation already exists.
//   - [relationerrors.RelationEndpointNotFound] is returned if no endpoint can
//     be inferred from one of the identifier.
func (st *State) AddRelation(ctx context.Context, epIdentifier1, epIdentifier2 domainrelation.CandidateEndpointIdentifier, cidrs ...string) (
	domainrelation.Endpoint,
	domainrelation.Endpoint,
	error) {
	db, err := st.DB(ctx)
	if err != nil {
		return domainrelation.Endpoint{}, domainrelation.Endpoint{}, errors.Capture(err)
	}

	var endpoint1, endpoint2 domainrelation.Endpoint

	return endpoint1, endpoint2, db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Infers endpoint, ie get both application_endpoint_uuid.
		ep1, ep2, err := st.inferEndpoints(ctx, tx, epIdentifier1, epIdentifier2)
		if err != nil {
			return errors.Errorf("cannot relate endpoints %q and %q: %w",
				epIdentifier1,
				epIdentifier2, err)
		}

		// Make sure both endpoints are not remote applications.
		if err := st.checkBothEndpointsNotRemote(ctx, tx, ep1, ep2); err != nil {
			return errors.Capture(err)
		}

		id, err := sequencestate.NextValue(ctx, st, tx, domainrelation.SequenceNamespace)
		if err != nil {
			return errors.Errorf("getting next relation id: %w", err)
		}

		relUUID, err := st.addRelation(ctx, tx, ep1, ep2, id)
		if err != nil {
			return errors.Errorf("cannot add relation %q, %q: %w", epIdentifier1, epIdentifier2, err)
		}

		if len(cidrs) > 0 {
			// We only allow adding network egress for CMRs.
			isCMR, err := st.relationContainsRemoteApplication(ctx, tx, relUUID.String())
			if err != nil {
				return errors.Errorf("checking if relation %q is CMR: %w", relUUID, err)
			}
			if !isCMR {
				return errors.Errorf("integration via subnets for non cross model relations").Add(coreerrors.NotSupported)
			}
			if err := st.insertRelationNetworkEgress(ctx, tx, relUUID.String(), cidrs...); err != nil {
				return errors.Errorf("inserting network egress for relation %q: %w", relUUID, err)
			}
		}

		// Get endpoints from UUID.
		endpoints, err := st.getRelationEndpoints(ctx, tx, relUUID.String())
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

// InferRelationUUIDByEndpoints infers the relation based on two endpoints.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationNotFound] is returned if endpoints cannot be
//     found.
func (st *State) InferRelationUUIDByEndpoints(
	ctx context.Context,
	epIdentifier1, epIdentifier2 domainrelation.CandidateEndpointIdentifier,
) (corerelation.UUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var potentialUUIDs []relationUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		ep1, ep2, err := st.inferEndpoints(ctx, tx, epIdentifier1, epIdentifier2)
		if errors.Is(err, relationerrors.CompatibleEndpointsNotFound) ||
			errors.Is(err, relationerrors.RelationEndpointNotFound) ||
			errors.Is(err, relationerrors.AmbiguousRelation) {
			return relationerrors.RelationNotFound
		} else if err != nil {
			return errors.Errorf("inferring endpoints: %w", err)
		}

		potentialUUIDs, err = st.getRegularRelationUUIDByEndpointIdentifiers(
			ctx,
			tx,
			ep1.toEndpointIdentifier(),
			ep2.toEndpointIdentifier(),
		)
		if err != nil {
			return errors.Errorf("getting uuid for %q, %q: %w",
				ep1.String(), ep2.String(), err)
		}
		return nil
	})
	if err != nil {
		return "", errors.Capture(err)
	}

	if len(potentialUUIDs) > 1 {
		// This should never happen.
		return "", errors.Errorf("found multiple relations for endpoint pair")
	}

	return corerelation.UUID(potentialUUIDs[0].UUID), nil
}

func (st *State) addRelation(
	ctx context.Context,
	tx *sqlair.TX,
	ep1, ep2 Endpoint,
	id uint64,
) (corerelation.UUID, error) {
	var (
		relUUID corerelation.UUID
		err     error
	)
	// Check the relation doesn't already exist.
	if err := st.relationAlreadyExists(ctx, tx, ep1, ep2); err != nil {
		return relUUID, errors.Errorf("relation %s %s: %w", ep1, ep2, err)
	}

	// Check both application are alive
	if alive, err := st.checkLife(ctx, tx, "application", ep1.ApplicationUUID.String(), life.IsAlive); err != nil {
		return relUUID, errors.Errorf("relation %s %s: cannot check application life: %w", ep1, ep2, err)
	} else if !alive {
		return relUUID, errors.Errorf("relation %s %s: application %s is not alive", ep1, ep2, ep1.ApplicationName).
			Add(applicationerrors.ApplicationNotAlive)
	}
	if alive, err := st.checkLife(ctx, tx, "application", ep2.ApplicationUUID.String(), life.IsAlive); err != nil {
		return relUUID, errors.Errorf("relation %s %s: cannot check application life: %w", ep1, ep2, err)
	} else if !alive {
		return relUUID, errors.Errorf("relation %s %s: application %s is not alive", ep1, ep2, ep2.ApplicationName).
			Add(applicationerrors.ApplicationNotAlive)
	}

	// Check the application bases are compatible, if required
	if err := st.checkCompatibleBases(ctx, tx, ep1, ep2); err != nil {
		return relUUID, errors.Errorf("relation %s %s: %w", ep1, ep2, err)
	}

	// Check that adding a relation won't exceed any endpoint limit
	if err := st.checkEndpointCapacity(ctx, tx, ep1); err != nil {
		return relUUID, errors.Errorf("relation %s %s: %w", ep1, ep2, err)
	}
	if err := st.checkEndpointCapacity(ctx, tx, ep2); err != nil {
		return relUUID, errors.Errorf("relation %s %s: %w", ep1, ep2, err)
	}

	// The relation scope property is defined by the scopes of the endpoints.
	// It is container-scoped if either of the endpoints is container-scoped,
	// and global-scoped otherwise. This means it is possible for a container-
	// scoped relation to be plugged into a global-scoped endpoint, but not
	// vice versa.
	scope := charm.ScopeGlobal
	if ep1.Scope == charm.ScopeContainer || ep2.Scope == charm.ScopeContainer {
		scope = charm.ScopeContainer
	}

	// Insert a new relation with a new relation UUID.
	relUUID, err = st.insertNewRelation(ctx, tx, id, scope)
	if err != nil {
		return relUUID, errors.Errorf("inserting new relation: %w", err)
	}

	// Insert relation status.
	if err := st.insertNewRelationStatus(ctx, tx, relUUID); err != nil {
		return relUUID, errors.Errorf("inserting new relation %s %s: %w", ep1, ep2, err)
	}

	// Insert both relation_endpoint from application_endpoint_uuid and relation
	// uuid.
	if err := st.insertNewRelationEndpoint(ctx, tx, relUUID, ep1.ApplicationEndpointUUID); err != nil {
		return relUUID, errors.Errorf("inserting new relation endpoint for %q: %w", ep1.String(), err)
	}
	if err := st.insertNewRelationEndpoint(ctx, tx, relUUID, ep2.ApplicationEndpointUUID); err != nil {
		return relUUID, errors.Errorf("inserting new relation endpoint for %q: %w", ep2.String(), err)
	}
	return relUUID, nil
}

// ApplicationRelationsInfo returns all EndpointRelationData for an application.
// If the application is not in any relations, no error is returned.
//
// The following error types can be expected to be returned:
//   - [applicationerrors.ApplicationNotFound] is returned if application does not
//     exist.
func (st *State) ApplicationRelationsInfo(
	ctx context.Context,
	appID application.UUID,
) ([]domainrelation.EndpointRelationData, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var results []domainrelation.EndpointRelationData
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		found, err := st.checkApplicationExists(ctx, tx, appID.String())
		if err != nil {
			return errors.Capture(err)
		} else if !found {
			return applicationerrors.ApplicationNotFound
		}

		// Find every relation the application is part of.
		relationIDUUIDAppNames, err := st.getEveryRelationForApplicationID(ctx, tx, appID)
		if errors.Is(relationerrors.RelationNotFound, err) {
			// The application is not in any relations.
			return nil
		} else if err != nil {
			return errors.Capture(err)
		}

		// For each relation, get its EndpointsRelationData based on the
		// application.
		results = make([]domainrelation.EndpointRelationData, len(relationIDUUIDAppNames))
		for i, rel := range relationIDUUIDAppNames {
			result := domainrelation.EndpointRelationData{}
			eps, err := st.getRelationEndpointIdentifiers(ctx, tx, rel.UUID)
			if err != nil {
				return errors.Capture(err)
			}
			result.RelationID = rel.ID

			if len(eps) == 1 {
				result.Endpoint = eps[0].EndpointName
				result.RelatedEndpoint = eps[0].EndpointName
			} else {
				for _, ep := range eps {
					if ep.ApplicationName == rel.AppName {
						result.Endpoint = ep.EndpointName
					} else {
						result.RelatedEndpoint = ep.EndpointName
					}
				}
			}

			appData, err := st.getApplicationSettingsByRelAndApp(ctx, tx, rel.UUID, appID.String())
			if err != nil {
				return errors.Errorf("getting relation application settings: %w", err)
			}
			result.ApplicationData = convertSettings(appData)

			result.UnitRelationData, err = st.getUnitsRelationData(ctx, tx, rel.UUID, appID.String())
			if err != nil {
				return errors.Errorf("getting unit relation data: %w", err)
			}
			results[i] = result
		}
		return nil
	})

	return results, err
}

func (st *State) getApplicationSettingsByRelAndApp(
	ctx context.Context,
	tx *sqlair.TX,
	relationUUID, applicationID string,
) ([]relationSetting, error) {

	id := relationAndApplicationUUID{
		RelationUUID:  relationUUID,
		ApplicationID: applicationID,
	}

	stmt, err := st.Prepare(`
SELECT &relationSetting.*
FROM   relation_application_setting AS ras
JOIN   relation_endpoint AS re ON ras.relation_endpoint_uuid = re.uuid
JOIN   relation AS r ON re.relation_uuid = r.uuid
JOIN   application_endpoint AS ae ON re.endpoint_uuid = ae.uuid
WHERE  r.uuid = $relationAndApplicationUUID.relation_uuid
AND    ae.application_uuid = $relationAndApplicationUUID.application_uuid
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

// GetAllRelationDetails retrieves the details of all relations from the
// database. This includes relations with synthetic applications (i.e. CMRs).
// It returns the list of endpoints, the life status and identification data
// (UUID and ID) for each relation.
func (st *State) GetAllRelationDetails(ctx context.Context) ([]domainrelation.RelationDetailsResult, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var relationsDetails []domainrelation.RelationDetailsResult
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// TODO: Do this all in one query
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

// GetGoalStateRelationDataForApplication returns GoalStateRelationData for all
// relations the given application is in, modulo peer relations.
//
// The following error types can be expected to be returned:
//   - [applicationerrors.ApplicationNotFound] is returned if application UUID
//     doesn't refer an existing application.
func (st *State) GetGoalStateRelationDataForApplication(
	ctx context.Context,
	applicationID application.UUID,
) ([]domainrelation.GoalStateRelationData, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var dbResult []goalStateData
	stmt, err := st.Prepare(`
SELECT ep1.application_name AS &goalStateData.ep1_application_name,
       ep1.endpoint_name AS &goalStateData.ep1_endpoint_name,
       ep1.role AS &goalStateData.ep1_role,
       ep2.application_name AS &goalStateData.ep2_application_name,
       ep2.endpoint_name AS &goalStateData.ep2_endpoint_name,
       ep2.role AS &goalStateData.ep2_role,
       rs.status AS &goalStateData.status,
       rs.updated_at AS &goalStateData.updated_at
FROM   v_relation_endpoint AS ep1
JOIN   v_relation_endpoint AS ep2 ON ep1.relation_uuid = ep2.relation_uuid
JOIN   v_relation_status AS rs ON ep1.relation_uuid = rs.relation_uuid
WHERE  ep1.application_uuid = $applicationUUID.application_uuid
AND    ep1.application_uuid != ep2.application_uuid
`, goalStateData{}, applicationUUID{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		app := applicationUUID{UUID: applicationID.String()}
		err = tx.Query(ctx, stmt, app).GetAll(&dbResult)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	results := make([]domainrelation.GoalStateRelationData, len(dbResult))
	for i, rel := range dbResult {
		results[i] = rel.convertToGoalStateRelationData()
	}
	return results, nil
}

// GetOtherRelatedEndpointApplicationData returns an OtherApplicationForWatcher struct
// for the other Endpoint in a relation with the given application UUID.
//
// The following error types can be expected to be returned:
//   - [applicationerrors.ApplicationNotFound] is returned if application UUID
//     is not used in any relations or if the other relation applications
//     are not found.
func (st *State) GetOtherRelatedEndpointApplicationData(
	ctx context.Context,
	relUUID corerelation.UUID,
	applicationID application.UUID,
) (domainrelation.OtherApplicationForWatcher, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return domainrelation.OtherApplicationForWatcher{}, errors.Capture(err)
	}

	otherAppSub := otherApplicationsForWatcher{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Find all applications in a relation with the given application.
		otherApp, err := st.getOtherApplicationInRelations(ctx, tx, relUUID.String(), applicationID.String())
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
		return domainrelation.OtherApplicationForWatcher{}, errors.Capture(err)
	}

	return domainrelation.OtherApplicationForWatcher{
		ApplicationID: otherAppSub.AppID,
		Subordinate:   otherAppSub.Subordinate,
	}, nil
}

// getOtherApplicationInRelations returns the applications ID
// at the other end of the relation from the given application UUID.
func (st *State) getOtherApplicationInRelations(
	ctx context.Context,
	tx *sqlair.TX,
	relUUID, appID string,
) (string, error) {
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
// for each given application UUID.
func (st *State) getApplicationSubordinate(
	ctx context.Context,
	tx *sqlair.TX,
	app string,
) (otherApplicationsForWatcher, error) {

	appSubordinateStmt, err := st.Prepare(`
SELECT application_uuid AS &otherApplicationsForWatcher.application_uuid,
       subordinate AS &otherApplicationsForWatcher.subordinate
FROM   v_application_subordinate
WHERE  application_uuid = $entityUUID.uuid
`, otherApplicationsForWatcher{}, entityUUID{})
	if err != nil {
		return otherApplicationsForWatcher{}, errors.Errorf("preparing other application query: %w", err)
	}

	appID := entityUUID{UUID: app}
	otherApp := otherApplicationsForWatcher{}
	err = tx.Query(ctx, appSubordinateStmt, appID).Get(&otherApp)
	if err != nil {
		return otherApplicationsForWatcher{}, errors.Capture(err)
	}

	return otherApp, nil
}

// GetRelationLifeSuspendedNameSpace returns the namespace for watching
// a relation's life and suspended state.'
func (st *State) GetRelationLifeSuspendedNameSpace() string {
	return "custom_relation_life_suspended"
}

// GetRelationLifeSuspendedStatus returns a life/suspended status
// struct for a specified relation uuid.
func (st *State) GetRelationLifeSuspendedStatus(
	ctx context.Context,
	relationUUID string,
) (internal.RelationLifeSuspendedStatus, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return internal.RelationLifeSuspendedStatus{}, errors.Capture(err)
	}

	var relationLifeSuspended lifeAndSuspended
	var endpoints []domainrelation.Endpoint
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		relationLifeSuspended, err = st.getRelationLifeAndSuspended(ctx, tx, relationUUID)
		if err != nil {
			return errors.Errorf("getting relation life and suspended status: %w", err)
		}
		endpoints, err = st.getRelationEndpoints(ctx, tx, relationUUID)
		if err != nil {
			return errors.Errorf("getting relation endpoints: %w", err)
		}
		return nil
	})
	if errors.Is(err, coreerrors.NotFound) {
		return internal.RelationLifeSuspendedStatus{}, errors.Capture(relationerrors.RelationNotFound)
	} else if err != nil {
		return internal.RelationLifeSuspendedStatus{}, errors.Capture(err)
	}

	return internal.RelationLifeSuspendedStatus{
		Life:            relationLifeSuspended.Life,
		Suspended:       relationLifeSuspended.Suspended,
		SuspendedReason: relationLifeSuspended.Reason,
		Endpoints:       endpoints,
	}, nil
}

// GetRelationUUIDByID returns the relation UUID based on the relation ID.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationNotFound] is returned if the relation UUID
//     relating to the relation ID cannot be found.
func (st *State) GetRelationUUIDByID(ctx context.Context, relationID int) (corerelation.UUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	id := relationIDAndUUID{
		ID: uint64(relationID),
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
// at the intersection of the relationUUID and applicationUUID.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationNotFound] is returned if the relation UUID
//     relating to the relation ID cannot be found.
func (st *State) GetRelationEndpointScope(
	ctx context.Context,
	relUUID corerelation.UUID,
	appID application.UUID,
) (charm.RelationScope, error) {
	db, err := st.DB(ctx)
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
AND     application_uuid = $entityUUID.uuid
`, scope{}, entityUUID{}, relationUUID{})
	if err != nil {
		return "", errors.Capture(err)
	}

	rel := relationUUID{
		UUID: relUUID.String(),
	}
	app := entityUUID{UUID: appID.String()}
	var output scope
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Check if the relation exists.
		relationFound, err := st.checkRelationExistsByUUID(ctx, tx, relUUID.String())
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
// It queries the database using the provided application UUID and relation UUID
// arguments.
//
// The following error types can be expected to be returned:
//   - [applicationerrors.ApplicationNotFound] is returned if the application
//     is not found.
//   - [relationerrors.RelationEndpointNotFound] is returned if the relation
//     Endpoint is not found.
//   - [relationerrors.RelationNotFound] is returned if the relation UUID
//     is not found.
func (st *State) GetRelationEndpointUUID(ctx context.Context, args domainrelation.GetRelationEndpointUUIDArgs) (
	corerelation.EndpointUUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	type relationEndpointArgs struct {
		AppID        string `db:"application_uuid"`
		RelationUUID string `db:"relation_uuid"`
	}
	dbArgs := relationEndpointArgs{
		AppID:        args.ApplicationUUID.String(),
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
			appFound, err := st.checkApplicationExists(ctx, tx, args.ApplicationUUID.String())
			if err != nil {
				return errors.Capture(err)
			}
			// Check if the relation exists.
			relationFound, err := st.checkRelationExistsByUUID(ctx, tx, args.RelationUUID.String())
			if err != nil {
				return errors.Capture(err)
			}
			var errs []error
			if !appFound {
				errs = append(errs, errors.Errorf("%w: %s", applicationerrors.ApplicationNotFound, args.ApplicationUUID))
			}
			if !relationFound {
				errs = append(errs, errors.Errorf("%w: %s", relationerrors.RelationNotFound, args.RelationUUID))
			}
			if len(errs) > 0 {
				return errors.Join(errs...)
			}
			return errors.Errorf("relationUUID %q with applicationID %q: %w",
				args.RelationUUID, args.ApplicationUUID, relationerrors.RelationEndpointNotFound)

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
) ([]domainrelation.RelationUnitStatusResult, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	type relationUnitStatus struct {
		RelationUUID string `db:"relation_uuid"`
		InScope      bool   `db:"in_scope"`
		Suspended    bool   `db:"suspended"`
	}
	type unitUUIDArg struct {
		UUID string `db:"unit_uuid"`
	}

	uuid := unitUUIDArg{
		UUID: unitUUID.String(),
	}

	stmt, err := st.Prepare(`
WITH in_scope_relation AS (
    SELECT re.relation_uuid
    FROM relation_endpoint AS re
    JOIN relation_unit AS ru ON re.uuid = ru.relation_endpoint_uuid
    WHERE ru.unit_uuid = $unitUUIDArg.unit_uuid
)
SELECT re.relation_uuid AS &relationUnitStatus.relation_uuid,
       re.relation_uuid IN in_scope_relation AS &relationUnitStatus.in_scope,
       r.suspended AS &relationUnitStatus.suspended
FROM      relation_endpoint AS re
JOIN      relation AS r ON re.relation_uuid = r.uuid
JOIN      application_endpoint AS ae ON re.endpoint_uuid = ae.uuid
JOIN      unit AS u ON ae.application_uuid = u.application_uuid
WHERE     u.uuid = $unitUUIDArg.unit_uuid
`, uuid, relationUnitStatus{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var relationUnitStatuses []domainrelation.RelationUnitStatusResult
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var statuses []relationUnitStatus
		err := tx.Query(ctx, stmt, uuid).GetAll(&statuses)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}

		for _, status := range statuses {
			endpoints, err := st.getRelationEndpoints(ctx, tx, status.RelationUUID)
			if err != nil {
				return errors.Errorf("getting endpoints of relation %q: %w", status.RelationUUID, err)
			}

			relationUnitStatuses = append(relationUnitStatuses, domainrelation.RelationUnitStatusResult{
				Endpoints: endpoints,
				InScope:   status.InScope,
				Suspended: status.Suspended,
			})
		}

		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return relationUnitStatuses, nil
}

// GetRelationEndpoints returns the relation's endpoints.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationNotFound] is returned if the relation UUID
//     is not found.
func (st *State) GetRelationEndpoints(
	ctx context.Context,
	relationUUID string,
) ([]domainrelation.Endpoint, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var endpoints []domainrelation.Endpoint
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		endpoints, err = st.getRelationEndpoints(ctx, tx, relationUUID)
		if err != nil {
			return errors.Errorf("getting relation endpoints: %w", err)
		}
		return errors.Capture(err)
	})
	return endpoints, errors.Capture(err)
}

// getRelationEndpoints retrieves the relation.ConsumerApplicationEndpoint of the specified relation.
func (st *State) getRelationEndpoints(
	ctx context.Context,
	tx *sqlair.TX,
	uuid string,
) ([]domainrelation.Endpoint, error) {
	id := relationUUID{
		UUID: uuid,
	}

	stmt, err := st.Prepare(`
SELECT &Endpoint.*
FROM   v_relation_endpoint
WHERE  relation_uuid = $relationUUID.uuid
`, id, Endpoint{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var endpoints []Endpoint
	err = tx.Query(ctx, stmt, id).GetAll(&endpoints)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, relationerrors.RelationNotFound
	} else if err != nil {
		return nil, errors.Capture(err)
	}

	if length := len(endpoints); length > 2 {
		return nil, errors.Errorf("internal error: expected 1 or 2 endpoints in relation, got %d", length)
	}

	var relationEndpoints []domainrelation.Endpoint
	for _, e := range endpoints {
		relationEndpoints = append(relationEndpoints, e.toRelationEndpoint())
	}

	return relationEndpoints, nil
}

// exportEndpoints gets information needed to export the endpoints of a
// relation.
func (st *State) exportRelationEndpoints(
	ctx context.Context,
	tx *sqlair.TX,
	uuid string,
) ([]exportEndpoint, error) {
	id := relationUUID{
		UUID: uuid,
	}
	stmt, err := st.Prepare(`
SELECT &exportEndpoint.*
FROM   v_relation_endpoint
WHERE  relation_uuid = $relationUUID.uuid
ORDER BY endpoint_name
`, id, exportEndpoint{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var endpoints []exportEndpoint
	err = tx.Query(ctx, stmt, id).GetAll(&endpoints)
	if err != nil {
		return nil, errors.Capture(err)
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
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var uuid []relationUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		uuid, err = st.getRegularRelationUUIDByEndpointIdentifiers(
			ctx,
			tx,
			endpoint1,
			endpoint2,
		)
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

func (st *State) getRegularRelationUUIDByEndpointIdentifiers(
	ctx context.Context,
	tx *sqlair.TX,
	endpoint1, endpoint2 corerelation.EndpointIdentifier,
) ([]relationUUID, error) {
	var uuid []relationUUID
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
		return uuid, errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, e1, e2).GetAll(&uuid)
	if errors.Is(err, sqlair.ErrNoRows) {
		return uuid, relationerrors.RelationNotFound
	}
	return uuid, errors.Capture(err)
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
	db, err := st.DB(ctx)
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
func (st *State) GetRelationDetails(ctx context.Context, relationUUID corerelation.UUID) (domainrelation.RelationDetailsResult, error) {
	db, err := st.DB(ctx)
	var result domainrelation.RelationDetailsResult
	if err != nil {
		return result, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result, err = st.getRelationDetails(ctx, tx, relationUUID.String())
		return errors.Capture(err)
	})
	return result, errors.Capture(err)
}

// GetRelationUnitChanges retrieves changes to relation unit states and
// application settings for the provided UUIDs.
// It takes a list of unit UUIDs and application UUIDs, returning the
// current setting version for each one, or departed if any unit is not found
//
// Note: a not found unit is assumed as departed, since this method is intended
// to be called after the domain has notified through a watcher that "some rows
// in relation_unit have been created, updated or deleted". So the only cause of
// having a unit UUID as a parameter here and not finding a related row in
// the relation_unit table is that the unit has departed, and thus, the related
// row has been deleted.
func (st *State) GetRelationUnitChanges(
	ctx context.Context, unitUUIDs []unit.UUID, appUUIDs []application.UUID,
) (domainrelation.RelationUnitsChange, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return domainrelation.RelationUnitsChange{}, errors.Capture(err)
	}

	type uuids []string
	type change struct {
		UUID string `db:"uuid"`
		Name string `db:"name"`
		Hash string `db:"sha256"`
	}

	unitStmt, err := st.Prepare(`
SELECT 
    ru.unit_uuid AS &change.uuid,
    u.name AS &change.name,
    rush.sha256 AS &change.sha256
FROM relation_unit AS ru
JOIN unit AS u ON ru.unit_uuid = u.uuid
LEFT JOIN  relation_unit_settings_hash AS rush ON ru.uuid = rush.relation_unit_uuid
WHERE ru.unit_uuid IN ($uuids[:])`, change{}, uuids{})
	if err != nil {
		return domainrelation.RelationUnitsChange{}, errors.Capture(err)
	}

	appStmt, err := st.Prepare(`
SELECT 
    ae.application_uuid AS &change.uuid,
    a.name AS &change.name,
    rash.sha256 AS &change.sha256
FROM application_endpoint AS ae
JOIN application AS a ON ae.application_uuid = a.uuid
JOIN relation_endpoint AS re ON ae.uuid = re.endpoint_uuid 
LEFT JOIN relation_application_settings_hash AS rash ON re.uuid = rash.relation_endpoint_uuid
WHERE ae.application_uuid IN ($uuids[:])`, change{}, uuids{})
	if err != nil {
		return domainrelation.RelationUnitsChange{}, errors.Capture(err)
	}

	departedStmt, err := st.Prepare(`
SELECT &getUnit.*
FROM unit AS u
WHERE u.uuid IN ($uuids[:])`, getUnit{}, uuids{})
	if err != nil {
		return domainrelation.RelationUnitsChange{}, errors.Capture(err)
	}

	var appChanges, unitChanges []change
	var departedUnits []getUnit
	apps := transform.Slice(appUUIDs, application.UUID.String)
	units := transform.Slice(unitUUIDs, unit.UUID.String)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, unitStmt, uuids(units)).GetAll(&unitChanges)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("failed to get relation unit changes: %w", err)
		}
		err = tx.Query(ctx, appStmt, uuids(apps)).GetAll(&appChanges)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("failed to get relation application changes: %w", err)
		}

		// Compute departed units, which are requested units not found into the unitChanges
		// (means we found them somehow, but there are not there anymore)
		requested := set.NewStrings(transform.Slice(unitUUIDs, unit.UUID.String)...)
		for _, c := range unitChanges {
			requested.Remove(c.UUID)
		}

		err = tx.Query(ctx, departedStmt, uuids(requested.Values())).GetAll(&departedUnits)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("failed to get relation application changes: %w", err)
		}

		return nil
	})
	if err != nil {
		return domainrelation.RelationUnitsChange{}, errors.Capture(err)
	}

	return domainrelation.RelationUnitsChange{
		Changed: transform.SliceToMap(unitChanges, func(c change) (unit.Name, int64) {
			return unit.Name(c.Name), hashToInt(c.Hash)
		}),
		AppChanged: transform.SliceToMap(appChanges, func(c change) (string, int64) {
			return c.Name, hashToInt(c.Hash)
		}),
		Departed: transform.Slice(departedUnits, func(f getUnit) unit.Name { return f.Name }),
	}, nil
}

// GetRelationUnitUUID retrieves the UUID of a relation unit based on the given
// relation UUID and unit name.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationUnitNotFound] if the relation unit cannot be
//     found.
func (st *State) GetRelationUnitUUID(
	ctx context.Context,
	relationUUID corerelation.UUID,
	unitName unit.Name,
) (corerelation.UnitUUID, error) {
	db, err := st.DB(ctx)
	var result corerelation.UnitUUID
	if err != nil {
		return result, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result, _, err = st.getRelationUnit(ctx, tx, relationUUID, unitName)
		return err
	})
	return result, errors.Capture(err)
}

// GetFullRelationUnitsChange returns RelationUnitChange for the given relation
// application pair.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationNotFound] if the relation  cannot be
//     found.
func (st *State) GetFullRelationUnitsChange(
	ctx context.Context,
	relationUUID corerelation.UUID,
	applicationUUID application.UUID,
) (domainrelation.FullRelationUnitChange, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return domainrelation.FullRelationUnitChange{}, errors.Capture(err)
	}

	var settings []relationSetting
	var unitRelationData map[string]domainrelation.RelationData
	var relationLifeSuspended lifeAndSuspended
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		relationLifeSuspended, err = st.getRelationLifeAndSuspended(ctx, tx, relationUUID.String())
		if err != nil {
			return errors.Capture(err)
		}

		settings, err = st.getApplicationSettingsByRelAndApp(ctx, tx, relationUUID.String(), applicationUUID.String())
		if err != nil {
			return errors.Capture(err)
		}

		unitRelationData, err = st.getUnitsRelationData(ctx, tx, relationUUID.String(), applicationUUID.String())
		if err != nil {
			return errors.Errorf("getting unit relation data: %w", err)
		}

		return nil
	})
	if errors.Is(err, coreerrors.NotFound) {
		return domainrelation.FullRelationUnitChange{}, errors.Capture(relationerrors.RelationNotFound)
	} else if err != nil {
		return domainrelation.FullRelationUnitChange{}, errors.Capture(err)
	}

	// Transform the data into RelationUnitChange.
	appSettings := transform.SliceToMap(settings, func(s relationSetting) (string, string) {
		return s.Key, s.Value
	})
	inScopeUnits := make([]int, 0)
	allUnits := make([]int, 0, len(unitRelationData))
	unitSettings := make([]domainrelation.UnitSettings, 0)
	for unitName, data := range unitRelationData {
		id := unit.Name(unitName).Number()
		allUnits = append(allUnits, id)
		if !data.InScope {
			continue
		}
		inScopeUnits = append(inScopeUnits, id)
		if len(data.UnitData) == 0 {
			continue
		}
		unitSettings = append(unitSettings, domainrelation.UnitSettings{
			UnitID:   id,
			Settings: data.UnitData,
		})
	}
	return domainrelation.FullRelationUnitChange{
		RelationUnitChange: domainrelation.RelationUnitChange{
			RelationUUID:        relationUUID,
			Life:                relationLifeSuspended.Life,
			UnitsSettings:       unitSettings,
			AllUnits:            allUnits,
			InScopeUnits:        inScopeUnits,
			ApplicationSettings: appSettings,
		},
		Suspended:       relationLifeSuspended.Suspended,
		SuspendedReason: relationLifeSuspended.Reason,
	}, nil
}

// GetRelationUnitsChanges returns RelationUnitChange for the given relation
// application pair.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationNotFound] if the relation  cannot be
//     found.
func (st *State) GetRelationUnitsChanges(
	ctx context.Context,
	relationUUID corerelation.UUID,
	applicationUUID application.UUID,
) (domainrelation.RelationUnitChange, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return domainrelation.RelationUnitChange{}, errors.Capture(err)
	}

	var settings []relationSetting
	var unitRelationData map[string]domainrelation.RelationData
	var relationLife life.Value
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		relationLife, err = st.getRelationLife(ctx, tx, relationUUID.String())
		if err != nil {
			return errors.Capture(err)
		}

		settings, err = st.getApplicationSettingsByRelAndApp(ctx, tx, relationUUID.String(), applicationUUID.String())
		if err != nil {
			return errors.Capture(err)
		}

		unitRelationData, err = st.getUnitsRelationData(ctx, tx, relationUUID.String(), applicationUUID.String())
		if err != nil {
			return errors.Errorf("getting unit relation data: %w", err)
		}

		return nil
	})
	if errors.Is(err, coreerrors.NotFound) {
		return domainrelation.RelationUnitChange{}, errors.Capture(relationerrors.RelationNotFound)
	} else if err != nil {
		return domainrelation.RelationUnitChange{}, errors.Capture(err)
	}

	// Transform the data into RelationUnitChange.
	appSettings := transform.SliceToMap(settings, func(s relationSetting) (string, string) {
		return s.Key, s.Value
	})
	inScopeUnits := make([]int, 0)
	allUnits := make([]int, 0, len(unitRelationData))
	unitSettings := make([]domainrelation.UnitSettings, 0)
	for unitName, data := range unitRelationData {
		id := unit.Name(unitName).Number()
		allUnits = append(allUnits, id)
		if !data.InScope {
			continue
		}
		inScopeUnits = append(inScopeUnits, id)
		if len(data.UnitData) == 0 {
			continue
		}
		unitSettings = append(unitSettings, domainrelation.UnitSettings{
			UnitID:   id,
			Settings: data.UnitData,
		})
	}
	return domainrelation.RelationUnitChange{
		RelationUUID:        relationUUID,
		Life:                relationLife,
		UnitsSettings:       unitSettings,
		AllUnits:            allUnits,
		InScopeUnits:        inScopeUnits,
		ApplicationSettings: appSettings,
	}, nil
}

// getRelationUnit returns the unit UUID and the relation unit UUID for the
// given relation and unit name.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationUnitNotFound] if the relation unit cannot be
//     found.
func (st *State) getRelationUnit(
	ctx context.Context,
	tx *sqlair.TX,
	relationUUID corerelation.UUID,
	unitName unit.Name,
) (corerelation.UnitUUID, unit.UUID, error) {
	args := getRelationUnit{
		RelationUUID: relationUUID,
		Name:         unitName,
	}
	stmt, err := st.Prepare(`
SELECT 
       ru.uuid AS &getRelationUnit.relation_unit_uuid,
       u.uuid  AS &getRelationUnit.unit_uuid
FROM   relation_unit ru
JOIN   unit AS u ON ru.unit_uuid = u.uuid 
JOIN   relation_endpoint AS re ON ru.relation_endpoint_uuid = re.uuid
WHERE  u.name = $getRelationUnit.name
AND    re.relation_uuid = $getRelationUnit.relation_uuid`, args)
	if err != nil {
		return "", "", errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, args).Get(&args)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", "", errors.Errorf("unit %q: %w", unitName, relationerrors.RelationUnitNotFound)
	}
	if err != nil {
		return "", "", errors.Errorf("getting unit: %w", err)
	}
	return args.RelationUnitUUID, args.UnitUUID, nil
}

// IsPeerRelation returns a boolean to indicate if the given
// relation UUID is for a peer relation.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationNotFound] if the relation cannot be found.
func (st *State) IsPeerRelation(ctx context.Context, relUUID string) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	countStmt, err := st.Prepare(`
SELECT count(*) AS &rows.count
FROM   relation_endpoint
WHERE  relation_uuid = $entityUUID.uuid`, rows{}, entityUUID{})
	if err != nil {
		return false, errors.Capture(err)
	}

	var found rows
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err = tx.Query(ctx, countStmt, entityUUID{UUID: relUUID}).Get(&found); err != nil {
			return errors.Errorf("querying relation endpoints for uuid %q: %w", relUUID, err)
		}
		return nil
	})

	if err != nil {
		return false, errors.Capture(err)
	}

	if found.Count == 0 {
		err = relationerrors.RelationNotFound
	}

	return found.Count == 1, err
}

// EnterScope indicates that the provided unit has joined the relation.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationNotFound] if the relation cannot be found.
//   - [applicationerrors.UnitNotFound] if no unit by the given name can be found
//   - [relationerrors.PotentialRelationUnitNotValid] if the unit entering scope
//     is a subordinate and its endpoint has scope charm.ScopeContainer, but the
//     principal application of the unit is not the application in the relation.
//   - [relationerrors.CannotEnterScopeNotAlive] if the unit or relation is not
//     alive.
func (st *State) EnterScope(
	ctx context.Context,
	relationUUID corerelation.UUID,
	unitName unit.Name,
	settings map[string]string,
) error {
	db, err := st.DB(ctx)
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
			return applicationerrors.UnitNotFound
		} else if err != nil {
			return errors.Capture(err)
		}

		// Check the unit can enter scope in this relation.
		err := st.checkUnitCanEnterScope(ctx, tx, relationUUID.String(), unitArgs.UUID.String())
		if err != nil {
			return errors.Errorf("checking unit valid in relation: %w", err)
		}

		// Upsert the row recording that the unit has entered scope.
		relationUnitUUID, err := st.insertRelationUnit(ctx, tx, relationUUID.String(), unitArgs.UUID.String())
		if err != nil {
			return errors.Capture(err)
		}

		// Set the relation unit settings.
		err = st.setRelationUnitSettings(ctx, tx, relationUnitUUID, settings)
		if err != nil {
			return errors.Errorf("setting relation unit settings: %w", err)
		}

		return nil
	})
	return errors.Capture(err)
}

// NeedsSubordinateUnit checks if there is a subordinate application
// related to the principal unit that needs a subordinate unit created whilst
// entering scope.
//
// In the case that all the following hold, parameters for creating a
// subordinate unit will be returned:
//   - The unit and relation are alive.
//   - The unit is in the relation.
//   - The relation is container scoped.
//   - The relation relates a subordinate application to a principal application.
//   - The unit is on the principal application.
//   - The unit does not already have a subordinate unit from the subordinate app.
//
// Unless one of the error cases is matched below, nil will be returned.
//
// The following errors can be return:
//   - [relationerrors.CannotEnterScopeNotAlive] if the unit or relation is not
//     alive.
//   - [relationerrors.CannotEnterScopeSubordinateNotAlive] if a subordinate unit
//     already exists, but is not alive.
func (st *State) NeedsSubordinateUnit(
	ctx context.Context,
	relationUUID corerelation.UUID,
	principalUnitName unit.Name,
) (*application.UUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var result *application.UUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Find the unit in the relation.
		relUnitUUID, unitUUID, err := st.getRelationUnit(ctx, tx, relationUUID, principalUnitName)
		if err != nil {
			return errors.Errorf("getting relation unit: %w", err)
		}

		// Check the relation is alive.
		if alive, err := st.checkLife(ctx, tx, "relation", relationUUID.String(), life.IsAlive); err != nil {
			return errors.Errorf("getting relation life: %w", err)
		} else if !alive {
			return relationerrors.CannotEnterScopeNotAlive
		}

		// Check the unit is alive.
		if alive, err := st.checkLife(ctx, tx, "unit", unitUUID.String(), life.IsAlive); err != nil {
			return errors.Errorf("getting unit life: %w", err)
		} else if !alive {
			return relationerrors.CannotEnterScopeNotAlive
		}

		// Check that we are in a container scoped relation.
		scope, err := st.getRelationScope(ctx, tx, relationUUID.String())
		if err != nil {
			return errors.Errorf("getting relation scope: %w", err)
		} else if scope != string(charm.ScopeContainer) {
			return nil
		}

		// Get the ID of the related subordinate application, if it exists.
		subAppID, relatedSubExists, err := st.findRelatedSubordinateApplication(ctx, tx, relUnitUUID)
		if err != nil {
			return errors.Errorf("getting related subordinate application: %w", err)
		} else if !relatedSubExists {
			return nil
		}

		// Check if there is already a subordinate unit.
		if exists, err := st.subordinateUnitExists(ctx, tx, subAppID, unitUUID); err != nil {
			return errors.Errorf("checking if subordinate already exists: %w", err)
		} else if exists {
			return nil
		}

		result = &subAppID
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return result, nil
}

// findRelatedSubordinateApplication returns the application UUID of the related
// subordinate application there is one, if there is not, it returns false as
// the boolean argument.
func (st *State) findRelatedSubordinateApplication(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID corerelation.UnitUUID,
) (application.UUID, bool, error) {
	type getSub struct {
		UnitUUID               corerelation.UnitUUID `db:"unit_uuid"`
		Subordinate            bool                  `db:"subordinate"`
		PrincipalApplicationID application.UUID      `db:"application_uuid"`
	}

	arg := getSub{
		UnitUUID: unitUUID,
	}
	stmt, err := st.Prepare(`
SELECT (cm.subordinate, ae.application_uuid) AS (&getSub.*)
FROM   relation_unit ru
JOIN   relation_endpoint re1 ON ru.relation_endpoint_uuid = re1.uuid
JOIN   relation_endpoint re2 ON re2.relation_uuid = re1.relation_uuid AND re1.uuid != re2.uuid 
JOIN   application_endpoint ae ON ae.uuid = re2.endpoint_uuid
JOIN   charm_relation cr ON cr.uuid = ae.charm_relation_uuid
JOIN   charm_metadata cm ON cm.charm_uuid = cr.charm_uuid
WHERE  ru.uuid = $getSub.unit_uuid
`, arg)
	if err != nil {
		return "", false, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, arg).Get(&arg)
	if errors.Is(err, sqlair.ErrNoRows) {
		// Peer relations will return no rows, so will units not in relations.
		// Return false for these.
		return "", false, nil
	}
	if err != nil {
		return "", false, errors.Capture(err)
	}

	return arg.PrincipalApplicationID, arg.Subordinate, nil
}

// subordinateUnitExists checks if the principal unit already has a subordinate
// unit of the given application.
//
// If the subordinate unit exists but is not alive
// [relationerrors.CannotEnterScopeSubordinateNotAlive] is returned.
func (st *State) subordinateUnitExists(
	ctx context.Context,
	tx *sqlair.TX,
	subordinateAppID application.UUID,
	principalUnit unit.UUID,
) (bool, error) {
	type getSub struct {
		PrincipalUnitUUID        unit.UUID        `db:"unit_uuid"`
		SubordinateApplicationID application.UUID `db:"application_uuid"`
		SubordinateLife          life.Value       `db:"value"`
	}
	arg := getSub{
		PrincipalUnitUUID:        principalUnit,
		SubordinateApplicationID: subordinateAppID,
	}
	stmt, err := st.Prepare(`
SELECT (u.application_uuid, l.value) AS (&getSub.*)
FROM   unit_principal AS up
JOIN   unit AS u ON u.uuid = up.unit_uuid
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

	if arg.SubordinateLife != life.Alive {
		return false, relationerrors.CannotEnterScopeSubordinateNotAlive
	}

	return true, nil
}

// checkUnitCanEnterScope checks that the unit can enter scope in the given
// relation.
func (st *State) checkUnitCanEnterScope(ctx context.Context, tx *sqlair.TX, relationUUID, unitUUID string) error {
	// Check relation is alive.
	relationLife, err := st.getRelationLife(ctx, tx, relationUUID)
	if errors.Is(err, coreerrors.NotFound) {
		return relationerrors.RelationNotFound
	} else if err != nil {
		return errors.Errorf("getting relation life: %w", err)
	}
	if relationLife != life.Alive {
		return relationerrors.CannotEnterScopeNotAlive
	}

	// Check unit is alive.
	unitLife, err := st.getUnitLife(ctx, tx, unitUUID)
	if errors.Is(err, coreerrors.NotFound) {
		return applicationerrors.UnitNotFound
	} else if err != nil {
		return errors.Errorf("getting unit life: %w", err)
	}
	if unitLife != life.Alive {
		return relationerrors.CannotEnterScopeNotAlive
	}

	// Get the IDs of the applications in the relation.
	appIDs, err := st.getApplicationsInRelation(ctx, tx, relationUUID)
	if err != nil {
		return errors.Errorf("getting applications in relation: %w", err)
	}

	// Get the ID of the application for the unit trying to enter scope.
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
		var otherAppID string
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

func (st *State) getRelationLife(ctx context.Context, tx *sqlair.TX, uuid string) (life.Value, error) {
	args := getLife{
		UUID: uuid,
	}
	stmt, err := st.Prepare(`
SELECT &getLife.*
FROM   relation t
JOIN   life l ON t.life_id = l.id
WHERE  t.uuid = $getLife.uuid
`, args)
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

func (st *State) getRelationLifeAndSuspended(ctx context.Context, tx *sqlair.TX, uuid string) (lifeAndSuspended, error) {
	arg := entityUUID{
		UUID: uuid,
	}
	stmt, err := st.Prepare(`
SELECT &lifeAndSuspended.*
FROM   relation t
JOIN   life l ON t.life_id = l.id
WHERE  t.uuid = $entityUUID.uuid
`, arg, lifeAndSuspended{})
	if err != nil {
		return lifeAndSuspended{}, errors.Capture(err)
	}
	var result lifeAndSuspended
	err = tx.Query(ctx, stmt, arg).Get(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return lifeAndSuspended{}, coreerrors.NotFound
	} else if err != nil {
		return lifeAndSuspended{}, errors.Capture(err)
	}

	return result, nil
}

func (st *State) getUnitLife(ctx context.Context, tx *sqlair.TX, uuid string) (life.Value, error) {
	args := getLife{
		UUID: uuid,
	}
	stmt, err := st.Prepare(`
SELECT &getLife.*
FROM   unit t
JOIN   life l ON t.life_id = l.id
WHERE  t.uuid = $getLife.uuid
`, args)
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
func (st *State) getApplicationOfUnit(ctx context.Context, tx *sqlair.TX, unitUUID string) (string, error) {
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
		return "", applicationerrors.UnitNotFound
	} else if err != nil {
		return "", errors.Capture(err)
	}

	return args.ApplicationUUID, nil
}

// getApplicationsInRelation gets all the applications that are in the given
// relation.
func (st *State) getApplicationsInRelation(ctx context.Context, tx *sqlair.TX, uuid string) ([]string, error) {
	relUUID := entityUUID{
		UUID: uuid,
	}
	stmt, err := st.Prepare(`
SELECT &applicationUUID.application_uuid
FROM   relation_endpoint re 
JOIN   application_endpoint ae ON re.endpoint_uuid = ae.uuid
WHERE  re.relation_uuid = $entityUUID.uuid
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

	var ids []string
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
// Then the subordinate unit cannot enter a relation unless its principal
// application is the one in the relation. In this case, the error
// [relationerrors.PotentialRelationUnitNotValid] is returned.
//
// The above scenario can happen when a subordinate application is deployed then
// related to multiple principal applications. The units of the single
// subordinate application can have different principal applications depending
// on which machine they are on. When the subordinate application is related to
// a new principal application, watchers will trigger for all of its units, and
// they will all try to enter the relation scope. They should only succeed if
// they are the units of the new principal application, otherwise the error is
// returned.
func (st *State) checkSubordinateUnitCanEnterScope(
	ctx context.Context, tx *sqlair.TX, relUUID, unitUUID, otherApplication string,
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
	if scope != string(charm.ScopeContainer) {
		return nil
	}

	// Check that the principal application of the unit is the other application
	// in the relation the unit is trying to enter scope in.
	principalAppID, err := st.getPrincipalApplicationOfUnit(ctx, tx, unitUUID)
	if err != nil {
		return errors.Errorf("getting principal application of unit: %w", err)
	}
	if principalAppID != otherApplication {
		return errors.Errorf(
			"unit cannot enter scope: principal application not in relation: %w",
			relationerrors.PotentialRelationUnitNotValid,
		)
	}

	return nil
}

func (st *State) getRelationScope(
	ctx context.Context,
	tx *sqlair.TX,
	uuid string,
) (string, error) {
	relUUID := entityUUID{
		UUID: uuid,
	}
	getScopeStmt, err := st.Prepare(`
SELECT rs.name AS &scope.scope
FROM   relation AS r
JOIN   charm_relation_scope AS rs ON r.scope_id = rs.id
WHERE  uuid = $entityUUID.uuid
`, relUUID, scope{})
	if err != nil {
		return "", errors.Capture(err)
	}

	var scope scope
	err = tx.Query(ctx, getScopeStmt, relUUID).Get(&scope)
	if err != nil {
		return "", errors.Capture(err)
	}

	return scope.Scope, nil
}

// hashToInt converts the first 8 bytes of a hash string into an int64
// using little-endian byte order.
// If the hash is shorter than 8 bytes, it pads it with zero bytes.
// Returns 0 for an empty hash.
func hashToInt(hash string) int64 {
	sha := []byte(hash)
	if len(sha) < 8 {
		// Avoid panic in case of empty or short hash.
		sha = append(sha, make([]byte, 8-len(sha))...)
	}
	// Empty hash should return 0, since we will have zero-bits.
	uintValue := binary.LittleEndian.Uint64(sha[:8])
	return int64(uintValue)
}

// isSubordinate returns true if the application is a subordinate application.
func (st *State) isSubordinate(
	ctx context.Context,
	tx *sqlair.TX,
	applicationUUID string,
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

// getPrincipalApplicationOfUnit returns the UUID of the principal application
// of a unit. If the unit has no principal application, then the error
// [relationerrors.UnitPrincipalNotFound] is returned (this means the error is
// always returned when the unit is not a subordinate).
func (st *State) getPrincipalApplicationOfUnit(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID string,
) (string, error) {
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

// insertRelationUnit inserts a relation unit record if it doesn't exist.
func (st *State) insertRelationUnit(
	ctx context.Context, tx *sqlair.TX, relationUUID, unitUUID string,
) (string, error) {
	// Check if a relation_unit record already exists for this unit.
	getRelationUnit := relationUnit{
		RelationUUID: relationUUID,
		UnitUUID:     unitUUID,
	}
	getRelationUnitStmt, err := st.Prepare(`
SELECT  ru.uuid AS &relationUnit.uuid
FROM    relation_unit AS ru
JOIN    relation_endpoint AS re ON ru.relation_endpoint_uuid = re.uuid
WHERE   re.relation_uuid = $relationUnit.relation_uuid
AND     ru.unit_uuid = $relationUnit.unit_uuid
`, getRelationUnit)
	if err != nil {
		return "", errors.Capture(err)
	}
	err = tx.Query(ctx, getRelationUnitStmt, getRelationUnit).Get(&getRelationUnit)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Capture(err)
	}
	if err == nil {
		// If there is already a relation unit in the table,
		// it means it is in scope
		return getRelationUnit.RelationUnitUUID, nil
	}

	// Insert a new relation unit
	uuid, err := corerelation.NewUnitUUID()
	if err != nil {
		return "", errors.Capture(err)
	}
	insertRelationUnit := relationUnit{
		RelationUnitUUID: uuid.String(),
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
		return "", errors.Capture(err)
	}

	var outcome sqlair.Outcome
	if err := tx.Query(ctx, insertStmt, insertRelationUnit).Get(&outcome); err != nil {
		return "", errors.Capture(err)
	}

	if num, err := outcome.Result().RowsAffected(); err != nil {
		return "", errors.Capture(err)
	} else if num != 1 {
		return "", errors.Errorf("expected 1 row inserted, got %d", num)
	}

	return uuid.String(), nil
}

// GetRelationApplicationSettings returns the application settings
// for the given application and relation identifier combination.
//
// The following error types can be expected to be returned:
//   - [applicationerrors.ApplicationNotFoundForRelation] is returned if the
//     application is not part of the relation.
//   - [relationerrors.RelationNotFound] is returned if the relation UUID
//     is not found.
func (st *State) GetRelationApplicationSettings(
	ctx context.Context,
	relationUUID corerelation.UUID,
	applicationID application.UUID,
) (map[string]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var settings []relationSetting
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		endpointUUID, err := st.getRelationEndpointUUID(ctx, tx, relationUUID.String(), applicationID.String())
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

// SetRelationApplicationAndUnitSettings records settings for a unit and
// an application in a relation.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationUnitNotFound] is returned if the
//     relation unit is not found.
func (st *State) SetRelationApplicationAndUnitSettings(
	ctx context.Context,
	relationUnitUUID corerelation.UnitUUID,
	applicationSettings, unitSettings map[string]string,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.setRelationUnitSettings(ctx, tx, relationUnitUUID.String(), unitSettings)
		if err != nil {
			return errors.Errorf("setting relation unit settings: %w", err)
		}

		relationUUID, applicationUUID, err := st.getRelationAndApplicationOfRelationUnit(ctx, tx, relationUnitUUID.String())
		if err != nil {
			return errors.Capture(err)
		}

		err = st.setRelationApplicationSettings(ctx, tx, relationUUID, applicationUUID, applicationSettings)
		if err != nil {
			return errors.Errorf("setting relation unit settings: %w", err)
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
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var settings []relationSetting
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		relUnitExists, err := st.checkRelationUnitExistsByUUID(ctx, tx, relationUnitUUID.String())
		if err != nil {
			return errors.Capture(err)
		} else if !relUnitExists {
			return relationerrors.RelationUnitNotFound
		}

		settings, err = st.getRelationUnitSettings(ctx, tx, relationUnitUUID.String())
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

// GetRelationUnitSettingsArchive returns the archived settings for the input
// unit name and relation UUID.
func (st *State) GetRelationUnitSettingsArchive(
	ctx context.Context, relUUID, unitName string,
) (map[string]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	type name struct {
		Name string `db:"unit_name"`
	}

	rUUID := entityUUID{UUID: relUUID}
	uName := name{Name: unitName}
	stmt, err := st.Prepare(`
SELECT &relationSetting.*
FROM   relation_unit_setting_archive
WHERE  relation_uuid = $entityUUID.uuid
AND    unit_name = $name.unit_name
`, rUUID, uName, relationSetting{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var settings []relationSetting
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, rUUID, uName).GetAll(&settings)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
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
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.setRelationUnitSettings(ctx, tx, relationUnitUUID.String(), settings)
	})
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (st *State) setRelationUnitSettings(
	ctx context.Context,
	tx *sqlair.TX,
	relationUnitUUID string,
	settings map[string]string,
) error {
	// If the settings are nil then there is nothing to do. Do not check for
	// length of 0, as that is valid for deleting all settings.
	if settings == nil {
		return nil
	}

	// Get the relation endpoint UUID.
	exists, err := st.checkRelationUnitExistsByUUID(ctx, tx, relationUnitUUID)
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
	if len(set) > 0 {
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
	}

	// Delete the keys to unset.
	if len(unset) > 0 {
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
	}
	return nil
}

func (st *State) getRelationUnitSettings(
	ctx context.Context, tx *sqlair.TX, relUnitUUID string,
) ([]relationSetting, error) {
	id := entityUUID{UUID: relUnitUUID}
	stmt, err := st.Prepare(`
SELECT &relationSetting.*
FROM   relation_unit_setting
WHERE  relation_unit_uuid = $entityUUID.uuid
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
	ctx context.Context, tx *sqlair.TX, relUnitUUID string, settings map[string]string,
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
	if len(set) > 0 {
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
	}

	// Delete the keys to unset.
	if len(unset) > 0 {
		id := entityUUID{UUID: relUnitUUID}
		deleteStmt, err := st.Prepare(`
DELETE FROM relation_unit_setting
WHERE       relation_unit_uuid = $entityUUID.uuid
AND         key IN ($keys[:])
`, id, unset)
		if err != nil {
			return errors.Capture(err)
		}
		err = tx.Query(ctx, deleteStmt, id, unset).Run()
		if err != nil {
			return errors.Capture(err)
		}
	}

	return nil
}

func (st *State) updateUnitSettingsHash(ctx context.Context, tx *sqlair.TX, unitUUID string, hash string) error {
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
	relationUUID string,
	applicationID string,
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
		relationExists, err := st.checkRelationExistsByUUID(ctx, tx, relationUUID)
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

// GetPrincipalSubordinateApplicationUUIDs returns the Principal and Subordinate
// application UUIDs for the given unit. The principal will be the first UUID
// returned and the subordinate will be the second. If the unit is not a
// subordinate, the second application UUID will be empty.
//
// The following error types can be expected to be returned:
//   - [relationerrors.UnitDead] if the unit is dead or not found.
func (st *State) GetPrincipalSubordinateApplicationUUIDs(
	ctx context.Context,
	unitUUID unit.UUID,
) (principal application.UUID, subordinate application.UUID, err error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", "", errors.Capture(err)
	}

	var principalAppID, subordinateAppID string

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if alive, err := st.checkLife(ctx, tx, "unit", unitUUID.String(), life.IsNotDead); err != nil {
			return errors.Errorf("cannot check unit %q life: %w", unitUUID, err)
		} else if !alive {
			return errors.Errorf("unit %s is dead", unitUUID).Add(relationerrors.UnitDead)
		}
		var principalUnit bool
		principalAppID, err = st.getPrincipalApplicationOfUnit(ctx, tx, unitUUID.String())
		if errors.Is(err, relationerrors.UnitPrincipalNotFound) {
			principalUnit = true
		} else if err != nil {
			return errors.Errorf("getting principal application of unit: %w", err)
		}

		unitApplicationID, err := st.getApplicationIDByUnitUUID(ctx, tx, unitUUID.String())
		if err != nil {
			return errors.Errorf("getting application of unit: %w", err)
		}

		if principalUnit {
			principalAppID = unitApplicationID
		} else {
			subordinateAppID = unitApplicationID
		}
		return err
	})
	return application.UUID(principalAppID), application.UUID(subordinateAppID), errors.Capture(err)
}

// ApplicationExists returns nil if the application with the given UUID exists,
//
// The following error types can be expected to be returned:
//   - [applicationerrors.ApplicationNotFound] if the application does not exist.
func (st *State) ApplicationExists(ctx context.Context, id application.UUID) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	var exists bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		exists, err = st.checkApplicationExists(ctx, tx, id.String())
		return errors.Capture(err)
	})
	if err != nil {
		return errors.Capture(err)
	} else if !exists {
		return applicationerrors.ApplicationNotFound
	}

	return nil
}

// InitialWatchLifeSuspendedStatus returns the two tables to watch for a
// relation's Life and Suspended status when the relation contains the provided
// application and the initial namespace query.
func (st *State) InitialWatchLifeSuspendedStatus(id application.UUID) (string, eventsource.NamespaceQuery) {
	queryFunc := func(ctx context.Context, runner database.TxnRunner) ([]string, error) {
		stmt, err := st.Prepare(`
SELECT  re.relation_uuid AS &relationUUID.uuid
FROM    relation_endpoint re
JOIN    application_endpoint ae ON ae.uuid = re.endpoint_uuid
WHERE   ae.application_uuid = $entityUUID.uuid
`, entityUUID{}, relationUUID{})
		if err != nil {
			return nil, errors.Capture(err)
		}

		appID := entityUUID{UUID: id.String()}

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

		return transform.Slice(results, func(r relationUUID) string { return r.UUID }), nil
	}

	return "relation", queryFunc
}

// WatcherApplicationSettingsNamespace returns the namespace string used for
// tracking application settings in the database.
func (st *State) WatcherApplicationSettingsNamespace() string {
	return "relation_application_settings_hash"
}

// GetWatcherRelationUnitsData returns the data used to the RelationsUnits
// watcher: relation endpoint UUID and namespaces.
func (st *State) GetWatcherRelationUnitsData(
	ctx context.Context,
	relationUUID corerelation.UUID,
	applicationUUID application.UUID,
) (internal.WatcherRelationUnitsData, error) {
	relationEndpointUUID, err := st.GetRelationEndpointUUID(ctx, domainrelation.GetRelationEndpointUUIDArgs{
		ApplicationUUID: applicationUUID,
		RelationUUID:    relationUUID,
	})
	if err != nil {
		return internal.WatcherRelationUnitsData{}, errors.Capture(err)
	}

	return internal.WatcherRelationUnitsData{
		RelationEndpointUUID:      relationEndpointUUID.String(),
		RelationUnitNS:            "custom_relation_unit_by_endpoint_uuid",
		ApplicationSettingsHashNS: "relation_application_settings_hash",
		UnitSettingsHashNS:        "relation_unit_settings_hash",
	}, nil
}

// GetRelationUnitUUIDsByEndpointUUID returns all unit relation uuids for the
// provided relation endpoint uuid.
func (st *State) GetRelationUnitUUIDsByEndpointUUID(ctx context.Context, relationEndpointUUID string) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	input := entityUUID{UUID: relationEndpointUUID}
	stmt, err := st.Prepare(`
SELECT uuid AS &entityUUID.uuid
FROM   relation_unit
WHERE  relation_endpoint_uuid = $entityUUID.uuid
`, entityUUID{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	var output []entityUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, input).GetAll(&output)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return transform.Slice(output, func(in entityUUID) string {
		return in.UUID
	}), nil
}

// GetMapperDataForWatchLifeSuspendedStatus returns data needed to evaluate a
// relation uuid as part of WatchLifeSuspendedStatus eventmapper.
//
// The following error types can be expected to be returned:
//   - [applicationerrors.ApplicationNotFoundForRelation] is returned if the
//     application is not part of the relation.
//   - [relationerrors.RelationNotFound] is returned if the relation UUID
//     is not found.
func (st *State) GetMapperDataForWatchLifeSuspendedStatus(
	ctx context.Context,
	relUUID corerelation.UUID,
	appID application.UUID,
) (domainrelation.RelationLifeSuspendedData, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return domainrelation.RelationLifeSuspendedData{}, errors.Capture(err)
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
		return domainrelation.RelationLifeSuspendedData{}, errors.Capture(err)
	}

	lifeSuspendedStmt, err := st.Prepare(`
SELECT (r.suspended, l.value) AS (&watcherMapperData.*)
FROM   relation r
JOIN   life l ON r.life_id = l.id
WHERE  r.uuid = $watcherMapperData.uuid
`, watcherMapperData{})
	if err != nil {
		return domainrelation.RelationLifeSuspendedData{}, errors.Capture(err)
	}

	var endpoints []domainrelation.Endpoint
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, relAppStmt, data).Get(&data)
		if errors.Is(err, sqlair.ErrNoRows) {
			return relationerrors.ApplicationNotFoundForRelation
		} else if err != nil {
			return errors.Errorf("verifying relation application intersection: %w", err)
		}

		err = tx.Query(ctx, lifeSuspendedStmt, data).Get(&data)
		if errors.Is(err, sqlair.ErrNoRows) {
			return relationerrors.RelationNotFound
		} else if err != nil {
			return errors.Errorf("getting relation life and suspended state: %w", err)
		}

		endpoints, err = st.getRelationEndpoints(ctx, tx, data.RelationUUID)
		if err != nil {
			return errors.Errorf("getting relation endpoints: %w", err)
		}
		return errors.Capture(err)
	})
	if err != nil {
		return domainrelation.RelationLifeSuspendedData{}, errors.Capture(err)
	}

	endpointIdentifiers := make([]corerelation.EndpointIdentifier, len(endpoints))
	for i, endpoint := range endpoints {
		endpointIdentifiers[i] = endpoint.EndpointIdentifier()
	}

	return domainrelation.RelationLifeSuspendedData{
		Life:                life.Value(data.Life),
		Suspended:           data.Suspended,
		EndpointIdentifiers: endpointIdentifiers,
	}, nil
}

// GetConsumerRelationUnitsChange returns the versions of the relation units
// settings and any departed units.
func (st *State) GetConsumerRelationUnitsChange(
	ctx context.Context,
	relationUUID, applicationUUID string,
) (domainrelation.ConsumerRelationUnitsChange, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return domainrelation.ConsumerRelationUnitsChange{}, errors.Capture(err)
	}

	var appSettingsVersion, unitsSettingsVersions map[string]int64
	var departed []string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		appSettingsVersion, err = st.getApplicationSettingsVersion(ctx, tx, relationUUID, applicationUUID)
		if err != nil {
			return errors.Errorf("getting application %q settings version: %w", applicationUUID, err)
		}
		unitsSettingsVersions, err = st.getUnitsSettingsVersions(ctx, tx, relationUUID, applicationUUID)
		if err != nil {
			return errors.Errorf("getting units settings versions: %w", err)
		}
		departed, err = st.getDepartedUnits(ctx, tx, relationUUID, applicationUUID)
		if err != nil {
			return errors.Errorf("getting departed units for relation %q: %w", relationUUID, err)
		}
		return nil
	})
	if err != nil {
		return domainrelation.ConsumerRelationUnitsChange{}, errors.Capture(err)
	}

	return domainrelation.ConsumerRelationUnitsChange{
		AppSettingsVersion:    appSettingsVersion,
		DepartedUnits:         departed,
		UnitsSettingsVersions: unitsSettingsVersions,
	}, nil
}

func (st *State) getApplicationSettingsVersion(
	ctx context.Context,
	tx *sqlair.TX,
	relationUUID, applicationUUID string,
) (map[string]int64, error) {
	stmt, err := st.Prepare(`
SELECT (rash.sha256, a.name) AS (&nameAndHash.*)
FROM   relation_application_settings_hash AS rash
JOIN   relation_endpoint AS re ON rash.relation_endpoint_uuid = re.uuid
JOIN   application_endpoint AS ae ON re.endpoint_uuid = ae.uuid
JOIN   application AS a ON ae.application_uuid = a.uuid
WHERE  a.uuid = $relationAndApplicationUUID.application_uuid
AND    re.relation_uuid = $relationAndApplicationUUID.relation_uuid
`, relationAndApplicationUUID{}, nameAndHash{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	relAndApp := relationAndApplicationUUID{
		RelationUUID:  relationUUID,
		ApplicationID: applicationUUID,
	}
	var result nameAndHash
	err = tx.Query(ctx, stmt, relAndApp).Get(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Capture(err)
	}
	return map[string]int64{result.Name: hashToInt(result.Hash)}, nil
}

func (st *State) getUnitsSettingsVersions(
	ctx context.Context,
	tx *sqlair.TX,
	relationUUID, applicationUUID string,
) (map[string]int64, error) {
	stmt, err := st.Prepare(`
SELECT (rush.sha256, u.name) AS (&nameAndHash.*)
FROM   relation_unit_settings_hash AS rush
JOIN   relation_unit AS ru ON rush.relation_unit_uuid = ru.uuid
JOIN   relation_endpoint AS re ON ru.relation_endpoint_uuid = re.uuid
JOIN   application_endpoint AS ae ON re.endpoint_uuid = ae.uuid
JOIN   application AS a ON ae.application_uuid = a.uuid
JOIN   unit AS u ON a.uuid = u.application_uuid AND ru.unit_uuid = u.uuid
WHERE  a.uuid = $relationAndApplicationUUID.application_uuid
AND    re.relation_uuid = $relationAndApplicationUUID.relation_uuid
`, relationAndApplicationUUID{}, nameAndHash{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	relAndApp := relationAndApplicationUUID{
		RelationUUID:  relationUUID,
		ApplicationID: applicationUUID,
	}
	var result []nameAndHash
	err = tx.Query(ctx, stmt, relAndApp).GetAll(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Capture(err)
	}
	return transform.SliceToMap(result, func(in nameAndHash) (string, int64) {
		return in.Name, hashToInt(in.Hash)
	}), nil
}

func (st *State) getDepartedUnits(
	ctx context.Context,
	tx *sqlair.TX,
	relationUUID, applicationUUID string,
) ([]string, error) {
	stmt, err := st.Prepare(`
WITH in_scope_units AS (
    SELECT u.uuid
    FROM   unit AS u 
    JOIN   application AS a ON u.application_uuid = a.uuid
    JOIN   application_endpoint AS ae ON a.uuid = ae.application_uuid
    JOIN   relation_endpoint AS re ON ae.uuid = re.endpoint_uuid
    JOIN   relation_unit AS ru ON re.uuid = ru.relation_endpoint_uuid AND u.uuid = ru.unit_uuid
    WHERE  a.uuid = $relationAndApplicationUUID.application_uuid
    AND    re.relation_uuid = $relationAndApplicationUUID.relation_uuid
)
SELECT u.name AS &name.name
FROM   unit AS u 
JOIN   application AS a ON u.application_uuid = a.uuid
WHERE  a.uuid = $relationAndApplicationUUID.application_uuid
AND    u.uuid NOT IN in_scope_units
`, relationAndApplicationUUID{}, name{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	relAndApp := relationAndApplicationUUID{
		RelationUUID:  relationUUID,
		ApplicationID: applicationUUID,
	}
	var result []name
	err = tx.Query(ctx, stmt, relAndApp).GetAll(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Capture(err)
	}
	return transform.Slice(result, func(in name) string {
		return in.Name
	}), nil
}

func (st *State) getRelationUnits(
	ctx context.Context,
	tx *sqlair.TX,
	endpointUUID string,
) ([]relationUnitUUIDAndName, error) {
	relEndpoint := relationEndpointUUID{UUID: endpointUUID}
	stmt, err := st.Prepare(`
SELECT (ru.uuid, u.name) AS (&relationUnitUUIDAndName.*)
FROM   relation_unit ru
JOIN   unit u ON u.uuid = ru.unit_uuid
WHERE  ru.relation_endpoint_uuid = $relationEndpointUUID.uuid
`, relationUnitUUIDAndName{}, relEndpoint)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var relUnits []relationUnitUUIDAndName
	err = tx.Query(ctx, stmt, relEndpoint).GetAll(&relUnits)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Capture(err)
	}

	return relUnits, nil
}

// checkCompatibleBases determines if the bases of two application endpoints
// are compatible for a relation.
// It compares the OS and channel of the base configurations for both endpoints.
// Returns an error if no compatible bases are found or if fetching bases fails.
func (st *State) checkCompatibleBases(ctx context.Context, tx *sqlair.TX, ep1 Endpoint, ep2 Endpoint) error {
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

// checkApplicationExists checks if the application with the specified UUID
// exists in the given table using a transaction and context.
func (st *State) checkApplicationExists(ctx context.Context, tx *sqlair.TX, uuid string) (bool, error) {
	appUUID := entityUUID{UUID: uuid}
	checkStmt, err := st.Prepare(`
SELECT &entityUUID.*
FROM   application 
WHERE  uuid = $entityUUID.uuid
`, appUUID)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, checkStmt, appUUID).Get(&appUUID)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Errorf("getting application %q: %w", uuid, err)
	}
	return true, nil
}

// checkRelationExistsByUUID checks if a record with the specified relation UUID
// exists in the given table using a transaction and context.
func (st *State) checkRelationExistsByUUID(
	ctx context.Context,
	tx *sqlair.TX,
	uuid string,
) (bool, error) {
	relationUUID := entityUUID{UUID: uuid}
	query := `
SELECT &entityUUID.* 
FROM   relation 
WHERE  uuid = $entityUUID.uuid
`
	checkStmt, err := st.Prepare(query, relationUUID)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, checkStmt, relationUUID).Get(&relationUUID)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Errorf("query %q: %w", query, err)
	}
	return true, nil
}

// checkRelationUnitExistsByUUID checks if a record with the specified relation
// unit UUID exists in the given table using a transaction and context.
func (st *State) checkRelationUnitExistsByUUID(
	ctx context.Context,
	tx *sqlair.TX,
	uuid string,
) (bool, error) {
	relationUnitUUID := entityUUID{UUID: uuid}
	query := `
SELECT &entityUUID.* 
FROM   relation_unit
WHERE  uuid = $entityUUID.uuid
`
	checkStmt, err := st.Prepare(query, relationUnitUUID)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, checkStmt, relationUnitUUID).Get(&relationUnitUUID)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Errorf("query %q: %w", query, err)
	}
	return true, nil
}

// checkEndpointCapacity validates whether adding a new relation to the given
// endpoint exceeds its defined capacity limit.
func (st *State) checkEndpointCapacity(ctx context.Context, tx *sqlair.TX, ep Endpoint) error {
	countStmt, err := st.Prepare(`
SELECT count(*) AS &rows.count
FROM   relation_endpoint
WHERE  endpoint_uuid = $Endpoint.application_endpoint_uuid`, rows{}, ep)
	if err != nil {
		return errors.Capture(err)
	}
	var found rows
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
	identifier domainrelation.CandidateEndpointIdentifier) ([]Endpoint, error) {

	epIdentifier := endpointIdentifier{
		ApplicationName: identifier.ApplicationName,
		EndpointName:    identifier.EndpointName,
	}

	stmt, err := st.Prepare(`
SELECT &Endpoint.*
FROM   v_application_endpoint
WHERE  application_name = $endpointIdentifier.application_name
AND    (
    endpoint_name    = $endpointIdentifier.endpoint_name
    OR $endpointIdentifier.endpoint_name = ''
)
`, Endpoint{}, epIdentifier)
	if err != nil {
		return nil, errors.Capture(err)
	}
	var endpoints []Endpoint
	err = tx.Query(ctx, stmt, epIdentifier).GetAll(&endpoints)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("getting candidate endpoints for %q: %w", identifier, err)
	}

	return endpoints, nil
}

// getApplicationEndpointUUID returns the application endpoint uuid for given
// application and endpoint name pair.
func (st *State) getApplicationEndpointUUID(ctx context.Context, tx *sqlair.TX,
	applicationName, endpointName string) (corerelation.EndpointUUID, error) {
	type applicationEndpointUUID relationEndpointUUID

	epIdentifier := endpointIdentifier{
		ApplicationName: applicationName,
		EndpointName:    endpointName,
	}

	stmt, err := st.Prepare(`
SELECT aeu.uuid AS &applicationEndpointUUID.uuid
FROM   v_application_endpoint_uuid AS aeu
JOIN   application a ON a.uuid = aeu.application_uuid
WHERE  aeu.name = $endpointIdentifier.endpoint_name
AND    a.name = $endpointIdentifier.application_name
`, applicationEndpointUUID{}, endpointIdentifier{})
	if err != nil {
		return "", errors.Capture(err)
	}
	var endpoint applicationEndpointUUID
	err = tx.Query(ctx, stmt, epIdentifier).Get(&endpoint)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("getting application endpoint uuid for %q:%q : %w", applicationName, endpointName, err)
	}

	return corerelation.EndpointUUID(endpoint.UUID), nil
}

// getBases retrieves a list of OS and channel information for a specific
// application, given an endpoint and transaction.
func (st *State) getBases(ctx context.Context, tx *sqlair.TX, ep1 Endpoint) ([]corebase.Base, error) {
	stmt, err := st.Prepare(`
SELECT 
    ap.channel AS &applicationPlatform.channel,
    os.name AS &applicationPlatform.os
FROM application_platform ap
JOIN os ON ap.os_id = os.id
WHERE ap.application_uuid = $Endpoint.application_uuid`, ep1, applicationPlatform{})
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

func (st *State) getRelationDetails(ctx context.Context, tx *sqlair.TX, relationUUID string) (domainrelation.RelationDetailsResult, error) {
	type getRelation struct {
		UUID      string     `db:"uuid"`
		ID        int        `db:"relation_id"`
		Life      life.Value `db:"value"`
		Suspended bool       `db:"suspended"`
	}

	rel := getRelation{
		UUID: relationUUID,
	}
	stmt, err := st.Prepare(`
SELECT (r.uuid, r.relation_id, l.value, r.suspended) AS (&getRelation.*)
FROM   relation r
JOIN   life l ON r.life_id = l.id
WHERE  r.uuid = $getRelation.uuid
`, rel)
	if err != nil {
		return domainrelation.RelationDetailsResult{}, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, rel).Get(&rel)
	if errors.Is(err, sqlair.ErrNoRows) {
		return domainrelation.RelationDetailsResult{}, relationerrors.RelationNotFound
	} else if err != nil {
		return domainrelation.RelationDetailsResult{}, errors.Capture(err)
	}

	var endpoints []domainrelation.Endpoint
	endpoints, err = st.getRelationEndpoints(ctx, tx, rel.UUID)
	if err != nil {
		return domainrelation.RelationDetailsResult{}, errors.Errorf("getting relation endpoints: %w", err)
	}

	inScopeCount, err := st.countInScopeRelations(ctx, tx, rel.UUID)
	if err != nil {
		return domainrelation.RelationDetailsResult{}, errors.Errorf("counting in-scope relations: %w", err)
	}

	f := domainrelation.RelationDetailsResult{
		Life:         rel.Life,
		UUID:         corerelation.UUID(rel.UUID),
		ID:           rel.ID,
		Suspended:    rel.Suspended,
		Endpoints:    endpoints,
		InScopeUnits: inScopeCount,
	}
	return f, nil
}

// countInScopeRelations counts the number of units in the relation that are in
// scope (i.e. the number of relation_unit documents associated with a
// relation).
func (st *State) countInScopeRelations(ctx context.Context, tx *sqlair.TX, relationUUID string) (int, error) {
	type getCount struct {
		UUID  string `db:"uuid"`
		Count int    `db:"count"`
	}

	count := getCount{UUID: relationUUID}
	stmt, err := st.Prepare(`
SELECT count(*) AS &getCount.count
FROM   relation_unit ru
JOIN   relation_endpoint re ON ru.relation_endpoint_uuid = re.uuid
WHERE  re.relation_uuid = $getCount.uuid
`, count)
	if err != nil {
		return 0, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, count).Get(&count)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return 0, errors.Capture(err)
	}

	return count.Count, nil
}

// inferEndpoints determines and validates the endpoints of a given relation
// based on the provided identifiers. It tries to find matching endpoint for both
// application, regarding the name of endpoint if provided, their interfaces,
// their roles and their scopes.
// If there is several matches or none, it will return an error.
func (st *State) inferEndpoints(
	ctx context.Context,
	tx *sqlair.TX,
	identifier1, identifier2 domainrelation.CandidateEndpointIdentifier) (Endpoint, Endpoint, error) {

	// Get candidate endpoints.
	endpoints1, err := st.getCandidateEndpoints(ctx, tx, identifier1)
	if err != nil {
		return Endpoint{}, Endpoint{}, errors.Capture(err)
	}
	endpoints2, err := st.getCandidateEndpoints(ctx, tx, identifier2)
	if err != nil {
		return Endpoint{}, Endpoint{}, errors.Capture(err)
	}

	var noCandidates []string
	if len(endpoints1) == 0 {
		noCandidates = append(noCandidates, identifier1.String())
	}
	if len(endpoints2) == 0 {
		noCandidates = append(noCandidates, identifier2.String())
	}
	if len(noCandidates) > 0 {
		return Endpoint{}, Endpoint{}, errors.Errorf("no candidates for %s: %w",
			strings.Join(noCandidates, " and "),
			relationerrors.RelationEndpointNotFound)
	}

	// It is ok since we checked that both sides have candidates,
	// and all candidates from one side should have the same application
	app1UUID := endpoints1[0].ApplicationUUID.String()
	app2UUID := endpoints2[0].ApplicationUUID.String()

	// Check if applications are subordinates.
	isSubordinate1, err := st.isSubordinate(ctx, tx, app1UUID)
	if err != nil {
		return Endpoint{}, Endpoint{}, errors.Capture(err)
	}
	isSubordinate2, err := st.isSubordinate(ctx, tx, app2UUID)
	if err != nil {
		return Endpoint{}, Endpoint{}, errors.Capture(err)
	}

	// Compute matches.
	type match struct {
		ep1 *Endpoint
		ep2 *Endpoint
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
		return Endpoint{}, Endpoint{}, relationerrors.CompatibleEndpointsNotFound
	} else if matchCount > 1 {
		possibleMatches := make([]string, 0, matchCount)
		for _, match := range matches {
			possibleMatches = append(possibleMatches, fmt.Sprintf("\"%s %s\"", match.ep1, match.ep2))
		}
		return Endpoint{}, Endpoint{}, errors.Errorf("%w: %q could refer to %s",
			relationerrors.AmbiguousRelation, fmt.Sprintf("%s %s", identifier1, identifier2),
			strings.Join(possibleMatches, "; "))
	}

	return *matches[0].ep1, *matches[0].ep2, nil
}

// insertNewRelation creates a new relation entry in the database and returns its UUID or an error if the operation fails.
func (st *State) insertNewRelation(
	ctx context.Context, tx *sqlair.TX, id uint64, scope charm.RelationScope,
) (corerelation.UUID, error) {
	relUUID, err := corerelation.NewUUID()
	if err != nil {
		return relUUID, errors.Errorf("generating new relation UUID: %w", err)
	}

	scopeID, err := encodeScope(scope)
	if err != nil {
		return relUUID, errors.Errorf("encoding scope: %w", err)
	}

	rel := relation{
		UUID:    relUUID,
		ID:      id,
		LifeID:  domainlife.Alive,
		ScopeID: scopeID,
	}

	stmtInsert, err := st.Prepare(`
INSERT INTO relation (*)
VALUES ($relation.*)
`, rel)
	if err != nil {
		return relUUID, errors.Capture(err)
	}

	if err := tx.Query(ctx, stmtInsert, rel).Run(); err != nil {
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
SELECT $setRelationStatus.relation_uuid, rst.id, $setRelationStatus.updated_at
FROM   relation_status_type AS rst
WHERE  rst.name = $setRelationStatus.status`, status)
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
func (st *State) relationAlreadyExists(ctx context.Context, tx *sqlair.TX, ep1 Endpoint, ep2 Endpoint) error {
	type (
		endpoint1 Endpoint
		endpoint2 Endpoint
	)
	e1 := endpoint1(ep1)
	e2 := endpoint2(ep2)

	stmt, err := st.Prepare(`
SELECT r.uuid as &relationUUID.uuid
FROM   relation r
JOIN   relation_endpoint e1 ON r.uuid = e1.relation_uuid
JOIN   relation_endpoint e2 ON r.uuid = e2.relation_uuid
WHERE  e1.endpoint_uuid = $endpoint1.application_endpoint_uuid                          
AND    e2.endpoint_uuid = $endpoint2.application_endpoint_uuid
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
	ctx context.Context, tx *sqlair.TX, unitUUID string,
) (string, error) {
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
		return "", applicationerrors.UnitNotFound
	} else if err != nil {
		return "", errors.Capture(err)
	}

	return getApplication.ApplicationUUID, nil
}

func (st *State) getRelationAndApplicationOfRelationUnit(
	ctx context.Context,
	tx *sqlair.TX,
	relationUnitUUID string,
) (relUUID string, appUUID string, err error) {
	args := getUnitRelAndApp{
		RelationUnitUUID: relationUnitUUID,
	}
	stmt, err := st.Prepare(`
SELECT (re.relation_uuid, ae.application_uuid) AS (&getUnitRelAndApp.*)
FROM   relation_unit ru
JOIN   relation_endpoint re ON re.uuid = ru.relation_endpoint_uuid
JOIN   application_endpoint ae ON ae.uuid = re.endpoint_uuid
WHERE  ru.uuid = $getUnitRelAndApp.uuid
`, args)
	if err != nil {
		return "", "", errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, args).Get(&args)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", "", applicationerrors.UnitNotFound
	} else if err != nil {
		return "", "", errors.Capture(err)
	}

	return args.RelationUUID, args.ApplicationUUID, nil
}

func convertSettings(input []relationSetting) map[string]string {
	output := make(map[string]string, len(input))
	for _, in := range input {
		output[in.Key] = in.Value
	}
	return output
}

func (st *State) getEveryRelationForApplicationID(
	ctx context.Context,
	tx *sqlair.TX,
	appID application.UUID,
) ([]relationIDUUIDAppName, error) {
	var result []relationIDUUIDAppName

	stmt, err := st.Prepare(`
SELECT r.uuid AS &relationIDUUIDAppName.uuid,
       r.relation_id AS &relationIDUUIDAppName.relation_id,
       a.name AS &relationIDUUIDAppName.application_name
FROM   relation AS r
JOIN   relation_endpoint AS re ON r.uuid = re.relation_uuid
JOIN   application_endpoint AS ae ON re.endpoint_uuid = ae.uuid
JOIN   application AS a ON ae.application_uuid = a.uuid
WHERE  a.uuid = $entityUUID.uuid
`, relationIDUUIDAppName{}, entityUUID{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	app := entityUUID{UUID: appID.String()}
	err = tx.Query(ctx, stmt, app).GetAll(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return []relationIDUUIDAppName{}, relationerrors.RelationNotFound
	}
	return result, errors.Capture(err)
}

func (st *State) getRelationEndpointIdentifiers(
	ctx context.Context,
	tx *sqlair.TX,
	relUUID string,
) ([]endpointIdentifier, error) {
	var result []endpointIdentifier

	stmt, err := st.Prepare(`
SELECT &endpointIdentifier.*
FROM   relation r
JOIN   v_relation_endpoint e ON r.uuid = e.relation_uuid
WHERE  r.uuid = $relationUUID.uuid
	`, endpointIdentifier{}, relationUUID{})
	if err != nil {
		return result, errors.Capture(err)
	}

	app := relationUUID{UUID: relUUID}
	err = tx.Query(ctx, stmt, app).GetAll(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return result, relationerrors.RelationNotFound
	}

	return result, nil
}

func (st *State) getUnitsRelationData(
	ctx context.Context,
	tx *sqlair.TX,
	relUUID string,
	appID string,
) (map[string]domainrelation.RelationData, error) {
	var result map[string]domainrelation.RelationData

	relUnits, err := st.getRelationUnitsWithUnits(ctx, tx, relUUID, appID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// For each relation unit, get settings and fill in RelationData.
	result = make(map[string]domainrelation.RelationData, len(relUnits))
	for _, relUnit := range relUnits {
		// Units without a relation unit are out of scope. Relation unit
		// settings only exist for relations in scope.
		if relUnit.RelationUnitUUID == "" {
			result[relUnit.UnitName.String()] = domainrelation.RelationData{InScope: false}
			continue
		}

		settings, err := st.getRelationUnitSettings(ctx, tx, relUnit.RelationUnitUUID)
		if err != nil {
			return nil, errors.Capture(err)
		}
		result[relUnit.UnitName.String()] = domainrelation.RelationData{
			InScope:  true,
			UnitData: convertSettings(settings),
		}
	}
	return result, nil
}

// getRelationUnitsWithUnits returns all relationUnitWithUnit data for all units
// of the given application. The data includes relation unit details if the
// unit is in scope.
func (st *State) getRelationUnitsWithUnits(
	ctx context.Context,
	tx *sqlair.TX,
	relationUUID string,
	appID string,
) ([]relationUnitWithUnit, error) {
	relationUnitStmt, err := st.Prepare(`
SELECT    ru.uuid AS &relationUnitWithUnit.uuid,
          u.name AS &relationUnitWithUnit.unit_name,
          u.uuid AS &relationUnitWithUnit.unit_uuid
FROM      unit AS u
JOIN      application_endpoint AS ae ON u.application_uuid = ae.application_uuid
JOIN      relation_endpoint AS re ON ae.uuid = re.endpoint_uuid
LEFT JOIN relation_unit AS ru ON re.uuid = ru.relation_endpoint_uuid AND u.uuid = ru.unit_uuid
WHERE     u.application_uuid = $relationAndApplicationUUID.application_uuid
AND       re.relation_uuid = $relationAndApplicationUUID.relation_uuid
`, relationUnitWithUnit{}, relationAndApplicationUUID{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var relationUnits []relationUnitWithUnit
	relAndApp := relationAndApplicationUUID{
		RelationUUID:  relationUUID,
		ApplicationID: appID,
	}
	err = tx.Query(ctx, relationUnitStmt, relAndApp).GetAll(&relationUnits)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Capture(err)
	}
	return relationUnits, nil
}

func (st *State) setRelationApplicationSettings(
	ctx context.Context,
	tx *sqlair.TX,
	relationUUID string,
	applicationID string,
	settings map[string]string,
) error {
	// If the settings are nil then there is nothing to do. Do not check for
	// length of 0, as that is valid for deleting all settings.
	if settings == nil {
		return nil
	}

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
}

func (st *State) relationContainsRemoteApplication(ctx context.Context, tx *sqlair.TX, relationUUID string) (bool, error) {
	ident := entityUUID{UUID: relationUUID}

	stmt, err := st.Prepare(`
SELECT COUNT(cs.name) AS &rows.count
FROM   charm_source AS cs
JOIN   charm AS c ON cs.id = c.source_id
JOIN   application AS a ON c.uuid = a.charm_uuid
JOIN   application_endpoint AS ae ON a.uuid = ae.application_uuid
JOIN   relation_endpoint AS re ON ae.uuid = re.endpoint_uuid
WHERE  re.relation_uuid = $entityUUID.uuid
AND    cs.name = 'cmr'
	`, rows{}, ident)
	if err != nil {
		return false, errors.Errorf("preparing statement: %w", err)
	}

	var result rows
	err = tx.Query(ctx, stmt, ident).Get(&result)
	if err != nil {
		return false, errors.Errorf("checking remote applications: %w", err)
	}

	return result.Count > 0, nil
}

func (st *State) insertRelationNetworkEgress(ctx context.Context, tx *sqlair.TX, relUUID string, cidrs ...string) error {
	insertStmt, err := st.Prepare(`
INSERT INTO relation_network_egress (relation_uuid, cidr)
VALUES ($relationNetworkEgress.*)
ON CONFLICT (relation_uuid, cidr) DO NOTHING
`, relationNetworkEgress{})
	if err != nil {
		return errors.Errorf("preparing insert statement: %w", err)
	}

	var egress []relationNetworkEgress
	for _, cidr := range cidrs {
		egress = append(egress, relationNetworkEgress{
			RelationUUID: relUUID,
			CIDR:         cidr,
		})
	}

	if err := tx.Query(ctx, insertStmt, egress).Run(); err != nil {
		return errors.Errorf("inserting CIDRs %v for relation %q: %w", cidrs, relUUID, err)
	}
	return nil
}

func (st *State) checkBothEndpointsNotRemote(ctx context.Context, tx *sqlair.TX, ep1, ep2 Endpoint) error {
	type endpointUUIDs []string
	uuids := endpointUUIDs{ep1.ApplicationEndpointUUID.String(), ep2.ApplicationEndpointUUID.String()}

	stmt, err := st.Prepare(`
SELECT COUNT(*) AS &rows.count
FROM   application_endpoint AS ae
JOIN   application AS a ON ae.application_uuid = a.uuid
JOIN   charm AS c ON a.charm_uuid = c.uuid
JOIN   charm_source AS cs ON c.source_id = cs.id
WHERE  ae.uuid IN ($endpointUUIDs[:])
AND    cs.name = 'cmr'
	`, rows{}, uuids)
	if err != nil {
		return errors.Capture(err)
	}

	var count rows
	err = tx.Query(ctx, stmt, uuids).Get(&count)
	if err != nil {
		return errors.Capture(err)
	}

	// If both endpoints are CMR (count == 2), return an error.
	if count.Count == 2 {
		return errors.Errorf("unable to integrate two cross model applications together").Add(relationerrors.BothEndpointsRemote)
	}

	return nil
}

// encodeScope encodes the scope of a relation.
func encodeScope(scope charm.RelationScope) (uint8, error) {
	switch scope {
	case charm.ScopeGlobal:
		return 0, nil
	case charm.ScopeContainer:
		return 1, nil
	default:
		return 0, errors.Errorf("invalid scope %q", scope)
	}
}
