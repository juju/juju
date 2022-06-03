// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CAASModelOperator", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newAPIFromContext(ctx)
	}, reflect.TypeOf((*API)(nil)))
}

// newAPIFromContext creates a new controller model facade from the supplied
// context.
func newAPIFromContext(ctx facade.Context) (*API, error) {
	authorizer := ctx.Auth()
	resources := ctx.Resources()
	systemState, err := ctx.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewAPI(authorizer, resources,
		stateShim{systemState},
		stateShim{ctx.State()})
}
