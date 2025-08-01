// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/changestream"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
)

// ObjectStoreModelState is the model state required by the provide service.
type ObjectStoreModelState interface {
	// GetModel returns the model info.
	GetModel(context.Context) (coremodel.ModelInfo, error)
}

// ObjectStoreService defines a service for interacting with the underlying model
// state, as opposed to the controller state.
type ObjectStoreService struct {
	modelSt        ObjectStoreModelState
	watcherFactory WatcherFactory
}

// NewObjectStoreService returns a new Service for interacting with a model's state.
func NewObjectStoreService(modelSt ObjectStoreModelState, watcherFactory WatcherFactory) *ObjectStoreService {
	return &ObjectStoreService{
		modelSt:        modelSt,
		watcherFactory: watcherFactory,
	}
}

// Model returns model info for the current service.
//
// The following error types can be expected to be returned:
// - [modelerrors.NotFound]: When the model is not found for a given uuid.
func (s *ObjectStoreService) Model(ctx context.Context) (coremodel.ModelInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.modelSt.GetModel(ctx)
}

// WatchModel returns a watcher that emits an event if the model changes.
func (s ObjectStoreService) WatchModel(ctx context.Context) (watcher.NotifyWatcher, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.watcherFactory.NewNotifyWatcher(
		eventsource.NamespaceFilter("model", changestream.All),
		eventsource.NamespaceFilter("model_life", changestream.All),
	)
}
