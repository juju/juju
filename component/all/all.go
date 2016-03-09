// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The all package facilitates the registration of Juju components into
// the relevant machinery. It is intended as the one place in Juju where
// the components (horizontal design layers) and the machinery
// (vertical/architectural layers) intersect. This approach helps
// alleviate interdependence between the components and the machinery.
//
// This is done in an independent package to avoid circular imports.
package all

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"
)

type component interface {
	registerForServer() error
	registerForClient() error
}

var components = []component{
	&payloads{},
	&resources{},
}

// RegisterForServer registers all the parts of the components with the
// Juju machinery for use as a server (e.g. jujud, jujuc).
func RegisterForServer() error {
	for _, c := range components {
		if err := c.registerForServer(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// RegisterForServer registers all the parts of the components with the
// Juju machinery for use as a client (e.g. juju).
func RegisterForClient() error {
	for _, c := range components {
		if err := c.registerForClient(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// registered tracks which parts of each component have been registered.
var registered = map[string]set.Strings{}

// markRegistered helps components track which things they've
// registered. If the part has already been registered then false is
// returned, indicating that marking failed. This way components can
// ensure a part is registered only once.
func markRegistered(component, part string) bool {
	parts, ok := registered[component]
	if !ok {
		parts = set.NewStrings()
		registered[component] = parts
	}
	if parts.Contains(part) {
		return false
	}
	parts.Add(part)
	return true
}
