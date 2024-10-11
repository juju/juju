// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"context"
	"fmt"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("ModelConfig", 3, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		facade, err := makeFacadeV3(stdCtx, ctx)
		if err != nil {
			return nil, fmt.Errorf("registering model config client facade: %w", err)
		}
		return facade, nil
	}, reflect.TypeOf((*ModelConfigAPIV3)(nil)))
	registry.MustRegister("ModelConfig", 4, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		facade, err := makeFacade(stdCtx, ctx)
		if err != nil {
			return nil, fmt.Errorf("registering model config client facade: %w", err)
		}
		return facade, nil
	}, reflect.TypeOf((*ModelConfigAPI)(nil)))
}

// makeFacade is used for API registration.
func makeFacade(stdCtx context.Context, ctx facade.ModelContext) (*ModelConfigAPI, error) {
	auth := ctx.Auth()

	model, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	domainServices := ctx.DomainServices()
	modelSecretBackend := domainServices.ModelSecretBackend()

	configService := domainServices.Config()
	modelInfo, err := domainServices.ModelInfo().GetModelInfo(stdCtx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewModelConfigAPI(
		modelInfo.UUID,
		NewStateBackend(model),
		modelSecretBackend, configService, auth,
		domainServices.BlockCommand(),
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
