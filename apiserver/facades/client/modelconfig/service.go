// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"context"

	"github.com/juju/juju/environs/config"
)

// ModelConfigService is an interface for interacting the a models underlying
// model configuration values.
type ModelConfigService interface {
	ModelConfigValues(context.Context) (config.ConfigValues, error)
	UpdateModelConfig(context.Context, map[string]any, []string, ...config.Validator) error
}

// SecretBackendService is an interface for interacting with secret backend service.
type SecretBackendService interface {
	PingSecretBackend(ctx context.Context, name string) error
}
