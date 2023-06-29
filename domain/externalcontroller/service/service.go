// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/watcher"
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
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewUUIDsWatcher returns a new watcher that observes changes to
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
	return controllerInfo, errors.Annotate(err, "retrieving external controller")
}

// ControllerForModel returns the controller record that's associated
// with the modelUUID.
func (s *Service) ControllerForModel(
	ctx context.Context,
	modelUUID string,
) (*crossmodel.ControllerInfo, error) {
	controllerInfo, err := s.st.ControllerForModel(ctx, modelUUID)
	return controllerInfo, errors.Annotate(err, "retrieving external controller for model")
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
	return s.watcherFactory.NewUUIDsWatcher("external_controller", changestream.Create|changestream.Update)
}
