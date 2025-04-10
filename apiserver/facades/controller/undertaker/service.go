// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"context"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/secrets/provider"
)

// SecretBackendService defines the methods that the secret backend service
type SecretBackendService interface {
	// GetSecretBackendConfigForAdmin returns the secret backend configuration
	// for the given backend ID for an admin user.
	GetSecretBackendConfigForAdmin(ctx context.Context, modelUUID coremodel.UUID) (*provider.ModelBackendConfigInfo, error)
}

// ModelConfigService is an interface that provides access to the
// model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(ctx context.Context) (*config.Config, error)
	// Watch returns a watcher that returns keys for any changes to model
	// config.
	Watch() (watcher.StringsWatcher, error)
}

// ModelInfoService describe the service for interacting and reading the underlying
// model information.
type ModelInfoService interface {
	// GetModelInfo returns the readonly model information for the model in
	// question.
	GetModelInfo(context.Context) (coremodel.ModelInfo, error)
}
