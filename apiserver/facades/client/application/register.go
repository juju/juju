// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Application", 19, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacadeV19(stdCtx, ctx) // Added new DeployFromRepository
	}, reflect.TypeOf((*APIv19)(nil)))

	registry.MustRegister("Application", 20, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacadeV20(stdCtx, ctx) // Remove remote space, rename storage constraint to storage directive
	}, reflect.TypeOf((*APIv20)(nil)))
	registry.MustRegister("Application", 21, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacadeV21(stdCtx, ctx) // Added ScaleApplication attach storage support
	}, reflect.TypeOf((*APIv21)(nil)))
	registry.MustRegister("Application", 22, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacadeV22(stdCtx, ctx) // Added GetApplicationStorage and UpdateApplicationStorage storage constraints support
	}, reflect.TypeOf((*APIv22)(nil)))
}

func newFacadeV19(stdCtx context.Context, ctx facade.ModelContext) (*APIv19, error) {
	api, err := newFacadeV20(stdCtx, ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv19{APIv20: api}, nil
}

func newFacadeV20(stdCtx context.Context, ctx facade.ModelContext) (*APIv20, error) {
	api, err := newFacadeV21(stdCtx, ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv20{APIv21: api}, nil
}

func newFacadeV21(stdCtx context.Context, ctx facade.ModelContext) (*APIv21, error) {
	api, err := newFacadeV22(stdCtx, ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv21{api}, nil
}

func newFacadeV22(stdCtx context.Context, ctx facade.ModelContext) (*APIv22, error) {
	api, err := newFacadeBase(stdCtx, ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv22{api}, nil
}
