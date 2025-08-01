// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/charm"
)

// Bindings defines a subset of the functionality provided by the
// state.Bindings type, as required by the application facade. For
// details on the methods, see the methods on state.Bindings with
// the same names.
type Bindings interface {
	Map() map[string]network.SpaceUUID
}

// CharmMeta describes methods that inform charm operation.
type CharmMeta interface {
	Manifest() *charm.Manifest
	Meta() *charm.Meta
}
