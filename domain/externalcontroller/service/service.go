// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/crossmodel"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/trace"
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

	// NamespaceForWatchExternalController returns the namespace identifier
	// used by watchers for external controller updates.
	NamespaceForWatchExternalController() string
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewUUIDsWatcher returns a watcher that emits the UUIDs for
	// changes to the input table name that match the input mask.
	NewUUIDsWatcher(context.Context, string, string, changestream.ChangeType) (watcher.StringsWatcher, error)
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
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	controllerInfo, err := s.st.Controller(ctx, controllerUUID)
	if err != nil {
		return controllerInfo, errors.Errorf("retrieving external controller %s: %w", controllerUUID, err)
	}
	return controllerInfo, nil
}

// ControllerForModel returns the controller record that's associated
// with the modelUUID.
func (s *Service) ControllerForModel(
	ctx context.Context,
	modelUUID string,
) (*crossmodel.ControllerInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	controllers, err := s.st.ControllersForModels(ctx, modelUUID)
	if err != nil {
		return nil, errors.Errorf("retrieving external controller for model %s: %w", modelUUID, err)
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
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.st.UpdateExternalController(ctx, ec); err != nil {
		return errors.Errorf("updating external controller state: %w", err)
	}
	return nil
}

// ImportExternalControllers imports the list of MigrationControllerInfo
// external controllers on one single transaction.
func (s *Service) ImportExternalControllers(
	ctx context.Context,
	externalControllers []crossmodel.ControllerInfo,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.ImportExternalControllers(ctx, externalControllers)
}

// ModelsForController returns the list of model UUIDs for
// the given controllerUUID.
func (s *Service) ModelsForController(
	ctx context.Context,
	controllerUUID string,
) ([]string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	models, err := s.st.ModelsForController(ctx, controllerUUID)
	if err != nil {
		return models, errors.Errorf("retrieving model UUIDs for controller %s: %w", controllerUUID, err)
	}
	return models, nil
}

// ControllersForModels returns the list of controllers which
// are part of the given modelUUIDs.
// The resulting MigrationControllerInfo contains the list of models
// for each controller.
func (s *Service) ControllersForModels(
	ctx context.Context,
	modelUUIDs ...string,
) ([]crossmodel.ControllerInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
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
func (s *WatchableService) Watch(ctx context.Context) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.watcherFactory.NewUUIDsWatcher(
		ctx,
		s.st.NamespaceForWatchExternalController(),
		"external controller watcher",
		changestream.Changed,
	)
}
