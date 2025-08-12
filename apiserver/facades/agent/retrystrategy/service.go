// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package retrystrategy

import (
	"context"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs/config"
)

// ModelConfigService allows access to the model's configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(ctx context.Context) (*config.Config, error)
	// Watch returns a watcher that returns keys for any changes to model
	// config.
	Watch(context.Context) (watcher.StringsWatcher, error)
}
