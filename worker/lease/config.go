// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"

	"github.com/juju/juju/core/lease"
)

// Secretary is responsible for validating the sanity of lease and holder names
// before bothering the manager with them.
type Secretary interface {

	// CheckLease returns an error if the supplied lease name is not valid.
	CheckLease(name string) error

	// CheckHolder returns an error if the supplied holder name is not valid.
	CheckHolder(name string) error

	// CheckDuration returns an error if the supplied duration is not valid.
	CheckDuration(duration time.Duration) error
}

// ManagerConfig contains the resources and information required to create a
// Manager.
type ManagerConfig struct {

	// Secretary determines validation given a namespace. The
	// secretary returned is responsible for validating lease names
	// and holder names for that namespace.
	Secretary func(namespace string) (Secretary, error)

	// Store is responsible for recording, retrieving, and expiring leases.
	Store lease.Store

	// Clock is responsible for reporting the passage of time.
	Clock clock.Clock

	// MaxSleep is the longest time the Manager should sleep before
	// refreshing its store's leases and checking for expiries.
	MaxSleep time.Duration

	// EntityUUID is the entity that we are running this Manager for. Used for
	// logging purposes.
	EntityUUID string
}

// Validate returns an error if the configuration contains invalid information
// or missing resources.
func (config ManagerConfig) Validate() error {
	if config.Secretary == nil {
		return errors.NotValidf("nil Secretary")
	}
	if config.Store == nil {
		return errors.NotValidf("nil Store")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.MaxSleep <= 0 {
		return errors.NotValidf("non-positive MaxSleep")
	}
	return nil
}
