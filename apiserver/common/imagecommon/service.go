// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagecommon

import (
	"context"

	"github.com/juju/juju/environs/config"
)

// ModelConfigService is an interface that provides access to model config.
type ModelConfigService interface {
	ModelConfig(ctx context.Context) (*config.Config, error)
}
