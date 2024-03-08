// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/version/v2"

	"github.com/juju/juju/state"
)

// SetAgentVersion sets the current agent version in the state's
// model configuration.
// This is similar to state.SetModelAgentVersion but it doesn't require that
// the model have all agents at the same version already.
func SetAgentVersion(st *state.State, vers version.Number) error {
	model, err := st.Model()
	if err != nil {
		return err
	}
	return model.UpdateModelConfig(state.NoopConfigSchemaSource, map[string]interface{}{"agent-version": vers.String()}, nil)
}
