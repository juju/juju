// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/assumes"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/watcher"
)

// State represents a type for interacting with the underlying state.
type State interface {
	ApplicationState
	CharmState
}

// Service provides the API for working with applications.
type Service struct {
	*CharmService
	*ApplicationService
}

// NewService returns a new Service for interacting with the underlying
// application state.
func NewService(
	appSt ApplicationState, charmSt CharmState,
	params ApplicationServiceParams,
	logger logger.Logger,
) *Service {
	return &Service{
		CharmService:       NewCharmService(charmSt, logger),
		ApplicationService: NewApplicationService(appSt, params, logger),
	}
}

// WatchableService provides the API for working with charms and the
// ability to create watchers.
type WatchableService struct {
	Service
	watchableCharmService       *WatchableCharmService
	watchableApplicationService *WatchableApplicationService
}

// NewWatchableService returns a new service reference wrapping the input state.
func NewWatchableService(
	appSt ApplicationState, charmSt CharmState, watcherFactory WatcherFactory,
	params ApplicationServiceParams,
	logger logger.Logger,
	modelID coremodel.UUID,
	agentVersionGetter AgentVersionGetter,
	provider providertracker.ProviderGetter[Provider],
) *WatchableService {
	watchableCharmService := NewWatchableCharmService(charmSt, watcherFactory, logger)
	watchableApplicationService := NewWatchableApplicationService(appSt, watcherFactory, params, logger, modelID, agentVersionGetter, provider)
	return &WatchableService{
		Service: Service{
			CharmService:       &watchableCharmService.CharmService,
			ApplicationService: &watchableApplicationService.ApplicationService,
		},
		watchableCharmService:       watchableCharmService,
		watchableApplicationService: watchableApplicationService,
	}
}

// GetSupportedFeatures returns the set of features supported by the service.
func (s *WatchableService) GetSupportedFeatures(ctx context.Context) (assumes.FeatureSet, error) {
	return s.watchableApplicationService.GetSupportedFeatures(ctx)
}

// WatchCharms returns a watcher that observes changes to charms.
func (s *WatchableService) WatchCharms() (watcher.StringsWatcher, error) {
	return s.watchableCharmService.WatchCharms()
}

// WatchApplicationUnitLife returns a watcher that observes changes to the life
// of any units if an application.
func (s *WatchableService) WatchApplicationUnitLife(_ context.Context, appName string) (watcher.StringsWatcher, error) {
	return s.watchableApplicationService.WatchApplicationUnitLife(appName)
}

// WatchApplicationScale returns a watcher that observes changes to an
// application's scale.
func (s *WatchableService) WatchApplicationScale(ctx context.Context, appName string) (watcher.NotifyWatcher, error) {
	return s.watchableApplicationService.WatchApplicationScale(ctx, appName)
}
