// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"context"

	"github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/domain/blockcommand"
	"github.com/juju/juju/environs/config"
)

// ModelAgentService is the controller service for interacting with agent
// information that runs on behalf of a model.
type ModelAgentService interface {
	// SetModelAgentStream is responsible for setting the agent stream that is
	// in use for the current model. If the agent stream supplied is not a
	// recognised value an error satisfying
	// [github.com/juju/juju/core/errors.NotValid] is returned.
	SetModelAgentStream(context.Context, agentbinary.AgentStream) error
}

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
	// GetModelConstraints returns the current model constraints.
	// It returns an error satisfying [modelerrors.NotFound] if the model does not
	// exist.
	// It returns an empty Value if the model does not have any constraints
	// configured.
	GetModelConstraints(ctx context.Context) (constraints.Value, error)

	// SetModelConstraints sets the model constraints to the new values removing
	// any previously set constraints.
	//
	// The following error types can be expected:
	// - [modelerrors.NotFound]: when the model does not exist
	// - [github.com/juju/juju/domain/network/errors.SpaceNotFound]: when the space
	// being set in the model constraint doesn't exist.
	// - [github.com/juju/juju/domain/machine/errors.InvalidContainerType]: when
	// the container type being set in the model constraint isn't valid.
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
