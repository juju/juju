// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/juju/state"
	"github.com/juju/juju/version"
)

// SetAgentVersion sets the current agent version in the state's
// model configuration.
// This is similar to state.SetModelAgentVersion but it doesn't require that
// the model have all agents at the same version already.
func SetAgentVersion(st *state.State, vers version.Number) error {
	return st.UpdateModelConfig(map[string]interface{}{"agent-version": vers.String()}, nil, nil)
}
