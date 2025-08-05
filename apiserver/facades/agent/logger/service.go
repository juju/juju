// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"context"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs/config"
)

// ModelConfigService is an interface that provides access to the
// model configuration.
type ModelConfigService interface {
	ModelConfig(ctx context.Context) (*config.Config, error)
	Watch(ctx context.Context) (watcher.StringsWatcher, error)
}
