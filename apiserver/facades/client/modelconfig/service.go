// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"context"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/config"
)

// ModelConfigService is an interface for interacting with a model's underlying
// model configuration values.
type ModelConfigService interface {
	ModelConfigValues(context.Context) (config.ConfigValues, error)
	UpdateModelConfig(context.Context, map[string]any, []string, ...config.Validator) error
}

// SecretBackendService is an interface for interacting with secret backend service.
type SecretBackendService interface {
	PingSecretBackend(ctx context.Context, name string) error

	GetModelSecretBackend(ctx context.Context, modelUUID coremodel.UUID) (string, error)
	SetModelSecretBackend(ctx context.Context, modelUUID coremodel.UUID, backendName string) error
}
