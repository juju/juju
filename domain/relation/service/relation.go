// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/logger"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/internal/errors"
)

// State describes retrieval and persistence methods for relations.
type State interface {
	WatcherState

	// ApplicationRelationEndpointNames returns a slice of names of the given application's
	// relation endpoints.
	ApplicationRelationsInfo(
		ctx context.Context,
		applicationID application.UUID,
	) ([]relation.EndpointRelationData, error)

	// AddRelation establishes a relation between two endpoints identified
	// by ep1 and ep2 and returns the created endpoints.
	AddRelation(ctx context.Context, ep1, ep2 relation.CandidateEndpointIdentifier) (relation.Endpoint, relation.Endpoint, error)

	// NeedsSubordinateUnit checks if there is a subordinate application
	// related to the principal unit that needs a subordinate unit created.
	NeedsSubordinateUnit(
		ctx context.Context,
		relationUUID corerelation.UUID,
		principalUnitName unit.Name,
	) (*application.UUID, error)

	// EnterScope indicates that the provided unit has joined the relation.
	// When the unit has already entered its relation scope, EnterScope will report
	// success but make no changes to state. The unit's settings are created or
	// overwritten in the relation according to the supplied map.
	EnterScope(
		ctx context.Context,
		relationUUID corerelation.UUID,
		unitName unit.Name,
		settings map[string]string,
	) error

	// GetAllRelationDetails return RelationDetailResults for all relations
	// for the current model.
	GetAllRelationDetails(ctx context.Context) ([]relation.RelationDetailsResult, error)

	// GetGoalStateRelationDataForApplication returns GoalStateRelationData for
	// all relations the given application is in, modulo peer relations.
	GetGoalStateRelationDataForApplication(
		ctx context.Context,
		applicationID application.UUID,
	) ([]relation.GoalStateRelationData, error)

	// GetPeerRelationUUIDByEndpointIdentifiers gets the UUID of a peer
	// relation specified by a single endpoint identifier.
	GetPeerRelationUUIDByEndpointIdentifiers(
		ctx context.Context,
		endpoint corerelation.EndpointIdentifier,
	) (corerelation.UUID, error)

	// GetRelationApplicationSettings returns the application settings
	// for the given application and relation identifier combination.
	GetRelationApplicationSettings(
		ctx context.Context,
		relationUUID corerelation.UUID,
		applicationID application.UUID,
	) (map[string]string, error)

	// GetRelationUUIDByID returns the relation UUID based on the relation ID.
	GetRelationUUIDByID(ctx context.Context, relationID int) (corerelation.UUID, error)

	// GetRelationEndpointUUID retrieves the unique identifier for a specific
	// relation endpoint based on the provided arguments.
	GetRelationEndpointUUID(ctx context.Context, args relation.GetRelationEndpointUUIDArgs) (corerelation.EndpointUUID, error)

	// GetRegularRelationUUIDByEndpointIdentifiers gets the UUID of a regular
	// relation specified by two endpoint identifiers.
	GetRegularRelationUUIDByEndpointIdentifiers(
		ctx context.Context,
		endpoint1, endpoint2 corerelation.EndpointIdentifier,
	) (corerelation.UUID, error)

	// GetRelationsStatusForUnit returns RelationUnitStatus for all relations
	// the unit is part of.
	GetRelationsStatusForUnit(ctx context.Context, unitUUID unit.UUID) ([]relation.RelationUnitStatusResult, error)

	// GetRelationDetails returns relation details for the given relationUUID.
	GetRelationDetails(ctx context.Context, relationUUID corerelation.UUID) (relation.RelationDetailsResult, error)

	// GetRelationUnitChanges retrieves changes to relation unit states and
	// application settings for the provided UUIDs.
	//
	// It takes a list of unit UUIDs and application UUIDs, returning the
	// current setting version for each one, or departed if any unit is not
	// found
	GetRelationUnitChanges(ctx context.Context, unitUUIDs []unit.UUID, appUUIDs []application.UUID) (relation.RelationUnitsChange, error)

	// GetRelationUnit retrieves the UUID of a relation unit based on the given
	// relation UUID and unit name.
	GetRelationUnitUUID(
		ctx context.Context,
		relationUUID corerelation.UUID,
		unitName unit.Name,
	) (corerelation.UnitUUID, error)

	// GetRelationUnitSettings returns the relation unit settings for the given
	// relation unit.
	GetRelationUnitSettings(ctx context.Context, relationUnitUUID corerelation.UnitUUID) (map[string]string, error)

	// GetRelationUnitSettingsArchive retrieves the archived relation settings
	// for the input relation UUID and unit name.
	// This is a fallback for accessing relation settings for units that are no
	// longer in the relation scope, which gauarantee the ability to do.
	GetRelationUnitSettingsArchive(ctx context.Context, relationUUID, unitName string) (map[string]string, error)

	// InferRelationUUIDByEndpoints infers the relation based on two endpoints.
	InferRelationUUIDByEndpoints(
		ctx context.Context,
		epIdentifier1, epIdentifier2 relation.CandidateEndpointIdentifier,
	) (corerelation.UUID, error)

	// IsPeerRelation returns a boolean to indicate if the given
	// relation UUID is for a peer relation.
	IsPeerRelation(ctx context.Context, relationUUID string) (bool, error)

	// SetRelationApplicationAndUnitSettings records settings for a unit and
	// an application in a relation.
	SetRelationApplicationAndUnitSettings(
		ctx context.Context,
		relationUnitUUID corerelation.UnitUUID,
		applicationSettings, unitSettings map[string]string,
	) error

	// ApplicationExists checks if the given application exists.
	ApplicationExists(ctx context.Context, applicationID application.UUID) error
}

// LeadershipService provides the API for working with the statuses of
// applications and units, including the API handlers that require leadership
// checks.
type LeadershipService struct {
	*Service
	leaderEnsurer leadership.Ensurer
}

// NewLeadershipService returns a new LeadershipService for working with
// the underlying state.
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

// GetRelationApplicationSettingsWithLeader returns the application settings
// for the given application and relation identifier combination.
//
// Only the leader unit may read the settings of the application in the local
// side of the relation.
//
// The following error types can be expected to be returned:
//   - [corelease.ErrNotHeld] if the unit is not the leader.
//   - [relationerrors.ApplicationNotFoundForRelation] is returned if the
//     application is not part of the relation.
//   - [relationerrors.RelationNotFound] is returned if the relation UUID
//     is not found.
func (s *LeadershipService) GetRelationApplicationSettingsWithLeader(
	ctx context.Context,
	unitName unit.Name,
	relationUUID corerelation.UUID,
	applicationID application.UUID,
) (map[string]string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	if err := relationUUID.Validate(); err != nil {
		return nil, errors.Errorf(
			"%w:%w", relationerrors.RelationUUIDNotValid, err)
	}
	if err := applicationID.Validate(); err != nil {
		return nil, errors.Errorf(
			"%w:%w", relationerrors.ApplicationUUIDNotValid, err)
	}
	settings := make(map[string]string)
	if err := s.leaderEnsurer.WithLeader(ctx, unitName.Application(), unitName.String(),
		func(ctx context.Context) error {
			var err error
			settings, err = s.st.GetRelationApplicationSettings(ctx, relationUUID, applicationID)
			if err != nil {
				return errors.Capture(err)
			}
			return nil
		},
	); err != nil {
		return nil, errors.Capture(err)
	}

	return settings, nil
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
	relationUUID corerelation.UUID,
	applicationSettings, unitSettings map[string]string,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// Note: do not check if the settings are length 0 here, as we want to
	// enable clearing settings by passing empty maps. If the maps are nil, then
	// it becomes a no-op.
	if applicationSettings == nil && unitSettings == nil {
		return nil
	}

	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}
	if err := relationUUID.Validate(); err != nil {
		return errors.Errorf(
			"%w:%w", relationerrors.RelationUUIDNotValid, err)
	}

	relationUnitUUID, err := s.st.GetRelationUnitUUID(ctx, relationUUID, unitName)
	if err != nil {
		return errors.Capture(fmt.Errorf("getting relation unit: %w", err))
	}

	// Always ensure that we are the leader, don't be tempted to skip the
	// leader check if applicationSettings is empty, as we need to
	// ensure that the unit is allowed to set its own settings.
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
func (s *Service) AddRelation(ctx context.Context, ep1, ep2 string) (relation.Endpoint, relation.Endpoint, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

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

// ApplicationRelationsInfo returns all EndpointRelationData for an application.
//
// The following error types can be expected to be returned:
//   - [relationerrors.ApplicationUUIDNotValid] if the application UUID is not valid.
//   - [relationerrors.ApplicationNotFound] is returned if the application is
//     not found.
func (s *Service) ApplicationRelationsInfo(
	ctx context.Context,
	applicationID application.UUID,
) ([]relation.EndpointRelationData, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := applicationID.Validate(); err != nil {
		return nil, relationerrors.ApplicationUUIDNotValid
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
//   - [relationerrors.CannotEnterScopeNotAlive] if the unit or relation is not
//     alive.
//   - [relationerrors.CannotEnterScopeSubordinateNotAlive] if a subordinate
//     unit is needed but already exists and is not alive.
func (s *Service) EnterScope(
	ctx context.Context,
	relationUUID corerelation.UUID,
	unitName unit.Name,
	settings map[string]string,
	subordinateCreator relation.SubordinateCreator,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := relationUUID.Validate(); err != nil {
		return errors.Errorf(
			"%w:%w", relationerrors.RelationUUIDNotValid, err)
	}
	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}

	// Enter the unit into the relation scope.
	if err := s.st.EnterScope(ctx, relationUUID, unitName, settings); err != nil {
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
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.GetAllRelationDetails(ctx)
}

// GetGoalStateRelationDataForApplication returns GoalStateRelationData for all
// relations the given application is in, modulo peer relations. No error is
// if the application is not in any relations.
//
// The following error types can be expected to be returned:
//   - [relationerrors.ApplicationUUIDNotValid] is returned if the application
//     UUID is not valid.
func (s *Service) GetGoalStateRelationDataForApplication(
	ctx context.Context,
	applicationID application.UUID,
) ([]relation.GoalStateRelationData, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	if err := applicationID.Validate(); err != nil {
		return nil, errors.Errorf(
			"%w: %w", relationerrors.ApplicationUUIDNotValid, err)
	}
	return s.st.GetGoalStateRelationDataForApplication(ctx, applicationID)
}

// GetRelationDetails returns RelationDetails for the given relation UUID.
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
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := relationUUID.Validate(); err != nil {
		return relation.RelationDetails{}, errors.Errorf(
			"%w: %w", relationerrors.RelationUUIDNotValid, err)
	}
	relationDetails, err := s.st.GetRelationDetails(ctx, relationUUID)
	if err != nil {
		return relation.RelationDetails{}, errors.Capture(err)
	}

	var identifiers []corerelation.EndpointIdentifier
	for _, e := range relationDetails.Endpoints {
		identifiers = append(identifiers, e.EndpointIdentifier())
	}
	key, err := corerelation.NewKey(identifiers)
	if err != nil {
		return relation.RelationDetails{}, errors.Errorf("generating relation key: %w", err)
	}

	return relation.RelationDetails{
		Life:         relationDetails.Life,
		UUID:         relationDetails.UUID,
		ID:           relationDetails.ID,
		Key:          key,
		Endpoints:    relationDetails.Endpoints,
		Suspended:    relationDetails.Suspended,
		InScopeUnits: relationDetails.InScopeUnits,
	}, nil
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
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

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
		var identifiers []corerelation.EndpointIdentifier
		for _, e := range result.Endpoints {
			identifiers = append(identifiers, e.EndpointIdentifier())
		}
		key, err := corerelation.NewKey(identifiers)
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

// GetRelationUnitUUID returns the relation unit UUID for the given unit for the
// given relation.
func (s *Service) GetRelationUnitUUID(
	ctx context.Context,
	relationUUID corerelation.UUID,
	unitName unit.Name,
) (corerelation.UnitUUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := relationUUID.Validate(); err != nil {
		return "", errors.Errorf(
			"%w: %w", relationerrors.RelationUUIDNotValid, err)
	}
	if err := unitName.Validate(); err != nil {
		return "", errors.Capture(err)
	}
	return s.st.GetRelationUnitUUID(ctx, relationUUID, unitName)
}

// getRelationUnitByID returns the relation unit UUID for the given unit for the
// given relation.
func (s *Service) getRelationUnitByID(
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
	return s.st.GetRelationUnitUUID(ctx, uuid, unitName)
}

// GetRelationUnitChanges validates the given unit and application UUIDs,
// and retrieves related unit changes.
// If any UUID is invalid, an appropriate error is returned.
func (s *Service) GetRelationUnitChanges(ctx context.Context, unitUUIDs []unit.UUID, appUUIDs []application.UUID) (relation.RelationUnitsChange, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	for _, uuid := range unitUUIDs {
		if err := uuid.Validate(); err != nil {
			return relation.RelationUnitsChange{}, relationerrors.UnitUUIDNotValid
		}
	}
	for _, uuid := range appUUIDs {
		if err := uuid.Validate(); err != nil {
			return relation.RelationUnitsChange{}, relationerrors.ApplicationUUIDNotValid
		}
	}

	return s.st.GetRelationUnitChanges(ctx, unitUUIDs, appUUIDs)
}

// GetRelationUnitSettings returns the relation settings for the
// input unit in the input relation.
// If there is no relation unit entry associated with the input,
// check for archived settings in case we're dealing with a
// former relation participant.
func (s *Service) GetRelationUnitSettings(
	ctx context.Context,
	relationUUID corerelation.UUID,
	unitName unit.Name,
) (map[string]string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := relationUUID.Validate(); err != nil {
		return nil, errors.Errorf(
			"%w: %w", relationerrors.RelationUUIDNotValid, err)
	}

	if err := unitName.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	ruUUID, err := s.st.GetRelationUnitUUID(ctx, relationUUID, unitName)
	if err != nil && !errors.Is(err, relationerrors.RelationUnitNotFound) {
		return nil, errors.Capture(err)
	}

	// There is a relation-unit for this combination.
	// Try to retrieve the settings.
	// It is possible (however unlikely) that the unit left scope between
	// getting the relation unit and attempting to access the settings.
	var settings map[string]string
	if err == nil {
		settings, err = s.st.GetRelationUnitSettings(ctx, ruUUID)
	}
	if !errors.Is(err, relationerrors.RelationUnitNotFound) {
		return settings, errors.Capture(err)
	}

	// If we got here, we got a not-found error either from getting the
	// relation-unit or the settings. Check the archive.
	settings, err = s.st.GetRelationUnitSettingsArchive(ctx, relationUUID.String(), unitName.String())
	if err != nil {
		return nil, errors.Capture(err)
	}
	if len(settings) == 0 {
		return nil, relationerrors.RelationUnitNotFound
	}
	return settings, nil
}

// GetRelationUUIDByID returns the relation UUID based on the relation ID.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationNotFound] is returned if the relation UUID
//     relating to the relation ID cannot be found.
func (s *Service) GetRelationUUIDByID(ctx context.Context, relationID int) (corerelation.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	return s.st.GetRelationUUIDByID(ctx, relationID)
}

// GetRelationUUIDByKey returns a relation UUID for the given Key.
//
// The following error types can be expected:
//   - [relationerrors.RelationNotFound]: when no relation exists for the given
//     key.
//   - [relationerrors.RelationKeyNotValid]: when the relation key is not valid.
func (s *Service) GetRelationUUIDByKey(ctx context.Context, relationKey corerelation.Key) (corerelation.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := relationKey.Validate(); err != nil {
		return "", relationerrors.RelationKeyNotValid
	}

	eids := relationKey.EndpointIdentifiers()
	var uuid corerelation.UUID
	var err error
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

// GetRelationUUIDForRemoval returns the relation UUID, of the relation
// represented in GetRelationUUIDForRemovalArgs, with the understanding
// this relation will be removed by an end user. Peer relations cannot be
// removed by an end user.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationNotFound] is returned if endpoints cannot be
//     found.
func (s *Service) GetRelationUUIDForRemoval(
	ctx context.Context,
	args relation.GetRelationUUIDForRemovalArgs,
) (corerelation.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := args.Validate(); err != nil {
		return "", errors.Capture(err)
	}

	if len(args.Endpoints) == 2 {
		return s.inferRelationUUIDByEndpoints(ctx, args.Endpoints[0], args.Endpoints[1])
	}

	// If we're not finding the relation by endpoints, use the relation ID.
	// 0 is a valid relation ID. Resolve the relation ID into a relationUUID,
	// verifying it is not a peer relation.
	relUUID, err := s.st.GetRelationUUIDByID(ctx, args.RelationID)
	if err != nil {
		return relUUID, errors.Errorf("finding relation uuid for id %d: %w", args.RelationID, err)
	}
	isPeer, err := s.st.IsPeerRelation(ctx, relUUID.String())
	if err != nil {
		return relUUID, errors.Errorf("checking if peer relation %q: %w", relUUID, err)
	}
	if isPeer {
		return relUUID, errors.Errorf("cannot remove a peer relation")
	}
	return relUUID, nil
}

// inferRelationUUIDByEndpoints infers the relation based on two endpoint
// strings. Unlike with GetRelationUUIDByKey, the endpoints may not be
// fully qualified and come from a user.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationNotFound] is returned if endpoints cannot be
//     found.
func (s *Service) inferRelationUUIDByEndpoints(ctx context.Context, ep1, ep2 string) (corerelation.UUID, error) {
	idep1, err := relation.NewCandidateEndpointIdentifier(ep1)
	if err != nil {
		return "", errors.Errorf("parsing endpoint identifier %q: %w", ep1, err)
	}
	idep2, err := relation.NewCandidateEndpointIdentifier(ep2)
	if err != nil {
		return "", errors.Errorf("parsing endpoint identifier %q: %w", ep2, err)
	}
	return s.st.InferRelationUUIDByEndpoints(ctx, idep1, idep2)
}

// GetRelationApplicationSettings returns the application settings
// for the given application and relation identifier combination.
//
// This function does not check leadership, so should only be used to check
// the settings of applications on the other end of the relation to the caller.
// To get the application settings with a leadership check, use
// [LeadershipService.GetRelationApplicationSettingsWithLeader].
//
// Returns NotFound if the application or relation is not found.
func (s *Service) GetRelationApplicationSettings(
	ctx context.Context,
	relationUUID corerelation.UUID,
	applicationID application.UUID,
) (map[string]string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := relationUUID.Validate(); err != nil {
		return nil, errors.Errorf(
			"%w:%w", relationerrors.RelationUUIDNotValid, err)
	}
	if err := applicationID.Validate(); err != nil {
		return nil, errors.Errorf(
			"%w:%w", relationerrors.ApplicationUUIDNotValid, err)
	}

	settings, err := s.st.GetRelationApplicationSettings(ctx, relationUUID, applicationID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return settings, nil
}

// RelationUnitInScopeByID returns a boolean to indicate whether the given
// unit is in scopen of a given relation
func (s *Service) RelationUnitInScopeByID(ctx context.Context, relationID int, unitName unit.Name) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if _, err := s.getRelationUnitByID(ctx, relationID, unitName); errors.Is(err, relationerrors.RelationUnitNotFound) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}
	return true, nil
}

// GetRelationUnits returns the current state of the relation units.
func (s *Service) GetRelationUnits(ctx context.Context, appID application.UUID) (relation.RelationUnitChange, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return relation.RelationUnitChange{}, nil
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
