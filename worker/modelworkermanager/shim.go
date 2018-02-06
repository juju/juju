// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package modelworkermanager

import (
	"github.com/juju/errors"
	"github.com/juju/juju/state"
)

// StatePoolModelGetter implements ModelGetter in terms of a *state.StatePool.
type StatePoolModelGetter struct {
	*state.StatePool
}

// Model is part of the ModelGetter interface.
func (g StatePoolModelGetter) Model(modelUUID string) (Model, func(), error) {
	model, cb, err := g.StatePool.GetModel(modelUUID)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return model, func() { cb.Release() }, nil
}
