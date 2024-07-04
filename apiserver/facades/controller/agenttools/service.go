// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agenttools

import (
	"context"

	"github.com/juju/version/v2"

	"github.com/juju/juju/environs/config"
)

// ModelConfigService provides access to the model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
}

// ModelAgentService provides access to the Juju agent version for the model.
type ModelAgentService interface {
	// GetModelAgentVersion returns the agent version for the current model.
	GetModelAgentVersion(ctx context.Context) (version.Number, error)
}
