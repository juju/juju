// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/changestream"
	corecloud "github.com/juju/juju/core/cloud"
	"github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
)

// ProviderModelState is the model state required by the provide service.
type ProviderModelState interface {
	// GetModel returns the model info.
	GetModel(context.Context) (coremodel.ModelInfo, error)
}

// ProviderControllerState is the controller state required by the provider service.
type ProviderControllerState interface {
	// GetModelCloudAndCredential returns the cloud and credential UUID for the model.
	// The following errors can be expected:
	// - [modelerrors.NotFound] if the model is not found.
	GetModelCloudAndCredential(
		ctx context.Context,
		modelUUID coremodel.UUID,
	) (corecloud.UUID, credential.UUID, error)
}

// ProviderService defines a service for interacting with the underlying model
// state, as opposed to the controller state.
type ProviderService struct {
	controllerSt   ProviderControllerState
	modelSt        ProviderModelState
	watcherFactory WatcherFactory
}

// NewProviderService returns a new Service for interacting with a model's state.
func NewProviderService(
	controllerSt ProviderControllerState, modelSt ProviderModelState, watcherFactory WatcherFactory,
) *ProviderService {
	return &ProviderService{
		controllerSt:   controllerSt,
		modelSt:        modelSt,
		watcherFactory: watcherFactory,
	}
}

// Model returns model info for the current service.
//
// The following error types can be expected to be returned:
// - [modelerrors.NotFound]: When the model is not found for a given uuid.
func (s *ProviderService) Model(ctx context.Context) (coremodel.ModelInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.modelSt.GetModel(ctx)
}

// WatchModelCloudCredential returns a new NotifyWatcher watching for changes that
// result in the cloud spec for a model changing. The changes watched for are:
// - updates to model cloud.
// - updates to model credential.
// - changes to the credential set on a model.
// The following errors can be expected:
// - [modelerrors.NotFound] when the model is not found.
func (s *ProviderService) WatchModelCloudCredential(ctx context.Context, modelUUID coremodel.UUID) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return watchModelCloudCredential(ctx, s.controllerSt, s.watcherFactory, modelUUID)
}

// WatchModel returns a watcher that emits an event if the model changes.
func (s ProviderService) WatchModel(ctx context.Context) (watcher.NotifyWatcher, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.watcherFactory.NewNotifyWatcher(
		eventsource.NamespaceFilter("model", changestream.All),
	)
}
