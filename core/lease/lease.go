// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import "github.com/juju/juju/core/model"

// LeaseManagerGetter is an interface that provides a method to get a lease
// manager for a given lease using its UUID. The lease namespace could be a
// model or an application.
type LeaseManagerGetter interface {
	// GetLeaseManager returns a lease manager for the given model UUID.
	GetLeaseManager(model.UUID) (Checker, error)
}

// ModelLeaseManagerGetter is an interface that provides a method to
// get a lease manager in the scope of a model.
type ModelLeaseManagerGetter interface {
	// GetLeaseManager returns a lease manager for the given model UUID.
	GetLeaseManager() (Checker, error)
}
