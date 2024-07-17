// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"context"

	"github.com/juju/juju/environs/config"
)

// ModelConfigService is an interface for interacting with a model's underlying
// model configuration values.
type ModelConfigService interface {
	// ModelConfigValues returns the current model configuration values.
	ModelConfigValues(context.Context) (config.ConfigValues, error)
	// UpdateModelConfig updates the model configuration values.
	UpdateModelConfig(context.Context, map[string]any, []string, ...config.Validator) error
}

// ModelSecretBackendService is an interface for interacting with model secret backend service.
type ModelSecretBackendService interface {
	// GetModelSecretBackend returns the name of the secret backend configured for the model.
	GetModelSecretBackend(ctx context.Context) (string, error)
	// SetModelSecretBackend sets the secret backend for the model.
	SetModelSecretBackend(ctx context.Context, backendName string) error
}
