// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/storage"
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
func NewService(st State, registry storage.ProviderRegistry, logger logger.Logger) *Service {
	return &Service{
		CharmService:       NewCharmService(st, logger),
		ApplicationService: NewApplicationService(st, registry, logger),
	}
}

// WatchableService provides the API for working with charms and the
// ability to create watchers.
type WatchableService struct {
	Service
	watchableCharmService *WatchableCharmService
	watcherFactory        WatcherFactory
}

// NewWatchableService returns a new service reference wrapping the input state.
func NewWatchableService(st State, watcherFactory WatcherFactory, registry storage.ProviderRegistry, logger logger.Logger) *WatchableService {
	watchableCharmService := NewWatchableCharmService(st, watcherFactory, logger)
	return &WatchableService{
		Service: Service{
			CharmService:       &watchableCharmService.CharmService,
			ApplicationService: NewApplicationService(st, registry, logger),
		},
		watchableCharmService: watchableCharmService,
		watcherFactory:        watcherFactory,
	}
}

// WatchCharms returns a watcher that observes changes to charms.
func (s *WatchableService) WatchCharms() (watcher.StringsWatcher, error) {
	return s.watchableCharmService.WatchCharms()
}
