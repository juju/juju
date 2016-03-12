// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"gopkg.in/juju/charm.v6-unstable"
)

// TODO(ericsnow) Move Unit to an internal package?

// Unit represents a Juju unit.
type Unit interface {
	// Name is the name of the Unit.
	Name() string

	// ServiceName is the name of the service to which the unit belongs.
	ServiceName() string

	// CharmURL returns the unit's charm URL.
	CharmURL() (*charm.URL, bool)
}
