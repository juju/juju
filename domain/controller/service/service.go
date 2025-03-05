// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/internal/logger"
)

var log = logger.GetLogger("juju.domain.controller.service")

// State defines an interface for interacting with the underlying state.
type State interface {
	ControllerModelUUID(ctx context.Context) (model.UUID, error)
	GetModelActivationStatus(ctx context.Context, controllerModelUUID string) (bool, error)
	AllModelActivationStatusQuery() string
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNamespaceNotifyMapperWatcher returns a new namespace notify watcher
	// for events based on the input change mask and mapper.
	NewNamespaceNotifyMapperWatcher(
		namespace string, changeMask changestream.ChangeType, mapper eventsource.Mapper,
	) (watcher.NotifyWatcher, error)

	NewNamespaceMapperWatcher(
		namespace string, changeMask changestream.ChangeType,
		initialStateQuery eventsource.NamespaceQuery, mapper eventsource.Mapper,
	) (watcher.StringsWatcher, error)
}

type WatcherFactoryGetter interface {
	GetWatcherFactory() WatcherFactory
}

// Service defines a service for interacting with the underlying state.
type Service struct {
	st             State
	watcherFactory WatcherFactory
}

// NewService returns a new Service for interacting with the underlying state.
func NewService(st State, watcherFactory WatcherFactory) *Service {
	return &Service{
		st:             st,
		watcherFactory: watcherFactory,
	}
}

// ControllerModelUUID returns the model UUID of the controller model.
func (s *Service) ControllerModelUUID(ctx context.Context) (model.UUID, error) {
	return s.st.ControllerModelUUID(ctx)
}

// Watch returns a watcher that monitors changes to models.
// func (s *Service) Watch(ctx context.Context) (watcher.NotifyWatcher, error) {
// 	mapper := func(ctx context.Context, db database.TxnRunner, changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {
// 		activatedChanges := make([]changestream.ChangeEvent, 0, len(changes))
// 		for _, change := range changes {

// 			modelUUID := change.Changed()

// 			// Watch all deleted model events.
// 			if change.Type() == changestream.Deleted {
// 				activatedChanges = append(activatedChanges, change)
// 				continue
// 			}

// 			// Check if the model is activated.
// 			modelActivationStatus, err := s.st.GetModelActivationStatus(ctx, modelUUID)
// 			if err != nil {
// 				log.Errorf(ctx, "failed to get model activation status: %v\n", err)
// 				continue
// 			}

// 			// Watch all activated model events.
// 			if modelActivationStatus {
// 				activatedChanges = append(activatedChanges, change)
// 			}
// 		}
// 		return activatedChanges, nil
// 	}

// 	return s.watcherFactory.NewNamespaceNotifyMapperWatcher(
// 		"model", changestream.All,
// 		mapper,
// 	)
// }

func (s *Service) Watch(ctx context.Context) (watcher.StringsWatcher, error) {
	mapper := func(ctx context.Context, db database.TxnRunner, changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {
		activatedChanges := make([]changestream.ChangeEvent, 0, len(changes))
		for _, change := range changes {

			modelUUID := change.Changed()

			// Watch all deleted model events.
			if change.Type() == changestream.Deleted {
				activatedChanges = append(activatedChanges, change)
				continue
			}

			// Check if the model is activated.
			modelActivationStatus, err := s.st.GetModelActivationStatus(ctx, modelUUID)
			if err != nil {
				log.Errorf(ctx, "failed to get model activation status: %v\n", err)
				continue
			}

			// Watch all activated model events.
			if modelActivationStatus {
				activatedChanges = append(activatedChanges, change)
			}
		}
		return activatedChanges, nil
	}

	return s.watcherFactory.NewNamespaceMapperWatcher(
		"model",
		changestream.All,
		eventsource.InitialNamespaceChanges(s.st.AllModelActivationStatusQuery()),
		mapper,
	)
}
