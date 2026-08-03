// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"context"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/logging"
	"github.com/juju/juju/environs/config"
)

// ModelConfigService is an interface that provides access to the
// model configuration.
type ModelConfigService interface {
	// ModelConfig reports the model logging-config value.
	ModelConfig(ctx context.Context) (*config.Config, error)

	// Watch starts a watcher for model logging-config changes.
	Watch(ctx context.Context) (watcher.StringsWatcher, error)
}

// ControllerLokiConfigService is an interface that provides access to the
// controller Loki configuration.
type ControllerLokiConfigService interface {
	// GetLokiConfig reports the controller-wide Loki configuration.
	GetLokiConfig(ctx context.Context) (logging.LokiConfig, error)

	// WatchLokiConfig starts a watcher for controller-wide Loki configuration
	// changes.
	WatchLokiConfig(ctx context.Context) (watcher.NotifyWatcher, error)
}
