// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
)

// SetAgentVersion sets the current agent version in the state's
// environment configuration.
func SetAgentVersion(st *state.State, vers version.Number) error {
	return UpdateConfig(st, map[string]interface{}{"agent-version": vers.String()})
}
