// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	corerelation "github.com/juju/juju/core/relation"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	internalrelation "github.com/juju/juju/internal/relation"
)

// State describes retrieval and persistence methods for relations.
type State interface {
	// GetPrincipalApplicationID return the principal application's ID given a
	// subordinate application's ID.
	GetPrincipalApplicationID(ctx context.Context, id application.ID) (application.ID, error)

	// GetRelatedEndpointsForWatcher returns an OtherEndpointsForWatcher struct
	// for each Endpoint in a relation with the given application ID.
	GetRelatedEndpointsForWatcher(
		ctx context.Context,
		applicationID application.ID,
	) ([]relation.OtherEndpointsForWatcher, error)

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
	GetRelationEndpoints(ctx context.Context, relationUUID corerelation.UUID) ([]internalrelation.Endpoint, error)

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

	// InitialWatchLifeSuspendedStatus returns the two tables to watch for
	// a relation's Life and Suspended status when the relation contains
	// the provided application and the initial namespace query.
	InitialWatchLifeSuspendedStatus(id application.ID) (string, string, eventsource.NamespaceQuery)

	// WatcherApplicationSettingsNamespace provides the table name to set up
	// watchers for relation application settings.
	WatcherApplicationSettingsNamespace() string

	// WatchLifeSuspendedStatusMapperData returns data needed to evaluate a relation
	// uuid as part of WatchLifeSuspendedStatus eventmapper.
	// The following error types can be expected to be returned:
	//
	// The following error types can be expected to be returned:
	//   - [relationerrors.ApplicationNotFound] is returned if the application
	//     is not found.
	//   - [relationerrors.ApplicationNotFoundForRelation] is returned if the
	//     application is not part of the relation.
	//   - [relationerrors.RelationNotFound] is returned if the relation UUID
	//     is not found.
	WatchLifeSuspendedStatusMapperData(
		ctx context.Context,
		relUUID corerelation.UUID,
		id application.ID,
	) (relation.RelationLifeSuspendedData, error)
}

// WatcherFactory instances return watchers for a given namespace and UUID.
type WatcherFactory interface {
	// NewNamespaceMapperWatcher returns a new watcher that receives changes
	// from the input base watcher's db/queue. Change-log events will be emitted
	// only if the filter accepts them, and dispatching the notifications via
	// the Changes channel, once the mapper has processed them. Filtering of
	// values is done first by the filter, and then by the mapper. Based on the
	// mapper's logic a subset of them (or none) may be emitted. A filter option
	// is required, though additional filter options can be provided.
	NewNamespaceMapperWatcher(
		initialStateQuery eventsource.NamespaceQuery,
		mapper eventsource.Mapper,
		filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)

	// NewNotifyWatcher returns a new watcher that filters changes from the
	// input base watcher's db/queue. A single filter option is required, though
	// additional filter options can be provided.
	NewNotifyWatcher(
		filter eventsource.FilterOption,
		filterOpts ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)
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
//
// If the identifiers do not uniquely specify a relation, an error will be
// returned.
func (s *Service) AddRelation(ctx context.Context, ep1, ep2 string) (relation.Endpoint, relation.Endpoint, error) {
	return relation.Endpoint{}, relation.Endpoint{}, coreerrors.NotImplemented
}

// AllRelations return all uuid of all relation for the current model.
func (s *Service) AllRelations(ctx context.Context) ([]corerelation.UUID, error) {
	return nil, coreerrors.NotImplemented
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
// Note: Replaces the functionality of the relationData method in the application
// facade. Used for UnitInfo call.
func (s *Service) ApplicationRelationsInfo(
	ctx context.Context,
	applicationID application.ID,
) ([]relation.EndpointRelationData, error) {
	return nil, coreerrors.NotImplemented
}

// EnterScope indicates that the provided unit has joined the relation.
//
// The following error types can be expected to be returned:
//   - [relationerrors.PotentialRelationUnitNotValid] if the unit entering
//     scope is a subordinate and the endpoint scope is charm.ScopeContainer
//     where the other application is a principal, but not in the current
//     relation.
func (s *Service) EnterScope(
	ctx context.Context,
	relationID corerelation.UUID,
	unitName unit.Name,
) error {
	// Before entering scope, validate the proposed relation unit based on
	// RelationUnit.Valid().
	return coreerrors.NotImplemented
}

// GetApplicationEndpoints returns all endpoints for the given application identifier.
func (s *Service) GetApplicationEndpoints(ctx context.Context, id application.ID) ([]internalrelation.Endpoint, error) {
	return nil, coreerrors.NotImplemented
}

// GetLocalRelationApplicationSettings returns the application settings
// for the given application and relation identifier combination.
// ApplicationSettings may only be read by the application leader.
// Returns NotFound if this unit is not the leader, if the application or
// relation is not found.
func (s *Service) GetLocalRelationApplicationSettings(
	ctx context.Context,
	unitName unit.Name,
	relationUUID corerelation.UUID,
	applicationID application.ID,
) (map[string]string, error) {
	// TODO: (hml) 12-Mar-2025
	// Implement leadership checking here: e.g.
	// return s.leaderEnsurer.WithLeader(ctx, appName, unitName.String(), func(ctx context.Context) error {
	//		return s.st.SetRelationStatus(ctx, appID, encodedStatus)
	//	})
	return nil, coreerrors.NotImplemented
}

// GetRelatedEndpoints returns the endpoints of the relation with which
// units of the named application will establish relations.
func (s *Service) GetRelatedEndpoints(
	ctx context.Context,
	relationUUID corerelation.UUID,
	applicationName string,
) ([]internalrelation.Endpoint, error) {
	return nil, coreerrors.NotImplemented
}

// GetRelationDetails returns RelationDetails for the given relationID.
func (s *Service) GetRelationDetails(ctx context.Context, relationID int) (relation.RelationDetails, error) {
	return relation.RelationDetails{}, coreerrors.NotImplemented
}

// GetRelationDetailsForUnit RelationDetails for the given relationID
// and unit combination
func (s *Service) GetRelationDetailsForUnit(
	ctx context.Context,
	relationUUID corerelation.UUID,
	unitName unit.Name,
) (relation.RelationDetails, error) {
	// TODO (hml) 2025-03-11
	// During implementation investigate the difference between the
	// service methods returning RelationDetails and how their use
	// by the uniter facade truly differs. Are both needed?
	return relation.RelationDetails{}, coreerrors.NotImplemented
}

// GetRelationEndpoint returns the endpoint for the given application and
// relation identifier combination.
func (s *Service) GetRelationEndpoint(
	ctx context.Context,
	relationUUID corerelation.UUID,
	applicationID application.ID,
) (internalrelation.Endpoint, error) {
	return internalrelation.Endpoint{}, coreerrors.NotImplemented
}

// GetRelationEndpoints returns all endpoints for the given relation UUID
func (s *Service) GetRelationEndpoints(ctx context.Context, id corerelation.UUID) ([]internalrelation.Endpoint, error) {
	return nil, coreerrors.NotImplemented
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
		return "", errors.Errorf(
			"%w:%w", relationerrors.RelationUUIDNotValid, err)
	}

	endpoints, err := s.st.GetRelationEndpoints(ctx, relationUUID)
	if err != nil {
		return "", errors.Capture(err)
	}

	return internalrelation.NaturalKey(endpoints), nil
}

// GetRelationStatus returns the status of the given relation.
func (s *Service) GetRelationStatus(
	ctx context.Context,
	relationUUID corerelation.UUID,
) (corestatus.StatusInfo, error) {
	return corestatus.StatusInfo{}, coreerrors.NotImplemented
}

// GetRelationsStatusForUnit returns RelationUnitStatus for
// any relation the unit is part of.
func (s *Service) GetRelationsStatusForUnit(
	ctx context.Context,
	unitUUID unit.UUID,
) ([]relation.RelationUnitStatus, error) {
	return []relation.RelationUnitStatus{}, coreerrors.NotImplemented
}

// GetRelationUnit returns the relation unit UUID for the given unit for the
// given relation.
func (s *Service) GetRelationUnit(
	ctx context.Context,
	relationUUID corerelation.UUID,
	unitName unit.Name,
) (corerelation.UnitUUID, error) {
	return "", coreerrors.NotImplemented
}

// GetRelationUnitByID returns the relation unit UUID for the given unit for the
// given relation.
func (s *Service) GetRelationUnitByID(
	ctx context.Context,
	relationID int,
	unitName unit.Name,
) (corerelation.UnitUUID, error) {
	return "", coreerrors.NotImplemented
}

// GetRelationUnitSettings returns the unit settings for the
// given unit and relation identifier combination.
func (s *Service) GetRelationUnitSettings(
	ctx context.Context,
	relationUnitUUID corerelation.UnitUUID,
) (map[string]string, error) {
	return nil, coreerrors.NotImplemented
}

// GetRelationUUIDByID returns the relation UUID based on the relation ID.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationNotFound] is returned if the relation UUID
//     relating to the relation ID cannot be found.
func (s *Service) GetRelationUUIDByID(ctx context.Context, relationID int) (corerelation.UUID, error) {
	return s.st.GetRelationUUIDByID(ctx, relationID)
}

// GetRelationUUIDFromKey returns a relation UUID for the given Key.
//
// The following error types can be expected:
//   - [relationerrors.RelationNotFound]: when no relation exists for the given
//     key.
func (s *Service) GetRelationUUIDFromKey(ctx context.Context, relationKey corerelation.Key) (corerelation.UUID, error) {
	return "", coreerrors.NotImplemented
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
func (s *Service) LeaveScope(ctx context.Context, relationID corerelation.UnitUUID) error {
	return coreerrors.NotImplemented
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

// RelationUnitEndpointName returns the name of the endpoint for the given
// relation unit.
// Note: replaces calls to relUnit.Endpoint().Name in the uniter facade.
func (s *Service) RelationUnitEndpointName(
	ctx context.Context,
	relationUnitUUID corerelation.UnitUUID,
) (string, error) {
	return "", coreerrors.NotImplemented
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

// SetRelationStatus sets the status of the relation to the status provided.
// Status may only be set by the application leader.
// Returns NotFound
func (s *Service) SetRelationStatus(
	ctx context.Context,
	unitName unit.Name,
	relationUUID corerelation.UUID,
	info corestatus.StatusInfo,
) error {
	// TODO: (hml) 6-Mar-2025
	// Implement leadership checking here: e.g.
	// return s.leaderEnsurer.WithLeader(ctx, appName, unitName.String(), func(ctx context.Context) error {
	//		return s.st.SetRelationStatus(ctx, appID, encodedStatus)
	//	})
	return coreerrors.NotImplemented
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

// SetRelationApplicationSettings records settings for a specific application
// relation combination.
func (s *Service) SetRelationApplicationSettings(
	ctx context.Context,
	relationUUID corerelation.UUID,
	applicationID application.ID,
	settings map[string]string,
) error {
	// TODO: (hml) 17-Mar-2025
	// Implement leadership checking here: e.g.
	// return s.leaderEnsurer.WithLeader(ctx, appName, unitName.String(), func(ctx context.Context) error {
	//		return s.st.SetRelationStatus(ctx, appID, encodedStatus)
	//	})
	return coreerrors.NotImplemented
}

// SetRelationUnitSettings records settings for a specific unit
// relation combination.
func (s *Service) SetRelationUnitSettings(
	ctx context.Context,
	relationUnitUUID corerelation.UnitUUID,
	settings map[string]string,
) error {
	return coreerrors.NotImplemented
}

// WatchableService provides the API for working with applications and the
// ability to create watchers.
type WatchableService struct {
	*Service
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new watchable service reference wrapping the input state.
func NewWatchableService(
	st State,
	watcherFactory WatcherFactory,
	logger logger.Logger,
) *WatchableService {
	return &WatchableService{
		Service:        NewService(st, logger),
		watcherFactory: watcherFactory,
	}
}

// WatchApplicationSettings returns a notify watcher that will signal
// whenever the specified application's relation settings are changed.
func (s *WatchableService) WatchApplicationSettings(
	ctx context.Context,
	relationUUID corerelation.UUID,
	applicationID application.ID,
) (watcher.NotifyWatcher, error) {
	relationEndpointUUID, err := s.getRelationEndpointUUID(ctx, relation.GetRelationEndpointUUIDArgs{
		RelationUUID:  relationUUID,
		ApplicationID: applicationID,
	})
	if err != nil {
		return nil, errors.Capture(errors.Errorf("watch application settings: %w", err))
	}
	return s.watcherFactory.NewNotifyWatcher(
		eventsource.PredicateFilter(
			s.st.WatcherApplicationSettingsNamespace(),
			changestream.All,
			func(s string) bool {
				return s == relationEndpointUUID.String()
			},
		),
	)
}

// WatchLifeSuspendedStatus returns a watcher that notifies of changes to the life
// or suspended status of the relation.
//
// The following error types can be expected to be returned:
//   - [relationerrors.ApplicationIDNotValid] is returned if the application
//     is not valid.
func (s *WatchableService) WatchLifeSuspendedStatus(
	applicationID application.ID,
) (watcher.StringsWatcher, error) {
	if err := applicationID.Validate(); err != nil {
		return nil, errors.Errorf(
			"%w: %w", relationerrors.ApplicationIDNotValid, err)
	}
	currentRelations := make(map[corerelation.UUID]relation.RelationLifeSuspendedData)
	// returns a set of relation keys if the life or suspended status has changed
	// for any relation this application is part of.

	lifeTableName, suspendedTableName, query := s.st.InitialWatchLifeSuspendedStatus(applicationID)
	return s.watcherFactory.NewNamespaceMapperWatcher(
		func(ctx context.Context, txn database.TxnRunner) ([]string, error) {
			relationUUIDStrings, err := query(ctx, txn)
			if err != nil {
				return nil, errors.Capture(err)
			}

			initialResults := make([]string, len(relationUUIDStrings))

			for i, relUUID := range relationUUIDStrings {
				relUUID := corerelation.UUID(relUUID)
				relationData, err := s.st.WatchLifeSuspendedStatusMapperData(ctx, relUUID, applicationID)
				if errors.Is(err, relationerrors.ApplicationNotFoundForRelation) {
					continue
				} else if err != nil {
					return nil, errors.Capture(err)
				}
				currentRelations[relUUID] = relationData
				initialResults[i] = relationData.Key.String()
			}

			return initialResults, nil
		},
		func(ctx context.Context, txn database.TxnRunner, changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {
			// If there are no changes, return no changes.
			if len(changes) == 0 {
				return nil, nil
			}
			var err error
			changeEvents := []changestream.ChangeEvent{}
			changeEvents, currentRelations, err = s.changeEventsForLifeSuspendedStatusMapper(
				ctx,
				applicationID,
				changeEvents,
				currentRelations,
			)
			if err != nil {
				return nil, errors.Capture(err)
			}
			return changeEvents, nil
		},
		eventsource.NamespaceFilter(lifeTableName, changestream.All),
		eventsource.NamespaceFilter(suspendedTableName, changestream.All),
	)
}

func (s *WatchableService) changeEventsForLifeSuspendedStatusMapper(
	ctx context.Context,
	applicationID application.ID,
	changes []changestream.ChangeEvent,
	currentRelations map[corerelation.UUID]relation.RelationLifeSuspendedData,
) ([]changestream.ChangeEvent, map[corerelation.UUID]relation.RelationLifeSuspendedData, error) {

	changeEvents := make([]changestream.ChangeEvent, 0)

	for _, change := range changes {
		relUUID := corerelation.UUID(change.Changed())

		changedRelationData, err := s.st.WatchLifeSuspendedStatusMapperData(ctx, relUUID, applicationID)
		if errors.Is(err, relationerrors.ApplicationNotFoundForRelation) {
			continue
		} else if errors.Is(err, relationerrors.RelationNotFound) {
			delete(currentRelations, relUUID)
			continue
		} else if err != nil {
			return nil, currentRelations, errors.Capture(err)
		}

		// If this is a known relation where neither the Life nor
		// Suspended value have changed, do not notify.
		currentRelationData, ok := currentRelations[relUUID]
		if ok && changedRelationData.Life == currentRelationData.Life &&
			changedRelationData.Suspended == currentRelationData.Suspended {
			continue
		}

		currentRelations[relUUID] = changedRelationData
		changeEvents = append(changeEvents, newMaskedChangeIDEvent(change, changedRelationData.Key.String()))
	}

	return changeEvents, currentRelations, nil
}

// WatchPrincipalLifeSuspendedStatus returns a watcher that notifies of
// changes to the life or suspended status any relation the given principal
// application is part of. Notifications on global scoped and container
// scoped relations for the principal application are given, all others are
// filtered out.
//
// The following error types can be expected to be returned:
//   - [relationerrors.ApplicationIDNotValid] is returned if the application
//     is not valid.
func (s *WatchableService) WatchPrincipalLifeSuspendedStatus(
	ctx context.Context,
	subordinateAppID application.ID,
) (watcher.StringsWatcher, error) {
	if err := subordinateAppID.Validate(); err != nil {
		return nil, errors.Errorf(
			"%w: %w", relationerrors.ApplicationIDNotValid, err)
	}
	currentRelations := make(map[corerelation.UUID]relation.RelationLifeSuspendedData)
	// returns a set of relation keys if the life or suspended status has changed
	// for any relation this application is part of.
	principalAppID, err := s.st.GetPrincipalApplicationID(ctx, subordinateAppID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	lifeTableName, suspendedTableName, query := s.st.InitialWatchLifeSuspendedStatus(principalAppID)
	return s.watcherFactory.NewNamespaceMapperWatcher(
		func(ctx context.Context, txn database.TxnRunner) ([]string, error) {
			relationUUIDStrings, err := query(ctx, txn)
			if err != nil {
				return nil, errors.Capture(err)
			}

			initialResults := make([]string, len(relationUUIDStrings))

			for i, relUUID := range relationUUIDStrings {
				relUUID := corerelation.UUID(relUUID)
				relationData, err := s.st.WatchLifeSuspendedStatusMapperData(ctx, relUUID, principalAppID)
				if errors.Is(err, relationerrors.ApplicationNotFoundForRelation) {
					continue
				} else if err != nil {
					return nil, errors.Capture(err)
				}
				currentRelations[relUUID] = relationData
				initialResults[i] = relationData.Key.String()
			}

			return initialResults, nil
		},
		func(ctx context.Context, txn database.TxnRunner, changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {
			// If there are no changes, return no changes.
			if len(changes) == 0 {
				return nil, nil
			}
			var err error
			changeEvents := []changestream.ChangeEvent{}
			changeEvents, currentRelations, err = s.changeEventsForPrincipalLifeSuspendedStatusMapper(
				ctx,
				subordinateAppID,
				principalAppID,
				changeEvents,
				currentRelations,
			)
			if err != nil {
				return nil, errors.Capture(err)
			}
			return changeEvents, nil
		},
		eventsource.NamespaceFilter(lifeTableName, changestream.All),
		eventsource.NamespaceFilter(suspendedTableName, changestream.All),
	)
}

func (s *WatchableService) changeEventsForPrincipalLifeSuspendedStatusMapper(
	ctx context.Context,
	principalAppID application.ID,
	subordinateAppID application.ID,
	changes []changestream.ChangeEvent,
	currentRelations map[corerelation.UUID]relation.RelationLifeSuspendedData,
) ([]changestream.ChangeEvent, map[corerelation.UUID]relation.RelationLifeSuspendedData, error) {

	changeEvents := make([]changestream.ChangeEvent, 0)
	for _, change := range changes {
		relUUID := corerelation.UUID(change.Changed())

		changedRelationData, err := s.st.WatchLifeSuspendedStatusMapperData(ctx, relUUID, principalAppID)
		if errors.Is(err, relationerrors.ApplicationNotFoundForRelation) {
			continue
		} else if errors.Is(err, relationerrors.RelationNotFound) {
			delete(currentRelations, relUUID)
			continue
		} else if err != nil {
			return nil, currentRelations, errors.Capture(err)
		}

		// If this is a known relation where neither the Life nor
		// Suspended value have changed, do not notify.
		currentRelationData, ok := currentRelations[relUUID]
		if ok && changedRelationData.Life == currentRelationData.Life &&
			changedRelationData.Suspended == currentRelationData.Suspended {
			continue
		}

		ok, err = s.sendChangeEvent(ctx, relUUID, principalAppID, subordinateAppID)
		if err != nil {
			return nil, currentRelations, errors.Capture(err)
		}
		if !ok {
			continue
		}

		currentRelations[relUUID] = changedRelationData
		changeEvents = append(changeEvents, newMaskedChangeIDEvent(change, changedRelationData.Key.String()))
	}

	return changeEvents, currentRelations, nil
}

// sendChangeEvent returns true if the changeEventsForPrincipalLifeSuspendedStatusMapper
// should emit the event. An event should be emitted if:
//   - The subordinate app's endpoint in the relation is global scoped
//   - If the other app in the relation is the principal app or
//     is a subordinate application.
func (s WatchableService) sendChangeEvent(
	ctx context.Context,
	relUUID corerelation.UUID,
	principalID, subordinateAppID application.ID,
) (bool, error) {
	// relation endpoint for subordinate - is it global? yes - send event
	scope, err := s.st.GetRelationEndpointScope(ctx, relUUID, subordinateAppID)
	if err != nil {
		return false, errors.Capture(err)
	}
	if scope == charm.ScopeGlobal {
		return true, nil
	}

	otherEnds, err := s.st.GetRelatedEndpointsForWatcher(ctx, subordinateAppID)
	if err != nil {
		return false, errors.Capture(err)
	}

	for _, otherEnd := range otherEnds {
		if otherEnd.ApplicationID == principalID {
			return true, nil
		}
		if otherEnd.Subordinate {
			return true, nil
		}
	}
	return false, nil
}

type maskedChangeIDEvent struct {
	changestream.ChangeEvent
	id string
}

func newMaskedChangeIDEvent(change changestream.ChangeEvent, id string) changestream.ChangeEvent {
	return maskedChangeIDEvent{
		ChangeEvent: change,
		id:          id,
	}
}

func (m maskedChangeIDEvent) Changed() string {
	return m.id
}

// WatchUnitScopes returns a watcher which notifies of counterpart units
// entering and leaving the unit's scope.
func (s *WatchableService) WatchUnitScopes(
	ctx context.Context,
	relationUnit corerelation.UnitUUID,
) (relation.RelationScopeWatcher, error) {
	return relation.RelationScopeWatcher{}, coreerrors.NotImplemented
}

// WatchUnitRelations returns a watcher that notifies of changes to counterpart units in
// the relation.
func (s *WatchableService) WatchUnitRelations(
	ctx context.Context,
	relationUnit corerelation.UnitUUID,
) (relation.RelationUnitsWatcher, error) {
	return nil, coreerrors.NotImplemented
}
