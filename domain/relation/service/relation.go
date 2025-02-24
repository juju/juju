// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/logger"
	corerelation "github.com/juju/juju/core/relation"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/relation"
	internalrelation "github.com/juju/juju/internal/relation"
)

// State describes retrieval and persistence methods for relations.
type State interface{}

// WatcherFactory instances return watchers for a given namespace and UUID.
type WatcherFactory interface {
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

// AddRelation adds a relation between the two provided endpoints.
func (s *Service) AddRelation(ctx context.Context, eps []relation.Endpoint) error {
	return errors.NotImplemented
}

// AllRelations return all uuid of all relation for the current model.
func (s *Service) AllRelations(ctx context.Context) ([]corerelation.UUID, error) {
	return nil, errors.NotImplemented
}

// ApplicationRelationEndpointNames returns a slice of names of the given application's
// relation endpoints.
// Note: Replaces the functionality in CharmRelations method of the application facade.
func (s *Service) ApplicationRelationEndpointNames(ctx context.Context, id application.ID) ([]string, error) {
	return nil, errors.NotImplemented
}

// ApplicationRelations returns relation UUIDs for the given
// application ID.
func (s *Service) ApplicationRelations(ctx context.Context, id application.ID) (
	[]corerelation.UUID, error) {
	return []corerelation.UUID{}, errors.NotImplemented
}

// ApplicationRelationsInfo returns all EndpointRelationData for an application.
// Note: Replaces the functionality of the relationData method in the application facade. Used
// for UnitInfo call.
func (s *Service) ApplicationRelationsInfo(
	ctx context.Context,
	applicationID application.ID,
) ([]relation.EndpointRelationData, error) {
	return nil, errors.NotImplemented
}

// EnterScope updates the given relation to indicate it is in scope.
func (s *Service) EnterScope(ctx context.Context, relationID corerelation.UnitUUID) error {
	return errors.NotImplemented
}

// GetApplicationEndpoints returns all endpoints for the given application identifier.
func (s *Service) GetApplicationEndpoints(ctx context.Context, id application.ID) ([]internalrelation.Endpoint, error) {
	return nil, errors.NotImplemented
}

// GetRelatedEndpoints returns the endpoints of the relation with which
// units of the named application will establish relations.
func (s *Service) GetRelatedEndpoints(
	ctx context.Context,
	relationUUID corerelation.UUID,
	applicationName string,
) ([]internalrelation.Endpoint, error) {
	return nil, errors.NotImplemented
}

// GetRelationApplicationSettings returns the application settings for the
// given application and relation identifier combination.
func (s *Service) GetRelationApplicationSettings(
	ctx context.Context,
	relationUUID corerelation.UUID,
	applicationID application.ID,
) (map[string]string, error) {
	return nil, errors.NotImplemented
}

// GetRelationEndpoint returns the endpoint for the given application and
// relation identifier combination.
func (s *Service) GetRelationEndpoint(
	ctx context.Context,
	relationUUID corerelation.UUID,
	applicationID application.ID,
) (internalrelation.Endpoint, error) {
	return internalrelation.Endpoint{}, errors.NotImplemented
}

// GetRelationEndpoints returns all endpoints for the given relation UUID
func (s *Service) GetRelationEndpoints(ctx context.Context, id corerelation.UUID) ([]internalrelation.Endpoint, error) {
	return nil, errors.NotImplemented
}

// GetRelationID returns the relation ID for the given relation UUID.
func (s *Service) GetRelationID(ctx context.Context, relationUUID corerelation.UUID) (int, error) {
	return -1, errors.NotImplemented
}

// GetRelationStatus returns the status of the given relation.
func (s *Service) GetRelationStatus(
	ctx context.Context,
	relationUUID corerelation.UUID,
) (corestatus.StatusInfo, error) {
	return corestatus.StatusInfo{}, errors.NotImplemented
}

// RelationString returns a key identifier for the given relation UUID.
// The key describes the relation defined by endpoints in sorted order.
// Note: See the state.relationKey() function.
func (s *Service) GetRelationString(ctx context.Context, relationUUID corerelation.UUID) string {
	return ""
}

// RelationTag returns the tag for the given relation UUID.
func (s *Service) GetRelationTag(ctx context.Context, relationUUID corerelation.UUID) names.Tag {
	return names.NewRelationTag(s.GetRelationString(ctx, relationUUID))
}

// GetRelationUnit returns the relation unit UUID for the given unit for the
// given relation.
func (s *Service) GetRelationUnit(
	ctx context.Context,
	relationUUID corerelation.UUID,
	unitName string,
) (corerelation.UnitUUID, error) {
	return "", errors.NotImplemented
}

// GetRelationApplicationSettings returns the application settings for the
// given unit and relation identifier combination.
func (s *Service) GetRelationUnitSettings(
	ctx context.Context,
	relationUnitUUID corerelation.UnitUUID,
) (map[string]string, error) {
	return nil, errors.NotImplemented
}

// IsRelationSuspended returns a boolean to indicate if the given
// relation UUID is suspended.
func (s *Service) IsRelationSuspended(ctx context.Context, relationUUID corerelation.UUID) bool {
	return false
}

// LeaveScope updates the given relation to indicate it is not in scope.
func (s *Service) LeaveScope(ctx context.Context, relationID corerelation.UnitUUID) error {
	return errors.NotImplemented
}

// ReestablishRelation brings the given relation back to normal after
// suspension, any reason given for the suspension is cleared.
func (s Service) ReestablishRelation(ctx context.Context, relationUUID corerelation.UUID) error {
	return errors.NotImplemented
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
	return "", errors.NotImplemented
}

// RelationUnitInScope returns a boolean to indicate whether the given
// relation unit is in scope.
func (s *Service) RelationUnitInScope(ctx context.Context, relationUnitUUID corerelation.UnitUUID) (bool, error) {
	return false, errors.NotImplemented
}

// RelationUnitInScope returns a boolean to indicate whether the given
// relation unit is in scope.
func (s *Service) RelationUnitValid(ctx context.Context, relationUnitUUID corerelation.UnitUUID) (bool, error) {
	return false, errors.NotImplemented
}

// SetRelationStatus sets the status of the relation to the status provided.
func (s *Service) SetRelationStatus(
	ctx context.Context,
	relationID int,
	info corestatus.StatusInfo,
) error {
	return errors.NotImplemented
}

// SetRelationSuspended marks the given relation as suspended. Providing a
// reason is optional.
func (s *Service) SetRelationSuspended(
	ctx context.Context,
	relationUUID corerelation.UUID,
	reason string,
) error {
	return errors.NotImplemented
}

func (s *Service) SetRelationApplicationSettings(
	ctx context.Context,
	relationUUID corerelation.UUID,
	applicationID application.ID,
	settings map[string]string,
) error {
	return errors.NotImplemented
}

func (s *Service) SetRelationUnitSettings(
	ctx context.Context,
	relationUnitUUID corerelation.UnitUUID,
	settings map[string]string,
) error {
	return errors.NotImplemented
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
	return nil, errors.NotImplemented
}

// WatchLifeSuspendedStatus returns a watcher that notifies of changes to the life
// or suspended status of the relation.
func (s *WatchableService) WatchLifeSuspendedStatus(
	ctx context.Context,
	relationUUID corerelation.UUID,
) (watcher.StringsWatcher, error) {
	return nil, errors.NotImplemented
}

// WatchUnitScopes returns a watcher which notifies of counterpart units
// entering and leaving the unit's scope.
func (s *WatchableService) WatchUnitScopes(
	ctx context.Context,
	relationUnit corerelation.UnitUUID,
) (relation.RelationScopeWatcher, error) {
	return relation.RelationScopeWatcher{}, errors.NotImplemented
}

// WatchUnitRelations returns a watcher that notifies of changes to counterpart units in
// the relation.
func (s *WatchableService) WatchUnitRelations(
	ctx context.Context,
	relationUnit corerelation.UnitUUID,
) (relation.RelationUnitsWatcher, error) {
	return nil, errors.NotImplemented
}
