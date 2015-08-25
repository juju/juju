// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"github.com/juju/errors"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/state/lease"
)

// ManagerConfig contains the resources and information required to create a
// Manager.
type ManagerConfig struct {
	Client lease.Client
	Clock  clock.Clock
}

// Validate returns an error if the configuration contains invalid information
// or missing resources.
func (config ManagerConfig) Validate() error {
	if config.Client == nil {
		return errors.New("missing client")
	}
	if config.Clock == nil {
		return errors.New("missing clock")
	}
	return nil
}
