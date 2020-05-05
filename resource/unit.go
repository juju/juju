// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"github.com/juju/charm/v7"
)

// TODO(ericsnow) Move Unit to an internal package?

// Unit represents a Juju unit.
type Unit interface {
	// Name is the name of the Unit.
	Name() string

	// ApplicationName is the name of the application to which the unit belongs.
	ApplicationName() string

	// CharmURL returns the unit's charm URL.
	CharmURL() (*charm.URL, bool)
}
