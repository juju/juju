// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
)

var ErrModelNotDying = errors.New("model is not dying")

// ProcessDyingModel checks if the model is Dying and empty, and if so,
// transitions the model to Dead.
//
// If the model is non-empty because it is the controller model and still
// contains hosted models, an error satisfying IsHasHostedModelsError will
// be returned. If the model is otherwise non-empty, an error satisfying
// IsNonEmptyModelError will be returned.
func (st *State) ProcessDyingModel() (err error) {

	model, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}

	if model.Life() != Dying {
		return errors.Trace(ErrModelNotDying)
	}

	if st.IsController() {
		// We should not mark the controller model as Dead until
		// all hosted models have been removed, otherwise the
		// hosted model environs may not have been destroyed.
		modelUUIDs, err := st.AllModelUUIDsIncludingDead()
		if err != nil {
			return errors.Trace(err)
		}
		if n := len(modelUUIDs) - 1; n > 0 {
			return errors.Trace(newHasHostedModelsError(n))
		}
	}

	modelEntityRefsDoc, err := model.getEntityRefs()
	if err != nil {
		return errors.Trace(err)
	}
	if _, err := checkModelEntityRefsEmpty(modelEntityRefsDoc); err != nil {
		return errors.Trace(err)
	}
	return nil
}
