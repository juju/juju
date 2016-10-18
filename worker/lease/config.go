// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/core/lease"
)

// Secretary is reponsible for validating the sanity of lease and holder names
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

	// Secretary is responsible for validating lease names and holder names.
	Secretary Secretary

	// Client is responsible for recording, retrieving, and expiring leases.
	Client lease.Client

	// Clock is reponsible for reporting the passage of time.
	Clock clock.Clock

	// MaxSleep is the longest time the Manager should sleep before
	// refreshing its client's leases and checking for expiries.
	MaxSleep time.Duration
}

// Validate returns an error if the configuration contains invalid information
// or missing resources.
func (config ManagerConfig) Validate() error {
	if config.Secretary == nil {
		return errors.NotValidf("nil Secretary")
	}
	if config.Client == nil {
		return errors.NotValidf("nil Client")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.MaxSleep <= 0 {
		return errors.NotValidf("non-positive MaxSleep")
	}
	return nil
}
