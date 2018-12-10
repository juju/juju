// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cachetest

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/state"
)

// ModelChangeFromState returns a ModelChange representing the current
// model for the state object.
func ModelChangeFromState(c *gc.C, st *state.State) cache.ModelChange {
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	return ModelChange(c, model)
}

// ModelChangeFromStateErr returns a ModelChange representing the current
// model for the state object. May return an error.
func ModelChangeFromStateErr(st *state.State) (cache.ModelChange, error) {
	model, err := st.Model()
	if err != nil {
		return cache.ModelChange{}, errors.Trace(err)
	}
	return ModelChangeErr(model)
}

// ModelChange returns a ModelChange representing the current state of the model.
func ModelChange(c *gc.C, model *state.Model) cache.ModelChange {
	change, err := ModelChangeErr(model)
	c.Assert(err, jc.ErrorIsNil)
	return change
}

// ModelChangeErr returns a ModelChange representing the current state of the model.
// May return an error if unable to load config or status.
func ModelChangeErr(model *state.Model) (cache.ModelChange, error) {
	change := cache.ModelChange{
		ModelUUID: model.UUID(),
		Name:      model.Name(),
		Life:      life.Value(model.Life().String()),
		Owner:     model.Owner().Name(),
	}
	config, err := model.Config()
	if err != nil {
		return cache.ModelChange{}, errors.Trace(err)
	}
	change.Config = config.AllAttrs()
	status, err := model.Status()
	if err != nil {
		return cache.ModelChange{}, errors.Trace(err)
	}
	change.Status = status
	return change, nil
}
