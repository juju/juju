// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"fmt"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/application/service"
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

// makeControllerAPI creates a new ControllerAPI.
func makeControllerAPI(stdCtx context.Context, ctx facade.MultiModelContext) (*ControllerAPI, error) {
	var (
		st             = ctx.State()
		authorizer     = ctx.Auth()
		pool           = ctx.StatePool()
		resources      = ctx.Resources()
		presence       = ctx.Presence()
		hub            = ctx.Hub()
		domainServices = ctx.DomainServices()
	)

	leadership, err := ctx.LeadershipReader()
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelAgentServiceGetter := func(modelID model.UUID) common.ModelAgentService {
		return ctx.DomainServicesForModel(modelID).Agent()
	}
	modelConfigServiceGetter := func(modelID model.UUID) common.ModelConfigService {
		return ctx.DomainServicesForModel(modelID).Config()
	}
	applicationServiceGetter := func(modelID model.UUID) ApplicationService {
		return ctx.DomainServicesForModel(modelID).Application(service.NotImplementedSecretService{})
	}
	blockCommandServiceGetter := func(modelID model.UUID) BlockCommandService {
		return ctx.DomainServicesForModel(modelID).BlockCommand()
	}

	return NewControllerAPI(
		stdCtx,
		st,
		pool,
		authorizer,
		resources,
		presence,
		hub,
		ctx.Logger().Child("controller"),
		domainServices.ControllerConfig(),
		domainServices.ExternalController(),
		domainServices.Cloud(),
		domainServices.Credential(),
		domainServices.Upgrade(),
		domainServices.Access(),
		domainServices.Machine(),
		domainServices.Model(),
		domainServices.BlockCommand(),
		applicationServiceGetter,
		modelAgentServiceGetter,
		modelConfigServiceGetter,
		blockCommandServiceGetter,
		domainServices.Proxy(),
		func(modelUUID model.UUID, legacyState facade.LegacyStateExporter) ModelExporter {
			return ctx.ModelExporter(modelUUID, legacyState)
		},
		ctx.ObjectStore(),
		leadership,
	)
}
