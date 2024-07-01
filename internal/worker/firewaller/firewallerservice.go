// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"context"

	"github.com/juju/juju/core/watcher"
)

// ServiceFactoryGetter defines an interface that returns a ServiceFactory
// for a given model UUID.
type ServiceFactoryGetter interface {
	// FactoryForModel returns a ProviderServiceFactory for the given model.
	FactoryForModel(modelUUID string) ServiceFactory
}

// ServiceFactory provides access to the services required by the provider.
type ServiceFactory interface {
	// Machine returns the machine service.
	Machine() MachineService
}

// MachineService represents the machine service provided by the provider.
type MachineService interface {
	// WatchMachines returns the read-only default model.
	WatchMachines(ctx context.Context) (watcher.StringsWatcher, error)
}
