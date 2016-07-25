// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateenvirons

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
)

// EnvironConfigGetter implements environs.EnvironConfigGetter
// in terms of a *state.State.
type EnvironConfigGetter struct {
	*state.State
}

// NewEnvironFunc defines the type of a function that, given a state.State,
// returns a new Environ.
type NewEnvironFunc func(*state.State) (environs.Environ, error)

// GetNewEnvironFunc returns a NewEnvironFunc, that constructs Environs
// using the given environs.NewEnvironFunc.
func GetNewEnvironFunc(newEnviron environs.NewEnvironFunc) NewEnvironFunc {
	return func(st *state.State) (environs.Environ, error) {
		g := EnvironConfigGetter{st}
		return environs.GetEnviron(g, newEnviron)
	}
}
