// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"context"
	"fmt"
	"reflect"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("ModelConfig", 3, func(_ context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		facade, err := makeFacadeV3(ctx)
		if err != nil {
			return nil, fmt.Errorf("registering model config client facade: %w", err)
		}
		return facade, nil
	}, reflect.TypeOf((*ModelConfigAPIV3)(nil)))
	registry.MustRegister("ModelConfig", 4, func(_ context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		facade, err := makeFacade(ctx)
		if err != nil {
			return nil, fmt.Errorf("registering model config client facade: %w", err)
		}
		return facade, nil
	}, reflect.TypeOf((*ModelConfigAPI)(nil)))
}

// makeFacade is used for API registration.
func makeFacade(ctx facade.ModelContext) (*ModelConfigAPI, error) {
	auth := ctx.Auth()
	if !auth.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	domainServices := ctx.DomainServices()
	return NewModelConfigAPI(
		auth,
		ctx.ControllerUUID(),
		ctx.ModelUUID(),
		domainServices.Agent(),
		domainServices.BlockCommand(),
		domainServices.Config(),
		domainServices.ModelSecretBackend(),
		domainServices.ModelInfo(),
		ctx.Logger().Child("modelconfig"),
	), nil
}

// makeFacadeV3 is used for API registration.
func makeFacadeV3(ctx facade.ModelContext) (*ModelConfigAPIV3, error) {
	api, err := makeFacade(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ModelConfigAPIV3{api}, nil
}
