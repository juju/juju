// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/state/lease"
)

// ManagerConfig contains the resources and information required to create a
// Manager.
type ManagerConfig struct {

	// Client reads and writes lease data.
	Client lease.Client

	// Clock supplies time services.
	Clock clock.Clock

	// MaxSleep is the longest time the Manager should sleep before
	// refreshing its client's leases and checking for expiries.
	MaxSleep time.Duration
}

// Validate returns an error if the configuration contains invalid information
// or missing resources.
func (config ManagerConfig) Validate() error {
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
