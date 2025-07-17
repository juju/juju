// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	"context"

	"github.com/juju/errors"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/uuid"
)

// EphemeralConfig is a struct that contains the necessary information to
// create a ephemeral worker.
type EphemeralConfig struct {
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
}

// Validate returns an error if the config cannot be used to start a Worker.
func (config EphemeralConfig) Validate() error {
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
	return nil
}

// NewEphemeralProvider loads a new ephemeral provider for the given model type
// and cloud spec. The provider is not updated, so if the credentials change,
// the provider will have to be recreated.
func NewEphemeralProvider(ctx context.Context, config EphemeralConfig) (Provider, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	getter := ephemeralProviderGetter{
		modelConfig:    config.ModelConfig,
		cloudSpec:      config.CloudSpec,
		controllerUUID: config.ControllerUUID,
	}
	// Given the model type, we can now get the provider.
	newProviderType, err := config.GetProviderForType(config.ModelType)
	if err != nil {
		return nil, errors.Trace(err)
	}
	provider, _, err := newProviderType(ctx, getter, environs.NoopCredentialInvalidator())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return provider, nil
}

type ephemeralProviderGetter struct {
	modelConfig    *config.Config
	cloudSpec      cloudspec.CloudSpec
	controllerUUID uuid.UUID
}

// ControllerUUID returns the controller UUID.
func (g ephemeralProviderGetter) ControllerUUID(context.Context) (string, error) {
	return g.controllerUUID.String(), nil
}

// ModelConfig returns the model config.
func (g ephemeralProviderGetter) ModelConfig(ctx context.Context) (*config.Config, error) {
	return g.modelConfig, nil
}

// CloudSpec returns the cloud spec for the model.
func (g ephemeralProviderGetter) CloudSpec(ctx context.Context) (cloudspec.CloudSpec, error) {
	return g.cloudSpec, nil
}
