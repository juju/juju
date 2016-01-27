// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

// This module implements a subset of the interface provided by
// state.Model, as needed by the uniter API.

// Model represents the state of a model.
type Model struct {
	name string
	uuid string
}

// UUID returns the universally unique identifier of the model.
func (e Model) UUID() string {
	return e.uuid
}

// Name returns the human friendly name of the model.
func (e Model) Name() string {
	return e.name
}
