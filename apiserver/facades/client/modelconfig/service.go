// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"context"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/domain/blockcommand"
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

// ModelService is a subset of the model domain service methods.
type ModelService interface {
	// GetModelConstraints returns the current model's constraints.
	GetModelConstraints(ctx context.Context) (constraints.Value, error)
	// SetModelConstraints replaces the current model constraints.
	SetModelConstraints(ctx context.Context, cons constraints.Value) error
}

// ModelSecretBackendService is an interface for interacting with model secret backend service.
type ModelSecretBackendService interface {
	// GetModelSecretBackend returns the name of the secret backend configured for the model.
	GetModelSecretBackend(ctx context.Context) (string, error)
	// SetModelSecretBackend sets the secret backend for the model.
	SetModelSecretBackend(ctx context.Context, backendName string) error
}

// BlockCommandService defines methods for interacting with block commands.
type BlockCommandService interface {
	// GetBlockSwitchedOn returns the optional block message if it is switched
	// on for the given type.
	GetBlockSwitchedOn(ctx context.Context, t blockcommand.BlockType) (string, error)

	// GetBlocks returns all the blocks that are currently in place.
	GetBlocks(ctx context.Context) ([]blockcommand.Block, error)
}
