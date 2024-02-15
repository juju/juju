// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager

import (
	"github.com/juju/errors"

	"github.com/juju/juju/state"
)

// StatePoolController implements Controller in terms of a *state.StatePool.
type StatePoolController struct {
	*state.StatePool
}

// Model is part of the Controller interface.
func (g StatePoolController) Model(modelUUID string) (Model, func(), error) {
	model, ph, err := g.StatePool.GetModel(modelUUID)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return model, func() { ph.Release() }, nil
}
