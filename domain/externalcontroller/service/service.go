// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/crossmodel"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/errors"
)

// State describes retrieval and persistence methods for storage.
type State interface {
	// Controller returns the controller record.
	Controller(ctx context.Context, controllerUUID string) (*crossmodel.ControllerInfo, error)

	// UpdateExternalController persists the input controller
	// record.
	UpdateExternalController(ctx context.Context, ec crossmodel.ControllerInfo) error

	// ModelsForController returns the list of model UUIDs for
	// the given controllerUUID.
	ModelsForController(ctx context.Context, controllerUUID string) ([]string, error)

	// ImportExternalControllers imports the list of MigrationControllerInfo
	// external controllers on one single transaction.
	ImportExternalControllers(ctx context.Context, infos []crossmodel.ControllerInfo) error

	// ControllersForModels returns the list of controllers which
	// are part of the given modelUUIDs.
	// The resulting MigrationControllerInfo contains the list of models
	// for each controller.
	ControllersForModels(ctx context.Context, modelUUIDs ...string) ([]crossmodel.ControllerInfo, error)
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewUUIDsWatcher returns a watcher that emits the UUIDs for
	// changes to the input table name that match the input mask.
	NewUUIDsWatcher(string, changestream.ChangeType) (watcher.StringsWatcher, error)
}

// Service provides the API for working with external controllers.
type Service struct {
	st State
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State) *Service {
	return &Service{
		st: st,
	}
}

// Controller returns the controller record.
func (s *Service) Controller(
	ctx context.Context,
	controllerUUID string,
) (*crossmodel.ControllerInfo, error) {
	controllerInfo, err := s.st.Controller(ctx, controllerUUID)
	return controllerInfo, errors.Errorf("retrieving external controller %s %w", controllerUUID, err)
}

// ControllerForModel returns the controller record that's associated
// with the modelUUID.
func (s *Service) ControllerForModel(
	ctx context.Context,
	modelUUID string,
) (*crossmodel.ControllerInfo, error) {
	controllers, err := s.st.ControllersForModels(ctx, modelUUID)

	if err != nil {
		return nil, errors.Errorf("retrieving external controller for model %s %w", modelUUID, err)
	}

	if len(controllers) == 0 {
		return nil, errors.Errorf("external controller for model %q %w", modelUUID, coreerrors.NotFound)
	}

	return &controllers[0], nil
}

// UpdateExternalController persists the input controller
// record and associates it with the input model UUIDs.
func (s *Service) UpdateExternalController(
	ctx context.Context, ec crossmodel.ControllerInfo,
) error {
	err := s.st.UpdateExternalController(ctx, ec)
	return errors.Errorf("updating external controller state %w", err)
}

// ImportExternalControllers imports the list of MigrationControllerInfo
// external controllers on one single transaction.
func (s *Service) ImportExternalControllers(
	ctx context.Context,
	externalControllers []crossmodel.ControllerInfo,
) error {
	return s.st.ImportExternalControllers(ctx, externalControllers)
}

// ModelsForController returns the list of model UUIDs for
// the given controllerUUID.
func (s *Service) ModelsForController(
	ctx context.Context,
	controllerUUID string,
) ([]string, error) {
	models, err := s.st.ModelsForController(ctx, controllerUUID)
	return models, errors.Errorf("retrieving model UUIDs for controller %s %w", controllerUUID, err)
}

// ControllersForModels returns the list of controllers which
// are part of the given modelUUIDs.
// The resulting MigrationControllerInfo contains the list of models
// for each controller.
func (s *Service) ControllersForModels(
	ctx context.Context,
	modelUUIDs ...string,
) ([]crossmodel.ControllerInfo, error) {
	return s.st.ControllersForModels(ctx, modelUUIDs...)
}

// WatchableService provides the API for working with external controllers
// and the ability to create watchers.
type WatchableService struct {
	Service
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new service reference wrapping the input state.
func NewWatchableService(st State, watcherFactory WatcherFactory) *WatchableService {
	return &WatchableService{
		Service: Service{
			st: st,
		},
		watcherFactory: watcherFactory,
	}
}

// Watch returns a watcher that observes changes to external controllers.
func (s *WatchableService) Watch() (watcher.StringsWatcher, error) {
	if s.watcherFactory != nil {
		return s.watcherFactory.NewUUIDsWatcher(
			"external_controller",
			changestream.Create|changestream.Update,
		)
	}
	return nil, errors.Errorf("external controller watcher %w", coreerrors.NotYetAvailable)
}
