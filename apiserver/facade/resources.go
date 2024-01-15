// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facade

import "github.com/juju/worker/v4"

// Resources allows you to store and retrieve Resource implementations.
//
// The lack of error returns are in deference to the existing
// implementation, not because they're a good idea.
//
// Deprecated: Resources are deprecated. Use WatcherRegistry instead.
type Resources interface {
	// Register registers the given resource. It returns a unique identifier
	// for the resource which can then be used in subsequent API requests to
	// refer to the resource.
	// Deprecated: Resources are deprecated. Use WatcherRegistry instead.
	Register(worker.Worker) string

	// Get returns the resource for the given id, or nil if there is no such
	// resource.
	// Deprecated: Resources are deprecated. Use WatcherRegistry instead.
	Get(string) worker.Worker

	// Stop stops the resource with the given id and unregisters it.
	// Deprecated: Resources are deprecated. Use WatcherRegistry instead.
	Stop(string) error
}
