// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/controller/state"
	"github.com/juju/juju/internal/logger"
)

var log = logger.GetLogger("juju.domain.controller.service")

// State defines an interface for interacting with the underlying state.
type State interface {
	ControllerModelUUID(ctx context.Context) (model.UUID, error)
	AllModelActivationStatusQuery() string
	GetModelActivationStatus(ctx context.Context, controllerModelUUID string) (bool, error)
	GetControllerModel(ctx context.Context, controllerUUID string) (*state.ControllerModel, error)
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNamespaceWatcher returns a new namespace watcher
	// for events based on the input change mask.
	// InitialStateQuery can be used to select only the columns that you want to observe.
	NewNamespaceMapperWatcher(
		namespace string, changeMask changestream.ChangeType, initialStateQuery eventsource.NamespaceQuery, mapper eventsource.Mapper,
	) (watcher.StringsWatcher, error)

	NewNamespaceNotifyMapperWatcher(
		namespace string, changeMask changestream.ChangeType, mapper eventsource.Mapper,
	) (watcher.NotifyWatcher, error)
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

// Watch returns a watcher that returns keys for any changes to model.
func (s *Service) Watch(ctx context.Context) (watcher.StringsWatcher, error) {
	mapper := func(ctx context.Context, db database.TxnRunner, changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {
		activatedChanges := make([]changestream.ChangeEvent, 0, len(changes))
		fmt.Printf("watched alvin \n")
		for _, change := range changes {
			fmt.Printf("changed: %+v\n", change)

			controllerModelUUID := change.Changed()

			change.Namespace()
			modelActivationStatus, err := s.st.GetModelActivationStatus(ctx, controllerModelUUID)
			if err != nil {
				log.Errorf(ctx, "failed to get model activation status: %v\n", err)
				continue
			}

			// model, err := s.st.GetControllerModel(ctx, controllerModelUUID)
			// if err != nil {
			// 	log.Errorf(ctx, "failed to get controller model: %v\n", err)
			// 	continue
			// }
			// fmt.Printf("current model: %+v\n", model)

			if modelActivationStatus {
				activatedChanges = append(activatedChanges, change)
			}
		}
		return activatedChanges, nil
	}

	return s.watcherFactory.NewNamespaceMapperWatcher(
		"model", changestream.All,
		eventsource.InitialNamespaceChanges(s.st.AllModelActivationStatusQuery()),
		mapper,
	)
	// return s.watcherFactory.NewNamespaceNotifyMapperWatcher(
	// 	"model", changestream.All,
	// 	mapper,
	// )
}
