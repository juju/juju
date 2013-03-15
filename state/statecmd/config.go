// The statecmd package is a temporary package
// to put code that's used by both cmd/juju and state/api.
// It is intended to wither away to nothing as functionality
// gets absorbed into state and state/api as appropriate
// when the command-line commands can invoking the
// API directly.
package statecmd

import (
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// ServiceSet changes a service's configuration values.
// Values set to the empty string will be deleted.
func ServiceSet(st *state.State, p params.ServiceSet) error {
	return juju.ServiceSet(st, p)
}

// ServiceSetYAML is like ServiceSet except that the
// configuration data is specified in YAML format.
func ServiceSetYAML(st *state.State, p params.ServiceSetYAML) error {
	return juju.ServiceSetYAML(st, p)
}
