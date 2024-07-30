// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"fmt"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/model"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegisterForMultiModel("Controller", 12, func(stdCtx context.Context, ctx facade.MultiModelContext) (facade.Facade, error) {
		api, err := makeControllerAPI(stdCtx, ctx)
		if err != nil {
			return nil, fmt.Errorf("creating Controller facade v12: %w", err)
		}
		return api, nil
	}, reflect.TypeOf((*ControllerAPI)(nil)))
}

// makeControllerAPI creates a new ControllerAPI
func makeControllerAPI(stdCtx context.Context, ctx facade.MultiModelContext) (*ControllerAPI, error) {
	var (
		st             = ctx.State()
		authorizer     = ctx.Auth()
		pool           = ctx.StatePool()
		resources      = ctx.Resources()
		presence       = ctx.Presence()
		hub            = ctx.Hub()
		serviceFactory = ctx.ServiceFactory()
	)

	leadership, err := ctx.LeadershipReader()
	if err != nil {
		return nil, errors.Trace(err)
	}

	controllerModelInfo, err := serviceFactory.ModelInfo().GetModelInfo(stdCtx)
	if err != nil {
		return nil, fmt.Errorf("getting model info: %w", err)
	}

	modelConfigServiceGetter := func(modelID model.UUID) ModelConfigService {
		return ctx.ServiceFactoryForModel(modelID).Config()
	}

	return NewControllerAPI(
		controllerModelInfo.UUID,
		st,
		pool,
		authorizer,
		resources,
		presence,
		hub,
		ctx.Logger().Child("controller"),
		serviceFactory.ControllerConfig(),
		serviceFactory.ExternalController(),
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
		serviceFactory.Upgrade(),
		serviceFactory.Access(),
		serviceFactory.Model(),
		modelConfigServiceGetter,
		serviceFactory.Agent(),
		ctx.ModelExporter(st),
		ctx.ObjectStore(),
		leadership,
	)
}
