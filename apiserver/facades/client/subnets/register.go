// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/v3/apiserver/facade"
	"github.com/juju/juju/v3/environs/context"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Subnets", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newAPI(ctx) // Adds SubnetsByCIDR; removes AllSpaces.
	}, reflect.TypeOf((*API)(nil)))
}

// newAPI creates a new Subnets API server-side facade with a
// state.State backing.
func newAPI(ctx facade.Context) (*API, error) {
	st := ctx.State()
	stateShim, err := NewStateShim(st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newAPIWithBacking(stateShim, context.CallContext(st), ctx.Resources(), ctx.Auth())
}
