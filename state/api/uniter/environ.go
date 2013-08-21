// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

// This module implements a subset of the interface provided by
// state.Environment, as needed by the uniter API.

// TODO: Only the required calls are added as placeholders,
// the actual implementation will come in a follow-up.

// Environment represents the state of an environment.
type Environment struct {
	st *State
}

// UUID returns the universally unique identifier of the environment.
func (e Environment) UUID() string {
	// TODO: Call Uniter.CurrentEnvironUUID()
	panic("not implemented")
}
