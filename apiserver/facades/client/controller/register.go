// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/v2/apiserver/common"
	"github.com/juju/juju/v2/apiserver/common/cloudspec"
	"github.com/juju/juju/v2/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Controller", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newControllerAPIv3(ctx)
	}, reflect.TypeOf((*ControllerAPIv3)(nil)))
	registry.MustRegister("Controller", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newControllerAPIv4(ctx)
	}, reflect.TypeOf((*ControllerAPIv4)(nil)))
	registry.MustRegister("Controller", 5, func(ctx facade.Context) (facade.Facade, error) {
		return newControllerAPIv5(ctx)
	}, reflect.TypeOf((*ControllerAPIv5)(nil)))
	registry.MustRegister("Controller", 6, func(ctx facade.Context) (facade.Facade, error) {
		return newControllerAPIv6(ctx)
	}, reflect.TypeOf((*ControllerAPIv6)(nil)))
	registry.MustRegister("Controller", 7, func(ctx facade.Context) (facade.Facade, error) {
		return newControllerAPIv7(ctx)
	}, reflect.TypeOf((*ControllerAPIv7)(nil)))
	registry.MustRegister("Controller", 8, func(ctx facade.Context) (facade.Facade, error) {
		return newControllerAPIv8(ctx)
	}, reflect.TypeOf((*ControllerAPIv8)(nil)))
	registry.MustRegister("Controller", 9, func(ctx facade.Context) (facade.Facade, error) {
		return newControllerAPIv9(ctx)
	}, reflect.TypeOf((*ControllerAPIv9)(nil)))
	registry.MustRegister("Controller", 10, func(ctx facade.Context) (facade.Facade, error) {
		return newControllerAPIv10(ctx)
	}, reflect.TypeOf((*ControllerAPIv10)(nil)))
	registry.MustRegister("Controller", 11, func(ctx facade.Context) (facade.Facade, error) {
		return newControllerAPIv11(ctx)
	}, reflect.TypeOf((*ControllerAPI)(nil)))
}

// newControllerAPIv11 creates a new ControllerAPIv11
func newControllerAPIv11(ctx facade.Context) (*ControllerAPI, error) {
	st := ctx.State()
	authorizer := ctx.Auth()
	pool := ctx.StatePool()
	resources := ctx.Resources()
	presence := ctx.Presence()
	hub := ctx.Hub()
	factory := ctx.MultiwatcherFactory()
	controller := ctx.Controller()

	return NewControllerAPI(
		st,
		pool,
		authorizer,
		resources,
		presence,
		hub,
		factory,
		controller,
	)
}

// newControllerAPIv10 creates a new ControllerAPIv10.
func newControllerAPIv10(ctx facade.Context) (*ControllerAPIv10, error) {
	v11, err := newControllerAPIv11(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ControllerAPIv10{v11}, nil
}

// newControllerAPIv9 creates a new ControllerAPIv9.
func newControllerAPIv9(ctx facade.Context) (*ControllerAPIv9, error) {
	v10, err := newControllerAPIv10(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	model, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	v10.CloudSpecer = cloudspec.NewCloudSpecV1(
		ctx.Resources(),
		cloudspec.MakeCloudSpecGetter(ctx.StatePool()),
		cloudspec.MakeCloudSpecWatcherForModel(ctx.State()),
		cloudspec.MakeCloudSpecCredentialWatcherForModel(ctx.State()),
		cloudspec.MakeCloudSpecCredentialContentWatcherForModel(ctx.State()),
		common.AuthFuncForTag(model.ModelTag()),
	)
	return &ControllerAPIv9{v10}, nil
}

// newControllerAPIv8 creates a new ControllerAPIv8.
func newControllerAPIv8(ctx facade.Context) (*ControllerAPIv8, error) {
	v9, err := newControllerAPIv9(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ControllerAPIv8{v9}, nil
}

// newControllerAPIv7 creates a new ControllerAPIv7.
func newControllerAPIv7(ctx facade.Context) (*ControllerAPIv7, error) {
	v8, err := newControllerAPIv8(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ControllerAPIv7{v8}, nil
}

// newControllerAPIv6 creates a new ControllerAPIv6.
func newControllerAPIv6(ctx facade.Context) (*ControllerAPIv6, error) {
	v7, err := newControllerAPIv7(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ControllerAPIv6{v7}, nil
}

// newControllerAPIv5 creates a new ControllerAPIv5.
func newControllerAPIv5(ctx facade.Context) (*ControllerAPIv5, error) {
	v6, err := newControllerAPIv6(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ControllerAPIv5{v6}, nil
}

// newControllerAPIv4 creates a new ControllerAPIv4.
func newControllerAPIv4(ctx facade.Context) (*ControllerAPIv4, error) {
	v5, err := newControllerAPIv5(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ControllerAPIv4{v5}, nil
}

// newControllerAPIv3 creates a new ControllerAPIv3.
func newControllerAPIv3(ctx facade.Context) (*ControllerAPIv3, error) {
	v4, err := newControllerAPIv4(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ControllerAPIv3{v4}, nil
}
