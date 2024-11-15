// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/assumes"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/providertracker"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/watcher"
)

// State represents a type for interacting with the underlying state.
type State interface {
	ApplicationState
	CharmState
	ResourceState
}

// Service provides the API for working with applications.
type Service struct {
	*ApplicationService
	*CharmService
	*ResourceService
}

// NewService returns a new Service for interacting with the underlying
// application state.
func NewService(
	appSt ApplicationState,
	deleteSecretSt DeleteSecretState,
	charmSt CharmState,
	resourceSt ResourceState,
	storageRegistryGetter corestorage.ModelStorageRegistryGetter,
	resourceStoreGetter ResourceStoreGetter,
	logger logger.Logger,
) *Service {
	return &Service{
		ApplicationService: NewApplicationService(appSt, deleteSecretSt, storageRegistryGetter, logger),
		CharmService:       NewCharmService(charmSt, logger),
		ResourceService:    NewResourceService(resourceSt, resourceStoreGetter, logger),
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
	appSt ApplicationState,
	deleteSecretSt DeleteSecretState,
	charmSt CharmState,
	resourceSt ResourceState,
	watcherFactory WatcherFactory,
	modelID coremodel.UUID,
	agentVersionGetter AgentVersionGetter,
	provider providertracker.ProviderGetter[Provider],
	storageRegistryGetter corestorage.ModelStorageRegistryGetter,
	resourceStoreGetter ResourceStoreGetter,
	logger logger.Logger,
) *WatchableService {
	watchableCharmService := NewWatchableCharmService(charmSt, watcherFactory, logger)
	watchableApplicationService := NewWatchableApplicationService(
		appSt, deleteSecretSt, watcherFactory, modelID, agentVersionGetter, provider, storageRegistryGetter, logger)
	return &WatchableService{
		Service: Service{
			CharmService:       &watchableCharmService.CharmService,
			ApplicationService: &watchableApplicationService.ApplicationService,
			ResourceService:    NewResourceService(resourceSt, resourceStoreGetter, logger),
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
