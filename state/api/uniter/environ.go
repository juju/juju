// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

// This module implements a subset of the interface provided by
// state.Environment, as needed by the uniter API.

// Environment represents the state of an environment.
type Environment struct {
	name string
	uuid string
}

// UUID returns the universally unique identifier of the environment.
func (e Environment) UUID() string {
	return e.uuid
}

// Name returns the human friendly name of the environment.
func (e Environment) Name() string {
	return e.name
}
