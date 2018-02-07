// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import "github.com/juju/juju/core/model"

// This module implements a subset of the interface provided by
// state.Model, as needed by the uniter API.

// Model represents the state of a model.
type Model struct {
	name      string
	uuid      string
	modelType model.ModelType
}

// UUID returns the universally unique identifier of the model.
func (m Model) UUID() string {
	return m.uuid
}

// Name returns the human friendly name of the model.
func (m Model) Name() string {
	return m.name
}

// Type returns the model type.
func (m Model) Type() model.ModelType {
	return m.modelType
}
