// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/externalcontroller"
)

// State describes retrieval and persistence methods for storage.
type State interface {
	// Controller returns the controller record.
	Controller(ctx context.Context, controllerUUID string) (*crossmodel.ControllerInfo, error)

	// ControllerForModel returns the controller record that's associated
	// with the modelUUID.
	ControllerForModel(ctx context.Context, modelUUID string) (*crossmodel.ControllerInfo, error)

	// UpdateExternalController persists the input controller
	// record and associates it with the input model UUIDs.
	UpdateExternalController(ctx context.Context, ec crossmodel.ControllerInfo, modelUUIDs []string) error

	// ModelsForController returns the list of model UUIDs for
	// the given controllerUUID.
	ModelsForController(ctx context.Context, controllerUUID string) ([]string, error)

	// ImportExternalControllers imports the list of MigrationControllerInfo
	// external controllers on one single transaction.
	ImportExternalControllers(ctx context.Context, infos []externalcontroller.MigrationControllerInfo) error
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewUUIDsWatcher returns a watcher that emits the UUIDs for
	// changes to the input table name that match the input mask.
	NewUUIDsWatcher(string, changestream.ChangeType) (watcher.StringsWatcher, error)
}

// Logger facilitates emitting log messages.
type Logger interface {
	Debugf(string, ...interface{})
}

// Service provides the API for working with external controllers.
type Service struct {
	st             State
	watcherFactory WatcherFactory
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State, watcherFactory WatcherFactory) *Service {
	return &Service{
		st:             st,
		watcherFactory: watcherFactory,
	}
}

// Controller returns the controller record.
func (s *Service) Controller(
	ctx context.Context,
	controllerUUID string,
) (*crossmodel.ControllerInfo, error) {
	controllerInfo, err := s.st.Controller(ctx, controllerUUID)
	return controllerInfo, errors.Annotatef(err, "retrieving external controller %s", controllerUUID)
}

// ControllerForModel returns the controller record that's associated
// with the modelUUID.
func (s *Service) ControllerForModel(
	ctx context.Context,
	modelUUID string,
) (*crossmodel.ControllerInfo, error) {
	controllerInfo, err := s.st.ControllerForModel(ctx, modelUUID)
	return controllerInfo, errors.Annotatef(err, "retrieving external controller for model %s", modelUUID)
}

// UpdateExternalController persists the input controller
// record and associates it with the input model UUIDs.
func (s *Service) UpdateExternalController(
	ctx context.Context, ec crossmodel.ControllerInfo, modelUUIDs ...string,
) error {
	err := s.st.UpdateExternalController(ctx, ec, modelUUIDs)
	return errors.Annotate(err, "updating external controller state")
}

// Watch returns a watcher that observes changes to external controllers.
func (s *Service) Watch() (watcher.StringsWatcher, error) {
	if s.watcherFactory != nil {
		return s.watcherFactory.NewUUIDsWatcher(
			"external_controller",
			changestream.Create|changestream.Update,
		)
	}
	return nil, errors.NotYetAvailablef("external controller watcher")
}

func (s *Service) ImportExternalControllers(
	ctx context.Context,
	externalControllers []externalcontroller.MigrationControllerInfo,
) error {
	return s.st.ImportExternalControllers(ctx, externalControllers)
}

func (s *Service) ModelsForController(
	ctx context.Context,
	controllerUUID string,
) ([]string, error) {
	models, err := s.ModelsForController(ctx, controllerUUID)
	return models, errors.Annotatef(err, "retrieving model UUIDs for controller %s", controllerUUID)
}
