// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

func ValidateState(state *State) error {
	return state.validate()
}
