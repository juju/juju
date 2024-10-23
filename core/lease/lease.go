// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import "github.com/juju/juju/core/model"

// LeaseCheckerWaiter is an interface that checks and waits if a lease is held
// by a holder.
type LeaseCheckerWaiter interface {
	Waiter
	Checker
}

// ApplicationLeaseManagerGetter is an interface that provides a method to get a
// lease manager for a given application using its UUID.
type ApplicationLeaseManagerGetter interface {
	// GetLeaseManager returns a lease manager for the given model UUID.
	GetLeaseManager(model.UUID) (LeaseCheckerWaiter, error)
}

// ModelApplicationLeaseManagerGetter is an interface that provides a method to
// get a lease manager in the scope of a model.
type ModelApplicationLeaseManagerGetter interface {
	// GetLeaseManager returns a lease manager for the given model UUID.
	GetLeaseManager() (LeaseCheckerWaiter, error)
}
