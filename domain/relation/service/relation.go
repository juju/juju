// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/logger"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

// State describes retrieval and persistence methods for relations.
type State interface {
	// ApplicationRelationEndpointNames returns a slice of names of the given application's
	// relation endpoints.
	ApplicationRelationsInfo(
		ctx context.Context,
		applicationID application.ID,
	) ([]relation.EndpointRelationData, error)

	// AddRelation establishes a relation between two endpoints identified
	// by ep1 and ep2 and returns the created endpoints.
	AddRelation(ctx context.Context, ep1, ep2 relation.CandidateEndpointIdentifier) (relation.Endpoint, relation.Endpoint, error)

	// SetRelationWithID establishes a relation between two endpoints identified
	// by ep1 and ep2 and returns the relation UUID. Used for migration
	// import.
	SetRelationWithID(
		ctx context.Context,
		ep1, ep2 corerelation.EndpointIdentifier,
		id uint64,
	) (corerelation.UUID, error)

	// NeedsSubordinateUnit checks if there is a subordinate application
	// related to the principal unit that needs a subordinate unit created.
	NeedsSubordinateUnit(
		ctx context.Context,
		relationUUID corerelation.UUID,
		principalUnitName unit.Name,
	) (*application.ID, error)

	// DeleteImportedRelations deletes all imported relations in a model during
	// an import rollback.
	DeleteImportedRelations(
		ctx context.Context,
	) error

	// EnterScope indicates that the provided unit has joined the relation.
	// When the unit has already entered its relation scope, EnterScope will report
	// success but make no changes to state. The unit's settings are created or
	// overwritten in the relation according to the supplied map.
	//
	// The following error types can be expected to be returned:
	//   - [relationerrors.RelationNotFound] if the relation cannot be found.
	//   - [relationerrors.UnitNotFound] if no unit by the given name can be found
	//   - [relationerrors.RelationNotAlive] if the relation is not alive.
	//   - [relationerrors.UnitNotAlive] if the unit is not alive.
	//   - [relationerrors.PotentialRelationUnitNotValid] if the unit entering
	//     scope is a subordinate and the endpoint scope is charm.ScopeContainer
	//     where the other application is a principal, but not in the current
	//     relation.
	EnterScope(
		ctx context.Context,
		relationUUID corerelation.UUID,
		unitName unit.Name,
		settings map[string]string,
	) error

	// ExportRelations returns all relation information to be exported for the
	// model.
	ExportRelations(ctx context.Context) ([]relation.ExportRelation, error)

	// GetAllRelationDetails return RelationDetailResults for all relations
	// for the current model.
	GetAllRelationDetails(ctx context.Context) ([]relation.RelationDetailsResult, error)

	// GetGoalStateRelationDataForApplication returns GoalStateRelationData for all
	// relations the given application is in, modulo peer relations.
	//
	// The following error types can be expected to be returned:
	//   - [relationerrors.ApplicationNotFound] is returned if the application
	//     is not found.
	GetGoalStateRelationDataForApplication(
		ctx context.Context,
		applicationID application.ID,
	) ([]relation.GoalStateRelationData, error)

	// GetApplicationEndpoints returns all endpoints for the given application
	// identifier.
	GetApplicationEndpoints(ctx context.Context, applicationID application.ID) ([]relation.Endpoint, error)

	// GetApplicationIDByName returns the application ID of the given application.
	GetApplicationIDByName(ctx context.Context, appName string) (application.ID, error)

	// GetApplicationRelations retrieves all relation UUIDs associated with a
	// specific application identified by its ID.
	GetApplicationRelations(ctx context.Context, id application.ID) ([]corerelation.UUID, error)

	// GetMapperDataForWatchLifeSuspendedStatus returns data needed to evaluate a relation
	// uuid as part of WatchLifeSuspendedStatus eventmapper.
	//
	// The following error types can be expected to be returned:
	//   - [relationerrors.ApplicationNotFoundForRelation] is returned if the
	//     application is not part of the relation.
	//   - [relationerrors.RelationNotFound] is returned if the relation UUID
	//     is not found.
	GetMapperDataForWatchLifeSuspendedStatus(
		ctx context.Context,
		relUUID corerelation.UUID,
		appID application.ID,
	) (relation.RelationLifeSuspendedData, error)

	// GetOtherRelatedEndpointApplicationData returns an OtherApplicationForWatcher struct
	// for each Endpoint in a relation with the given application ID.
	GetOtherRelatedEndpointApplicationData(
		ctx context.Context,
		relUUID corerelation.UUID,
		applicationID application.ID,
	) (relation.OtherApplicationForWatcher, error)

	// GetPeerRelationUUIDByEndpointIdentifiers gets the UUID of a peer
	// relation specified by a single endpoint identifier.
	//
	// The following error types can be expected to be returned:
	//   - [relationerrors.RelationNotFound] is returned if endpoint cannot be
	//     found.
	GetPeerRelationUUIDByEndpointIdentifiers(
		ctx context.Context,
		endpoint corerelation.EndpointIdentifier,
	) (corerelation.UUID, error)

	// GetPrincipalSubordinateApplicationIDs returns the Principal and
	// Subordinate application IDs for the given unit. The principal will
	// be the first ID returned and the subordinate will be the second. If
	// the unit is not a subordinate, the second application ID will be
	// empty.
	GetPrincipalSubordinateApplicationIDs(
		ctx context.Context,
		unitUUID unit.UUID,
	) (application.ID, application.ID, error)

	// GetRelationApplicationSettings returns the application settings
	// for the given application and relation identifier combination.
	//
	// The following error types can be expected to be returned:
	//   - [relationerrors.ApplicationNotFoundForRelation] is returned if the
	//     application is not part of the relation.
	//   - [relationerrors.RelationNotFound] is returned if the relation UUID
	//     is not found.
	GetRelationApplicationSettings(
		ctx context.Context,
		relationUUID corerelation.UUID,
		applicationID application.ID,
	) (map[string]string, error)

	// GetRelationID returns the relation ID for the given relation UUID.
	//
	// The following error types can be expected to be returned:
	//   - [relationerrors.RelationNotFound] is returned if the relation UUID
	//     is not found.
	GetRelationID(ctx context.Context, relationUUID corerelation.UUID) (int, error)

	// GetRelationUUIDByID returns the relation UUID based on the relation ID.
	//
	// The following error types can be expected to be returned:
	//   - [relationerrors.RelationNotFound] is returned if the relation UUID
	//     relating to the relation ID cannot be found.
	GetRelationUUIDByID(ctx context.Context, relationID int) (corerelation.UUID, error)

	// GetRelationEndpoints returns all relation endpoints for the given
	// relation UUID.
	//
	// The following error types can be expected:
	//   - [relationerrors.RelationNotFound]: when no relation exists for the
	//     given UUID.
	GetRelationEndpoints(ctx context.Context, relationUUID corerelation.UUID) ([]relation.Endpoint, error)

	// GetRelationEndpointScope returns the scope of the relation endpoint
	// at the intersection of the relationUUID and applicationID.
	//
	// The following error types can be expected to be returned:
	//   - [relationerrors.RelationNotFound] is returned if the relation UUID
	//     relating to the relation ID cannot be found.
	GetRelationEndpointScope(
		ctx context.Context,
		relationUUID corerelation.UUID,
		applicationID application.ID,
	) (charm.RelationScope, error)

	// GetRelationEndpointUUID retrieves the unique identifier for a specific
	// relation endpoint based on the provided arguments.
	GetRelationEndpointUUID(ctx context.Context, args relation.GetRelationEndpointUUIDArgs) (corerelation.EndpointUUID, error)

	// GetRegularRelationUUIDByEndpointIdentifiers gets the UUID of a regular
	// relation specified by two endpoint identifiers.
	//
	// The following error types can be expected to be returned:
	//   - [relationerrors.RelationNotFound] is returned if endpoints cannot be
	//     found.
	GetRegularRelationUUIDByEndpointIdentifiers(
		ctx context.Context,
		endpoint1, endpoint2 corerelation.EndpointIdentifier,
	) (corerelation.UUID, error)

	// GetRelationsStatusForUnit returns RelationUnitStatus for all relations
	// the unit is part of.
	GetRelationsStatusForUnit(ctx context.Context, unitUUID unit.UUID) ([]relation.RelationUnitStatusResult, error)

	// GetRelationDetails returns relation details for the given relationUUID.
	//
	// The following error types can be expected to be returned:
	//   - [relationerrors.RelationNotFound] is returned if the relation UUID
	//     is not found.
	GetRelationDetails(ctx context.Context, relationUUID corerelation.UUID) (relation.RelationDetailsResult, error)

	// GetRelationUnitChanges retrieves changes to relation unit states and
	// application settings for the provided UUIDs.
	// It takes a list of unit UUIDs and application UUIDs, returning the
	// current setting version for each one, or departed if any unit is not found
	GetRelationUnitChanges(ctx context.Context, unitUUIDs []unit.UUID, appUUIDs []application.ID) (relation.RelationUnitsChange, error)

	// GetRelationUnitEndpointName returns the name of the endpoint for the given
	// relation unit.
	//
	// The following error types can be expected to be returned:
	//   - [relationerrors.RelationUnitNotFound] if the relation unit cannot be found.
	GetRelationUnitEndpointName(ctx context.Context, relationUnitUUID corerelation.UnitUUID) (string, error)

	// GetRelationUnit retrieves the UUID of a relation unit based on the given
	// relation UUID and unit name.
	GetRelationUnit(
		ctx context.Context,
		relationUUID corerelation.UUID,
		unitName unit.Name,
	) (corerelation.UnitUUID, error)

	// GetRelationUnitSettings returns the relation unit settings for the given
	// relation unit.
	//
	// The following error types can be expected to be returned:
	//   - [relationerrors.RelationUnitNotFound] is returned if the
	//     unit is not part of the relation.
	GetRelationUnitSettings(ctx context.Context, relationUnitUUID corerelation.UnitUUID) (map[string]string, error)

	// InitialWatchLifeSuspendedStatus returns the two tables to watch for
	// a relation's Life and Suspended status when the relation contains
	// the provided application and the initial namespace query.
	InitialWatchLifeSuspendedStatus(id application.ID) (string, string, eventsource.NamespaceQuery)

	// LeaveScope updates the given relation to indicate it is not in scope.
	//
	// The following error types can be expected to be returned:
	//   - [relationerrors.RelationUnitNotFound] if the relation unit cannot be
	//     found.
	LeaveScope(ctx context.Context, relationUnitUUID corerelation.UnitUUID) error

	// SetRelationApplicationSettings records settings for a specific application
	// relation combination.
	//
	// The following error types can be expected to be returned:
	//   - [relationerrors.ApplicationNotFoundForRelation] is returned if the
	//     application is not part of the relation.
	//   - [relationerrors.RelationNotFound] is returned if the relation UUID
	//     is not found.
	SetRelationApplicationSettings(
		ctx context.Context,
		relationUUID corerelation.UUID,
		applicationID application.ID,
		settings map[string]string,
	) error

	// SetRelationApplicationAndUnitSettings records settings for a unit and
	// an application in a relation.
	//
	// The following error types can be expected to be returned:
	//   - [relationerrors.RelationUnitNotFound] is returned if the
	//     relation unit is not found.
	SetRelationApplicationAndUnitSettings(
		ctx context.Context,
		relationUnitUUID corerelation.UnitUUID,
		applicationSettings, unitSettings map[string]string,
	) error

	// SetRelationUnitSettings records settings for a specific relation unit.
	//
	// The following error types can be expected to be returned:
	//   - [relationerrors.RelationUnitNotFound] is returned if the unit is not
	//     part of the relation.
	SetRelationUnitSettings(
		ctx context.Context,
		relationUnitUUID corerelation.UnitUUID,
		settings map[string]string,
	) error

	// WatcherApplicationSettingsNamespace provides the table name to set up
	// watchers for relation application settings.
	WatcherApplicationSettingsNamespace() string

	// InitialWatchRelatedUnits initializes a watch for changes related to the
	// specified unit in the given relation.
	InitialWatchRelatedUnits(name unit.Name, uuid corerelation.UUID) ([]string, eventsource.NamespaceQuery, eventsource.Mapper)
}

// LeadershipService provides the API for working with the statuses of applications
// and units, including the API handlers that require leadership checks.
type LeadershipService struct {
	*Service
	leaderEnsurer leadership.Ensurer
}

func NewLeadershipService(
	st State,
	leaderEnsurer leadership.Ensurer,
	logger logger.Logger,
) *LeadershipService {
	return &LeadershipService{
		Service:       NewService(st, logger),
		leaderEnsurer: leaderEnsurer,
	}
}

// GetLocalRelationApplicationSettings returns the application settings
// for the given application and relation identifier combination.
// ApplicationSettings may only be read by the application leader.
//
// The following error types can be expected to be returned:
//   - [corelease.ErrNotHeld] if the unit is not the leader.
//   - [relationerrors.ApplicationNotFoundForRelation] is returned if the
//     application is not part of the relation.
//   - [relationerrors.RelationNotFound] is returned if the relation UUID
//     is not found.
func (s *LeadershipService) GetLocalRelationApplicationSettings(
	ctx context.Context,
	unitName unit.Name,
	relationUUID corerelation.UUID,
	applicationID application.ID,
) (map[string]string, error) {
	if err := unitName.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	if err := relationUUID.Validate(); err != nil {
		return nil, errors.Errorf(
			"%w:%w", relationerrors.RelationUUIDNotValid, err)
	}
	if err := applicationID.Validate(); err != nil {
		return nil, errors.Errorf(
			"%w:%w", relationerrors.ApplicationIDNotValid, err)
	}
	settings := make(map[string]string)
	err := s.leaderEnsurer.WithLeader(ctx, unitName.Application(), unitName.String(),
		func(ctx context.Context) error {
			var err error
			settings, err = s.st.GetRelationApplicationSettings(ctx, relationUUID, applicationID)
			if err != nil {
				return errors.Capture(err)
			}
			return nil
		},
	)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return settings, nil
}

// SetRelationApplicationSettings records settings for a specific application
// relation combination.
//
// When settings is not empty, the following error types can be expected to be
// returned:
//   - [corelease.ErrNotHeld] if the unit is not the leader.
//   - [relationerrors.ApplicationNotFoundForRelation] is returned if the
//     application is not part of the relation.
//   - [relationerrors.RelationNotFound] is returned if the relation UUID
//     is not found.
func (s *LeadershipService) SetRelationApplicationSettings(
	ctx context.Context,
	unitName unit.Name,
	relationUUID corerelation.UUID,
	applicationID application.ID,
	settings map[string]string,
) error {
	if len(settings) == 0 {
		return nil
	}

	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}
	if err := relationUUID.Validate(); err != nil {
		return errors.Errorf(
			"%w:%w", relationerrors.RelationUUIDNotValid, err)
	}
	if err := applicationID.Validate(); err != nil {
		return errors.Errorf(
			"%w:%w", relationerrors.ApplicationIDNotValid, err)
	}
	return s.leaderEnsurer.WithLeader(ctx, unitName.Application(), unitName.String(), func(ctx context.Context) error {
		return s.st.SetRelationApplicationSettings(ctx, relationUUID, applicationID, settings)
	})
}

// SetRelationApplicationAndUnitSettings records settings for a unit and
// an application in a relation.
//
// The following error types can be expected to be returned:
//   - [corelease.ErrNotHeld] if the unit is not the leader and
//     applicationSettings has a none zero length.
//   - [relationerrors.RelationUnitNotFound] is returned if the
//     relation unit is not found.
func (s *LeadershipService) SetRelationApplicationAndUnitSettings(
	ctx context.Context,
	unitName unit.Name,
	relationUnitUUID corerelation.UnitUUID,
	applicationSettings, unitSettings map[string]string,
) error {
	if len(applicationSettings) == 0 && len(unitSettings) == 0 {
		return nil
	}

	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}
	if err := relationUnitUUID.Validate(); err != nil {
		return errors.Errorf(
			"%w:%w", relationerrors.RelationUUIDNotValid, err)
	}

	// If not setting the application settings, do not check leadership.
	if len(applicationSettings) == 0 {
		return s.st.SetRelationUnitSettings(ctx, relationUnitUUID, unitSettings)
	}

	return s.leaderEnsurer.WithLeader(ctx, unitName.Application(), unitName.String(), func(ctx context.Context) error {
		return s.st.SetRelationApplicationAndUnitSettings(ctx, relationUnitUUID, applicationSettings, unitSettings)
	})

}

// Service provides the API for working with relations.
type Service struct {
	st     State
	logger logger.Logger
}

// NewService returns a new service reference wrapping the input state.
func NewService(
	st State,
	logger logger.Logger,
) *Service {
	return &Service{
		st:     st,
		logger: logger,
	}
}

// AddRelation takes two endpoints identifiers of the form
// <application>[:<endpoint>]. The identifiers will be used to infer two
// endpoint between applications on the model. A new relation will be created
// between these endpoints and the details of the endpoint returned.
func (s *Service) AddRelation(ctx context.Context, ep1, ep2 string) (relation.Endpoint,
	relation.Endpoint, error) {
	var none relation.Endpoint
	idep1, err := relation.NewCandidateEndpointIdentifier(ep1)
	if err != nil {
		return none, none, errors.Errorf("parsing endpoint identifier %q: %w", ep1, err)
	}
	idep2, err := relation.NewCandidateEndpointIdentifier(ep2)
	if err != nil {
		return none, none, errors.Errorf("parsing endpoint identifier %q: %w", ep2, err)
	}

	return s.st.AddRelation(ctx, idep1, idep2)
}

// ApplicationRelationEndpointNames returns a slice of names of the given application's
// relation endpoints.
// Note: Replaces the functionality in CharmRelations method of the application facade.
func (s *Service) ApplicationRelationEndpointNames(ctx context.Context, id application.ID) ([]string, error) {
	return nil, coreerrors.NotImplemented
}

// ApplicationRelations returns relation UUIDs for the given
// application ID.
func (s *Service) ApplicationRelations(ctx context.Context, id application.ID) (
	[]corerelation.UUID, error) {
	return []corerelation.UUID{}, coreerrors.NotImplemented
}

// ApplicationRelationsInfo returns all EndpointRelationData for an application.
//
// The following error types can be expected to be returned:
//   - [relationerrors.ApplicationIDNotValid] if the application id is not valid.
//   - [relationerrors.ApplicationNotFound] is returned if the application is
//     not found.
func (s *Service) ApplicationRelationsInfo(
	ctx context.Context,
	applicationID application.ID,
) ([]relation.EndpointRelationData, error) {
	if err := applicationID.Validate(); err != nil {
		return nil, relationerrors.ApplicationIDNotValid
	}
	return s.st.ApplicationRelationsInfo(ctx, applicationID)
}

// EnterScope indicates that the provided unit has joined the relation.
// When the unit has already entered its relation scope, EnterScope will report
// success but make no changes to state. The unit's settings are created or
// overwritten in the relation according to the supplied map.
//
// If there is a subordinate application related to the unit entering scope that
// needs a subordinate unit creating, then the subordinate unit will be created
// with the provided createSubordinate function.
//
// The following error types can be expected to be returned:
//   - [relationerrors.PotentialRelationUnitNotValid] if the unit entering
//     scope is a subordinate and the endpoint scope is charm.ScopeContainer
//     where the other application is a principal, but not in the current
//     relation.
func (s *Service) EnterScope(
	ctx context.Context,
	relationUUID corerelation.UUID,
	unitName unit.Name,
	settings map[string]string,
	subordinateCreator relation.SubordinateCreator,
) error {
	if err := relationUUID.Validate(); err != nil {
		return errors.Errorf(
			"%w:%w", relationerrors.RelationUUIDNotValid, err)
	}
	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}

	// Enter the unit into the relation scope.
	err := s.st.EnterScope(ctx, relationUUID, unitName, settings)
	if err != nil {
		return errors.Capture(err)
	}

	// Check if a subordinate unit needs creating.
	subID, err := s.st.NeedsSubordinateUnit(ctx, relationUUID, unitName)
	if err != nil {
		return errors.Capture(err)
	} else if subID != nil {
		// Create the required unit on the related subordinate application.
		//
		// TODO(aflynn): In 3.6 the subordinate was created in the same
		// transaction as the principal entering scope. This is not the case
		// here. If the subordinate creation fails, there should be some retry
		// mechanism, or a rollback of enter scope.
		if subordinateCreator == nil {
			return errors.Errorf("subordinate creator is nil")
		}
		err := subordinateCreator.CreateSubordinate(ctx, *subID, unitName)
		if err != nil {
			return errors.Errorf("creating subordinate unit on application %q: %w", *subID, err)
		}
	}

	return nil
}

// GetAllRelationDetails return RelationDetailResults of all relation for the current model.
func (s *Service) GetAllRelationDetails(ctx context.Context) ([]relation.RelationDetailsResult, error) {
	return s.st.GetAllRelationDetails(ctx)
}

// GetGoalStateRelationDataForApplication returns GoalStateRelationData for all
// relations the given application is in, modulo peer relations. No error is
// if the application is not in any relations.
//
// The following error types can be expected to be returned:
//   - [relationerrors.ApplicationIDNotValid] is returned if the application
//     UUID is not valid.
func (s *Service) GetGoalStateRelationDataForApplication(
	ctx context.Context,
	applicationID application.ID,
) ([]relation.GoalStateRelationData, error) {
	if err := applicationID.Validate(); err != nil {
		return nil, errors.Errorf(
			"%w: %w", relationerrors.ApplicationIDNotValid, err)
	}
	return s.st.GetGoalStateRelationDataForApplication(ctx, applicationID)
}

// GetApplicationEndpoints returns all endpoints for the given application identifier.
func (s *Service) GetApplicationEndpoints(ctx context.Context, applicationID application.ID) ([]relation.Endpoint, error) {
	if err := applicationID.Validate(); err != nil {
		return nil, errors.Errorf(
			"%w: %w", relationerrors.ApplicationIDNotValid, err)
	}
	return s.st.GetApplicationEndpoints(ctx, applicationID)
}

// GetApplicationRelations returns relation UUIDs for the given
// application ID.
//
// The following error types can be expected to be returned:
//   - [relationerrors.ApplicationIDNotValid] is returned if the application
//     UUID is not valid.
//   - [relationerrors.ApplicationNotFound] is returned if the application is
//     not found.
func (s *Service) GetApplicationRelations(ctx context.Context, id application.ID) (
	[]corerelation.UUID, error) {
	if err := id.Validate(); err != nil {
		return nil, errors.Errorf(
			"%w: %w", relationerrors.ApplicationIDNotValid, err)
	}
	return s.st.GetApplicationRelations(ctx, id)
}

// GetRelationDetails returns RelationDetails for the given relationID.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationNotFound] is returned if the relation UUID
//     is not found.
//   - [relationerrors.RelationUUIDNotValid] is returned if the relation UUID
//     is not valid.
func (s *Service) GetRelationDetails(
	ctx context.Context,
	relationUUID corerelation.UUID,
) (relation.RelationDetails, error) {
	if err := relationUUID.Validate(); err != nil {
		return relation.RelationDetails{}, errors.Errorf(
			"%w: %w", relationerrors.RelationUUIDNotValid, err)
	}
	relationDetails, err := s.st.GetRelationDetails(ctx, relationUUID)
	if err != nil {
		return relation.RelationDetails{}, errors.Capture(err)
	}

	var eids []corerelation.EndpointIdentifier
	for _, e := range relationDetails.Endpoints {
		eids = append(eids, e.EndpointIdentifier())
	}
	key, err := corerelation.NewKey(eids)
	if err != nil {
		return relation.RelationDetails{}, errors.Errorf("generating relation key: %w", err)
	}

	return relation.RelationDetails{
		Life:      relationDetails.Life,
		UUID:      relationDetails.UUID,
		ID:        relationDetails.ID,
		Key:       key,
		Endpoints: relationDetails.Endpoints,
	}, nil
}

// GetRelationEndpoints returns all endpoints for the given relation UUID
func (s *Service) GetRelationEndpoints(ctx context.Context, relationUUID corerelation.UUID) ([]relation.Endpoint, error) {
	if err := relationUUID.Validate(); err != nil {
		return nil, errors.Errorf(
			"%w: %w", relationerrors.RelationUUIDNotValid, err)
	}
	return s.st.GetRelationEndpoints(ctx, relationUUID)
}

// getRelationEndpointUUID retrieves the unique identifier for a specific
// relation endpoint based on the provided arguments.
func (s *Service) getRelationEndpointUUID(ctx context.Context, args relation.GetRelationEndpointUUIDArgs) (
	corerelation.EndpointUUID, error) {
	if err := args.RelationUUID.Validate(); err != nil {
		return "", errors.Errorf(
			"%w: %w", relationerrors.RelationUUIDNotValid, err)
	}
	if err := args.ApplicationID.Validate(); err != nil {
		return "", errors.Errorf(
			"%w: %w", relationerrors.ApplicationIDNotValid, err)
	}
	return s.st.GetRelationEndpointUUID(ctx, args)
}

// GetRelationID returns the relation ID for the given relation UUID.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationNotFound] is returned if the relation UUID
//     is not found.
//   - [relationerrors.RelationUUIDNotValid] is returned if the relation UUID
//     is not valid.
func (s *Service) GetRelationID(ctx context.Context, relationUUID corerelation.UUID) (int, error) {
	if err := relationUUID.Validate(); err != nil {
		return 0, errors.Errorf(
			"%w:%w", relationerrors.RelationUUIDNotValid, err)
	}
	return s.st.GetRelationID(ctx, relationUUID)
}

// GetRelationKey returns a key identifier for the given relation UUID.
// The key describes the relation defined by endpoints in sorted order.
//
// The following error types can be expected:
//   - [relationerrors.RelationNotFound]: when no relation exists for the given
//     UUID.
//   - [relationerrors.RelationUUIDNotValid] is returned if the relation UUID
//     is not valid.
func (s *Service) GetRelationKey(ctx context.Context, relationUUID corerelation.UUID) (corerelation.Key, error) {
	if err := relationUUID.Validate(); err != nil {
		return corerelation.Key{}, errors.Errorf(
			"%w:%w", relationerrors.RelationUUIDNotValid, err)
	}

	endpoints, err := s.st.GetRelationEndpoints(ctx, relationUUID)
	if err != nil {
		return corerelation.Key{}, errors.Capture(err)
	}

	var eids []corerelation.EndpointIdentifier
	for _, ep := range endpoints {
		eids = append(eids, ep.EndpointIdentifier())
	}

	return corerelation.NewKey(eids)
}

// GetRelationsStatusForUnit returns RelationUnitStatus for all relations the
// unit is part of.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationUUIDNotValid] is returned if the relation UUID
//     is not valid.
func (s *Service) GetRelationsStatusForUnit(
	ctx context.Context,
	unitUUID unit.UUID,
) ([]relation.RelationUnitStatus, error) {
	if err := unitUUID.Validate(); err != nil {
		return nil, errors.Errorf(
			"%w:%w", relationerrors.UnitUUIDNotValid, err)
	}

	results, err := s.st.GetRelationsStatusForUnit(ctx, unitUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var statuses []relation.RelationUnitStatus
	for _, result := range results {
		var eids []corerelation.EndpointIdentifier
		for _, e := range result.Endpoints {
			eids = append(eids, e.EndpointIdentifier())
		}
		key, err := corerelation.NewKey(eids)
		if err != nil {
			return nil, errors.Errorf("generating relation key: %w", err)
		}
		statuses = append(statuses, relation.RelationUnitStatus{
			Key:       key,
			InScope:   result.InScope,
			Suspended: result.Suspended,
		})
	}

	return statuses, nil
}

// GetRelationUnit returns the relation unit UUID for the given unit for the
// given relation.
func (s *Service) GetRelationUnit(
	ctx context.Context,
	relationUUID corerelation.UUID,
	unitName unit.Name,
) (corerelation.UnitUUID, error) {
	if err := relationUUID.Validate(); err != nil {
		return "", errors.Errorf(
			"%w: %w", relationerrors.RelationUUIDNotValid, err)
	}
	if err := unitName.Validate(); err != nil {
		return "", errors.Capture(err)
	}
	return s.st.GetRelationUnit(ctx, relationUUID, unitName)
}

// GetRelationUnitByID returns the relation unit UUID for the given unit for the
// given relation.
func (s *Service) GetRelationUnitByID(
	ctx context.Context,
	relationID int,
	unitName unit.Name,
) (corerelation.UnitUUID, error) {
	if err := unitName.Validate(); err != nil {
		return "", errors.Capture(err)
	}
	uuid, err := s.st.GetRelationUUIDByID(ctx, relationID)
	if err != nil {
		return "", errors.Capture(err)
	}
	return s.st.GetRelationUnit(ctx, uuid, unitName)
}

// GetRelationUnitChanges validates the given unit and application UUIDs,
// and retrieves related unit changes.
// If any UUID is invalid, an appropriate error is returned.
func (s *Service) GetRelationUnitChanges(ctx context.Context, unitUUIDs []unit.UUID, appUUIDs []application.ID) (relation.RelationUnitsChange, error) {
	for _, uuid := range unitUUIDs {
		if err := uuid.Validate(); err != nil {
			return relation.RelationUnitsChange{}, relationerrors.UnitUUIDNotValid
		}
	}
	for _, uuid := range appUUIDs {
		if err := uuid.Validate(); err != nil {
			return relation.RelationUnitsChange{}, relationerrors.ApplicationIDNotValid
		}
	}

	return s.st.GetRelationUnitChanges(ctx, unitUUIDs, appUUIDs)
}

// GetRelationUnitEndpointName returns the name of the endpoint for the given
// relation unit.
func (s *Service) GetRelationUnitEndpointName(
	ctx context.Context,
	relationUnitUUID corerelation.UnitUUID,
) (string, error) {
	if err := relationUnitUUID.Validate(); err != nil {
		return "", errors.Capture(err)
	}
	return s.st.GetRelationUnitEndpointName(ctx, relationUnitUUID)
}

// GetRelationUnitSettings returns the relation unit settings for the given
// relation unit.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationUnitNotFound] is returned if the
//     unit is not part of the relation.
func (s *Service) GetRelationUnitSettings(
	ctx context.Context,
	relationUnitUUID corerelation.UnitUUID,
) (map[string]string, error) {
	if err := relationUnitUUID.Validate(); err != nil {
		return nil, errors.Errorf(
			"%w:%w", relationerrors.RelationUUIDNotValid, err)
	}

	return s.st.GetRelationUnitSettings(ctx, relationUnitUUID)
}

// GetRelationUUIDByID returns the relation UUID based on the relation ID.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationNotFound] is returned if the relation UUID
//     relating to the relation ID cannot be found.
func (s *Service) GetRelationUUIDByID(ctx context.Context, relationID int) (corerelation.UUID, error) {
	return s.st.GetRelationUUIDByID(ctx, relationID)
}

// GetRelationUUIDByKey returns a relation UUID for the given Key.
//
// The following error types can be expected:
//   - [relationerrors.RelationNotFound]: when no relation exists for the given
//     key.
//   - [relationerrors.RelationKeyNotValid]: when the relation key is not valid.
func (s *Service) GetRelationUUIDByKey(ctx context.Context, relationKey corerelation.Key) (corerelation.UUID, error) {
	err := relationKey.Validate()
	if err != nil {
		return "", relationerrors.RelationKeyNotValid
	}

	eids := relationKey.EndpointIdentifiers()
	var uuid corerelation.UUID
	switch len(eids) {
	case 1:
		uuid, err = s.st.GetPeerRelationUUIDByEndpointIdentifiers(
			ctx,
			eids[0],
		)
		if err != nil {
			return "", errors.Errorf("getting peer relation by key: %w", err)
		}
		return uuid, nil
	case 2:
		uuid, err = s.st.GetRegularRelationUUIDByEndpointIdentifiers(
			ctx,
			eids[0],
			eids[1],
		)
		if err != nil {
			return "", errors.Errorf("getting regular relation by key: %w", err)
		}
		return uuid, nil
	default:
		return "", errors.Errorf("internal error: unexpected number of endpoints %d", len(eids))
	}
}

// GetRemoteRelationApplicationSettings returns the application settings
// for the given application and relation identifier combination.
// Returns NotFound if the application or relation is not found.
func (s *Service) GetRemoteRelationApplicationSettings(
	ctx context.Context,
	relationUUID corerelation.UUID,
	applicationID application.ID,
) (map[string]string, error) {
	return nil, coreerrors.NotImplemented
}

// IsRelationSuspended returns a boolean to indicate if the given
// relation UUID is suspended.
func (s *Service) IsRelationSuspended(ctx context.Context, relationUUID corerelation.UUID) bool {
	return false
}

// LeaveScope updates the given relation to indicate it is not in scope.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationUnitNotFound] if the relation unit cannot be
//     found.
func (s *Service) LeaveScope(ctx context.Context, relationUnitUUID corerelation.UnitUUID) error {
	if err := relationUnitUUID.Validate(); err != nil {
		return errors.Errorf(
			"%w:%w", relationerrors.RelationUUIDNotValid, err)
	}
	return s.st.LeaveScope(ctx, relationUnitUUID)
}

// ReestablishRelation brings the given relation back to normal after
// suspension, any reason given for the suspension is cleared.
func (s *Service) ReestablishRelation(ctx context.Context, relationUUID corerelation.UUID) error {
	return coreerrors.NotImplemented
}

// RelationSuspendedReason returns the reason a relation was suspended if
// provided by the user.
func (s *Service) RelationSuspendedReason(ctx context.Context, relationUUID corerelation.UUID) string {
	return ""
}

// RelationUnitInScope returns a boolean to indicate whether the given
// relation unit is in scope.
func (s *Service) RelationUnitInScope(ctx context.Context, relationUnitUUID corerelation.UnitUUID) (bool, error) {
	return false, coreerrors.NotImplemented
}

// RelationUnitValid returns a boolean to indicate whether the given
// relation unit is in scope.
func (s *Service) RelationUnitValid(ctx context.Context, relationUnitUUID corerelation.UnitUUID) (bool, error) {
	return false, coreerrors.NotImplemented
}

// SetRelationSuspended marks the given relation as suspended. Providing a
// reason is optional.
func (s *Service) SetRelationSuspended(
	ctx context.Context,
	relationUUID corerelation.UUID,
	reason string,
) error {
	return coreerrors.NotImplemented
}

// SetRelationUnitSettings records settings for a specific unit
// relation combination.
//
// If settings is not empty, the following error types can be expected to be
// returned:
//   - [relationerrors.RelationUnitNotFound] is returned if the unit is not
//     part of the relation.
func (s *Service) SetRelationUnitSettings(
	ctx context.Context,
	relationUnitUUID corerelation.UnitUUID,
	settings map[string]string,
) error {
	if len(settings) == 0 {
		return nil
	}

	if err := relationUnitUUID.Validate(); err != nil {
		return errors.Errorf(
			"%w:%w", relationerrors.RelationUUIDNotValid, err)
	}
	return s.st.SetRelationUnitSettings(ctx, relationUnitUUID, settings)
}

// ImportRelations sets relations imported in migration.
func (s *Service) ImportRelations(ctx context.Context, args relation.ImportRelationsArgs) error {
	for _, arg := range args {
		relUUID, err := s.importRelation(ctx, arg)
		if err != nil {
			return errors.Capture(err)
		}

		for _, ep := range arg.Endpoints {
			err = s.importRelationEndpoint(ctx, relUUID, ep)
			if err != nil {
				return errors.Capture(err)
			}
		}
	}
	return nil
}

// ExportRelations returns all relation information to be exported for the
// model.
func (s *Service) ExportRelations(ctx context.Context) ([]relation.ExportRelation, error) {
	relations, err := s.st.ExportRelations(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Generate the relation keys.
	for i, r := range relations {
		var eids []corerelation.EndpointIdentifier
		for _, ep := range r.Endpoints {
			eids = append(eids, corerelation.EndpointIdentifier{
				ApplicationName: ep.ApplicationName,
				EndpointName:    ep.Name,
				Role:            ep.Role,
			})
		}
		relations[i].Key, err = corerelation.NewKey(eids)
		if err != nil {
			return nil, errors.Errorf("generating relation key: %w", err)
		}
	}

	return relations, nil
}

func (s *Service) importRelation(ctx context.Context, arg relation.ImportRelationArg) (corerelation.UUID, error) {
	var relUUID corerelation.UUID

	eps := arg.Key.EndpointIdentifiers()
	var err error

	switch len(eps) {
	case 1:
		// Peer relations are implicitly imported during migration of applications
		// during the call to CreateApplication.
		relUUID, err = s.st.GetPeerRelationUUIDByEndpointIdentifiers(ctx, eps[0])
		if err != nil {
			return relUUID, errors.Errorf("getting peer relation %d by endpoint %q: %w", arg.ID, eps[0], err)
		}
	case 2:
		relUUID, err = s.st.SetRelationWithID(ctx, eps[0], eps[1], uint64(arg.ID))
		if err != nil {
			return relUUID, errors.Capture(err)
		}
	default:
		return relUUID, errors.Errorf("unexpected number of endpoints %d for %q", len(eps), arg.Key)
	}
	return relUUID, nil
}

func (s *Service) importRelationEndpoint(ctx context.Context, relUUID corerelation.UUID, ep relation.ImportEndpoint) error {
	appID, err := s.st.GetApplicationIDByName(ctx, ep.ApplicationName)
	if err != nil {
		return err
	}

	settings, err := settingsMap(ep.ApplicationSettings)
	if err != nil {
		return err
	}
	err = s.st.SetRelationApplicationSettings(ctx, relUUID, appID, settings)
	if err != nil {
		return err
	}
	for unitName, unitSettings := range ep.UnitSettings {
		settings, err = settingsMap(unitSettings)
		if err != nil {
			return err
		}
		err = s.st.EnterScope(ctx, relUUID, unit.Name(unitName), settings)
		if err != nil {
			return err
		}
	}
	return nil
}

func settingsMap(in map[string]interface{}) (map[string]string, error) {
	var errs error
	return transform.Map(in, func(k string, v interface{}) (string, string) {
		switch v.(type) {
		case string:
		default:
			errs = errors.Join(errs, errors.Errorf("%+v no a string", v))
		}
		return k, fmt.Sprintf("%v", v)
	}), errs
}

// DeleteImportedRelations deletes all imported relations in a model during
// an import rollback.
func (s *Service) DeleteImportedRelations(
	ctx context.Context,
) error {
	return s.st.DeleteImportedRelations(ctx)
}
