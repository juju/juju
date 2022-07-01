// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/v2/apiserver/facade"
	"github.com/juju/juju/v2/environs/context"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Subnets", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newAPIv2(ctx)
	}, reflect.TypeOf((*APIv2)(nil)))
	registry.MustRegister("Subnets", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newAPIv3(ctx)
	}, reflect.TypeOf((*APIv3)(nil)))
	registry.MustRegister("Subnets", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newAPI(ctx) // Adds SubnetsByCIDR; removes AllSpaces.
	}, reflect.TypeOf((*API)(nil)))
}

// newAPIv2 is a wrapper that creates a V2 subnets API.
func newAPIv2(ctx facade.Context) (*APIv2, error) {
	api, err := newAPIv3(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv2{api}, nil
}

// newAPIv3 is a wrapper that creates a V3 subnets API.
func newAPIv3(ctx facade.Context) (*APIv3, error) {
	api, err := newAPI(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv3{api}, nil
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
