// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"context"

	"github.com/juju/version/v2"
)

// ModelAgentService provides access to the Juju agent version for the model.
type ModelAgentService interface {
	// GetModelAgentVersion returns the agent version for the current model.
	GetModelAgentVersion(ctx context.Context) (version.Number, error)
}
