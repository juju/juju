// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"launchpad.net/juju-core/state"
)

// TODO [waigani] replace UpdateConfig with UpdateEnvironConfig
// UpdateConfig sets the current agent version in the state's
// environment configuration.
func UpdateConfig(st *state.State, newValues map[string]interface{}) error {
	return st.UpdateEnvironConfig(newValues, []string{})
}
