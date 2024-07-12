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
		return makeFacadeV3(stdCtx, ctx)
	}, reflect.TypeOf((*ModelConfigAPIV3)(nil)))
	registry.MustRegister("ModelConfig", 4, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return makeFacade(stdCtx, ctx)
	}, reflect.TypeOf((*ModelConfigAPI)(nil)))
}

// makeFacade is used for API registration.
func makeFacade(stdCtx context.Context, ctx facade.ModelContext) (*ModelConfigAPI, error) {
	auth := ctx.Auth()

	model, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	serviceFactory := ctx.ServiceFactory()
	modelSecretBackend := serviceFactory.ModelSecretBackend()

	configService := serviceFactory.Config()
	configSchemaSourceGetter := environs.ProviderConfigSchemaSource(ctx.ServiceFactory().Cloud())
	modelInfo, err := serviceFactory.ModelInfo().GetModelInfo(stdCtx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewModelConfigAPI(
		modelInfo.UUID,
		NewStateBackend(model, configSchemaSourceGetter),
		modelSecretBackend, configService, auth,
	)
}

// makeFacadeV3 is used for API registration.
func makeFacadeV3(stdCtx context.Context, ctx facade.ModelContext) (*ModelConfigAPIV3, error) {
	api, err := makeFacade(stdCtx, ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ModelConfigAPIV3{api}, nil
}
