// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Application", 15, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV15(ctx)
	}, reflect.TypeOf((*APIv15)(nil)))
	registry.MustRegister("Application", 16, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV16(ctx) // DestroyApplication & DestroyUnit gains dry-run
	}, reflect.TypeOf((*APIv16)(nil)))
	registry.MustRegister("Application", 17, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV17(ctx) // Drop deprecated DestroyUnits & Destroy facades
	}, reflect.TypeOf((*APIv17)(nil)))
	registry.MustRegister("Application", 18, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV18(ctx) // Added new DeployFromRepository
	}, reflect.TypeOf((*APIv18)(nil)))
	registry.MustRegister("Application", 19, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV19(ctx) // Added new DeployFromRepository
	}, reflect.TypeOf((*APIv19)(nil)))
	registry.MustRegister("Application", 20, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV20(ctx) // Remove remote space
	}, reflect.TypeOf((*APIv20)(nil)))
	registry.MustRegister("Application", 21, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV21(ctx) // Added ScaleApplication attach storage support
	}, reflect.TypeOf((*APIv21)(nil)))
}

func newFacadeV21(ctx facade.Context) (*APIv21, error) {
	api, err := newFacadeBase(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv21{api}, nil
}

func newFacadeV20(ctx facade.Context) (*APIv20, error) {
	api, err := newFacadeV21(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv20{api}, nil
}

func newFacadeV19(ctx facade.Context) (*APIv19, error) {
	api, err := newFacadeV20(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv19{api}, nil
}

func newFacadeV18(ctx facade.Context) (*APIv18, error) {
	api, err := newFacadeV19(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv18{api}, nil
}

func newFacadeV17(ctx facade.Context) (*APIv17, error) {
	api, err := newFacadeV18(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv17{api}, nil
}

func newFacadeV16(ctx facade.Context) (*APIv16, error) {
	api, err := newFacadeV17(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv16{api}, nil
}

func newFacadeV15(ctx facade.Context) (*APIv15, error) {
	api, err := newFacadeV16(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv15{api}, nil
}
