// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import "github.com/juju/juju/core/watcher"

// Application broker interface
type Application interface {
	Ensure(config *ApplicationConfig) error
	Exists() (DeploymentState, error)
	Delete() error
	Watch() (watcher.NotifyWatcher, error)
}
