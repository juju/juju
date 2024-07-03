// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/environs"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("ModelConfig", 3, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacadeV3(ctx)
	}, reflect.TypeOf((*ModelConfigAPIV3)(nil)))
	registry.MustRegister("ModelConfig", 4, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*ModelConfigAPI)(nil)))
}

// newFacade is used for API registration.
func newFacade(ctx facade.ModelContext) (*ModelConfigAPI, error) {
	auth := ctx.Auth()

	model, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	serviceFactory := ctx.ServiceFactory()
	backendService := serviceFactory.SecretBackend()

	configService := serviceFactory.Config()
	configSchemaSourceGetter := environs.ProviderConfigSchemaSource(ctx.ServiceFactory().Cloud())
	return NewModelConfigAPI(
		ctx.State().ModelUUID(),
		NewStateBackend(model, configSchemaSourceGetter), backendService, configService, auth,
	)
}

// newFacadeV3 is used for API registration.
func newFacadeV3(ctx facade.ModelContext) (*ModelConfigAPIV3, error) {
	api, err := newFacade(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ModelConfigAPIV3{api}, nil
}
