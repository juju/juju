// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"context"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs/config"
)

// ControllerNodeService defines the methods on the controller node service
// that are needed the proxy updater API.
type ControllerNodeService interface {
	// GetAllNoProxyAPIAddressesForAgents returns a sorted, comma separated string
	// of agent API addresses suitable for no proxy settings.
	GetAllNoProxyAPIAddressesForAgents(ctx context.Context) (string, error)
	// WatchControllerAPIAddresses returns a watcher that observes changes to the
	// controller ip addresses.
	WatchControllerAPIAddresses(context.Context) (watcher.NotifyWatcher, error)
}

// ModelConfigService provides access to the model's configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
	// Watch returns a watcher that returns keys for any changes to model
	// config.
	Watch() (watcher.StringsWatcher, error)
}
