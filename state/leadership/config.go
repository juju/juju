// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"github.com/juju/errors"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/core/lease"
)

// ManagerConfig contains the resources and information required to create a
// Manager.
type ManagerConfig struct {

	// Secretary is responsible for validating lease names and holder names.
	Secretary Secretary

	// Client is responsible for recording, retrieving, and expiring leases.
	Client lease.Client

	// Clock is reponsible for reporting the passage of time.
	Clock clock.Clock
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
	return nil
}
