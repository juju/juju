// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/logger"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/errors"
)

// WatcherFactory is a subset of domain.WatcherFactory method that are used
// in the relation domain.
type WatcherFactory interface {
	NewNotifyWatcher(
		filter eventsource.FilterOption,
		filterOpts ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)

	NewNamespaceMapperWatcher(
		initialQuery eventsource.NamespaceQuery,
		mapper eventsource.Mapper,
		filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)
}

// WatchableService provides the API for working with applications and the
// ability to create watchers.
type WatchableService struct {
	*LeadershipService
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new watchable service reference wrapping the input state.
func NewWatchableService(
	st State,
	watcherFactory WatcherFactory,
	leaderEnsurer leadership.Ensurer,
	logger logger.Logger,
) *WatchableService {
	return &WatchableService{
		LeadershipService: NewLeadershipService(st, leaderEnsurer, logger),
		watcherFactory:    watcherFactory,
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

// WatchLifeSuspendedStatus returns a watcher that notifies of changes to
// the life or suspended status any relation the unit's application is part
// of. If the unit is a subordinate, its principal application is watched.
func (s *WatchableService) WatchLifeSuspendedStatus(
	ctx context.Context,
	unitUUID unit.UUID,
) (watcher.StringsWatcher, error) {
	return nil, coreerrors.NotImplemented
}

// WatchUnitScopes returns a watcher which notifies of counterpart units
// entering and leaving the unit's scope.
func (s *WatchableService) WatchUnitScopes(
	ctx context.Context,
	relationUnit corerelation.UnitUUID,
) (relation.RelationScopeWatcher, error) {
	return relation.RelationScopeWatcher{}, coreerrors.NotImplemented
}

// WatchRelatedUnits returns a watcher that notifies of changes to counterpart units in
// the relation.
func (s *WatchableService) WatchRelatedUnits(
	ctx context.Context,
	unitName unit.Name,
	relationUUID corerelation.UUID,
) (watcher.StringsWatcher, error) {
	if err := unitName.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	if err := relationUUID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	relationUnitNamespace, relationEndpointNamespace, initialQuery, mapper := s.st.InitialWatchRelatedUnits(unitName,
		relationUUID)
	return s.watcherFactory.NewNamespaceMapperWatcher(initialQuery,
		mapper,
		eventsource.NamespaceFilter(relationUnitNamespace, changestream.All),
		eventsource.NamespaceFilter(relationEndpointNamespace, changestream.All))
}
