// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
)

// SetAgentVersion sets the current agent version in the state's
// environment configuration.
// This is similar to state.SetEnvironAgentVersion but it doesn't require that
// the environment have all agents at the same version already.
func SetAgentVersion(st *state.State, vers version.Number) error {
	return st.UpdateEnvironConfig(map[string]interface{}{"agent-version": vers.String()}, nil, nil)
}
