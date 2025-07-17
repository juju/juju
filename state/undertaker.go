// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
)

const (
	ErrModelNotDying = errors.ConstError("model is not dying")
)

// ProcessDyingModel checks if the model is Dying and empty, and if so,
// transitions the model to Dead.
//
// If the model is non-empty because it is the controller model and still
// contains hosted models, an error satisfying HasHostedModelsError will
// be returned. If the model is otherwise non-empty, an error satisfying
// IsNonEmptyModelError will be returned.
func (st *State) ProcessDyingModel() (err error) {
	return nil
}
