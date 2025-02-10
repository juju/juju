// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/uuid"
)

// NonTrackedConfig is a struct that contains the necessary information to
// create a non-tracked worker.
type NonTrackedConfig struct {
	// ModelType is the type of the model.
	ModelType coremodel.ModelType

	// ModelConfig is the model configuration for the provider.
	ModelConfig *config.Config

	// CloudSpec is the cloud spec for the provider.
	CloudSpec cloudspec.CloudSpec

	// ControllerUUID is the UUID of the controller that the provider is
	// associated with. This is currently only used for k8s providers.
	ControllerUUID uuid.UUID

	// GetProviderForType returns a provider for the given model type.
	GetProviderForType func(coremodel.ModelType) (GetProviderFunc, error)
	Logger             logger.Logger
}

// Validate returns an error if the config cannot be used to start a Worker.
func (config NonTrackedConfig) Validate() error {
	if config.ModelConfig == nil {
		return errors.NotValidf("nil ModelConfig")
	}
	if err := config.CloudSpec.Validate(); err != nil {
		return errors.NotValidf("CloudSpec: %v", err)
	}
	if !uuid.IsValidUUIDString(config.ControllerUUID.String()) {
		return errors.NotValidf("ControllerUUID")
	}
	if config.GetProviderForType == nil {
		return errors.NotValidf("nil GetProviderForType")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// nonTrackedWorker loads an environment, makes it available to clients. This
// is a non-tracked worker, meaning that it does not have a corresponding
// tracked worker. It environment is not updated, so if the credentials change,
// the worker must be recreated.
type nonTrackedWorker struct {
	tomb           tomb.Tomb
	internalStates chan string

	provider Provider
}

// NewNonTrackedWorker loads a provider for the given model type and cloud spec.
// This is a non-tracked worker, meaning that it does not have a corresponding
// tracked worker. It environment is not updated, so if the credentials change,
// the worker must be recreated.
func NewNonTrackedWorker(ctx context.Context, config NonTrackedConfig) (worker.Worker, error) {
	return newNonTrackedWorker(ctx, config, nil)
}

func newNonTrackedWorker(ctx context.Context, config NonTrackedConfig, internalStates chan string) (*nonTrackedWorker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	getter := nonTrackedProviderGetter{
		modelType:      config.ModelType,
		modelConfig:    config.ModelConfig,
		cloudSpec:      config.CloudSpec,
		controllerUUID: config.ControllerUUID,
	}
	// Given the model, we can now get the provider.
	newProviderType, err := config.GetProviderForType(config.ModelType)
	if err != nil {
		return nil, errors.Trace(err)
	}
	provider, _, err := newProviderType(ctx, getter)
	if err != nil {
		return nil, errors.Trace(err)
	}

	t := &nonTrackedWorker{
		internalStates: internalStates,
		provider:       provider,
	}
	t.tomb.Go(t.loop)
	return t, nil
}

// Provider returns the encapsulated Environ. It will continue to be updated in
// the background for as long as the Worker continues to run.
func (t *nonTrackedWorker) Provider() (Provider, error) {
	select {
	case <-t.tomb.Dying():
		return nil, tomb.ErrDying
	default:
		return t.provider, nil
	}
}

// Kill is part of the worker.Worker interface.
func (t *nonTrackedWorker) Kill() {
	t.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (t *nonTrackedWorker) Wait() error {
	return t.tomb.Wait()
}

// loop is the main loop of the worker.
func (t *nonTrackedWorker) loop() error {
	// Report the initial started state.
	t.reportInternalState(stateStarted)

	for {
		select {
		case <-t.tomb.Dying():
			return tomb.ErrDying
		}
	}
}

func (t *nonTrackedWorker) reportInternalState(state string) {
	select {
	case <-t.tomb.Dying():
	case t.internalStates <- state:
	default:
	}
}

type nonTrackedProviderGetter struct {
	modelType      coremodel.ModelType
	modelConfig    *config.Config
	cloudSpec      cloudspec.CloudSpec
	controllerUUID uuid.UUID
}

// ControllerUUID returns the controller UUID.
func (g nonTrackedProviderGetter) ControllerUUID() uuid.UUID {
	return g.controllerUUID
}

// ModelUUID returns the model UUID.
func (g nonTrackedProviderGetter) ModelConfig(ctx context.Context) (*config.Config, error) {
	return g.modelConfig, nil
}

// CloudSpec returns the cloud spec for the model.
func (g nonTrackedProviderGetter) CloudSpec(ctx context.Context) (cloudspec.CloudSpec, error) {
	return g.cloudSpec, nil
}
