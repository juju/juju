// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Action", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newActionAPIV2(ctx)
	}, reflect.TypeOf((*APIv2)(nil)))
	registry.MustRegister("Action", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newActionAPIV3(ctx)
	}, reflect.TypeOf((*APIv3)(nil)))
	registry.MustRegister("Action", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newActionAPIV4(ctx)
	}, reflect.TypeOf((*APIv4)(nil)))
	registry.MustRegister("Action", 5, func(ctx facade.Context) (facade.Facade, error) {
		return newActionAPIV5(ctx)
	}, reflect.TypeOf((*APIv5)(nil)))
	registry.MustRegister("Action", 6, func(ctx facade.Context) (facade.Facade, error) {
		return newActionAPIV6(ctx)
	}, reflect.TypeOf((*APIv6)(nil)))
}

// newActionAPIV2 returns an initialized ActionAPI for version 2.
func newActionAPIV2(ctx facade.Context) (*APIv2, error) {
	api, err := newActionAPIV3(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv2{api}, nil
}

// newActionAPIV3 returns an initialized ActionAPI for version 3.
func newActionAPIV3(ctx facade.Context) (*APIv3, error) {
	api, err := newActionAPIV4(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv3{api}, nil
}

// newActionAPIV4 returns an initialized ActionAPI for version 4.
func newActionAPIV4(ctx facade.Context) (*APIv4, error) {
	api, err := newActionAPIV5(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv4{api}, nil
}

// newActionAPIV5 returns an initialized ActionAPI for version 5.
func newActionAPIV5(ctx facade.Context) (*APIv5, error) {
	api, err := newActionAPIV6(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv5{api}, nil
}

// newActionAPIV6 returns an initialized ActionAPI for version 6.
func newActionAPIV6(ctx facade.Context) (*APIv6, error) {
	st := ctx.State()
	api, err := newActionAPI(&stateShim{st: st}, ctx.Resources(), ctx.Auth())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv6{api}, nil
}
