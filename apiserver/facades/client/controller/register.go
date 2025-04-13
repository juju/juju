// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"fmt"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common/cloudspec"
	commonmodel "github.com/juju/juju/apiserver/common/model"
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

// makeControllerAPI creates a new ControllerAPI.
func makeControllerAPI(stdCtx context.Context, ctx facade.MultiModelContext) (*ControllerAPI, error) {
	var (
		st             = ctx.State()
		authorizer     = ctx.Auth()
		pool           = ctx.StatePool()
		resources      = ctx.Resources()
		hub            = ctx.Hub()
		domainServices = ctx.DomainServices()
	)

	modelAgentServiceGetter := func(c context.Context, modelUUID model.UUID) (ModelAgentService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Agent(), nil
	}
	modelConfigServiceGetter := func(c context.Context, modelUUID model.UUID) (cloudspec.ModelConfigService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Config(), nil
	}
	applicationServiceGetter := func(c context.Context, modelUUID model.UUID) (ApplicationService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Application(), nil
	}
	statusServiceGetter := func(c context.Context, modelUUID model.UUID) (StatusService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Status(), nil
	}
	blockCommandServiceGetter := func(c context.Context, modelUUID model.UUID) (BlockCommandService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.BlockCommand(), nil
	}
	machineServiceGetter := func(c context.Context, modelUUID model.UUID) (commonmodel.MachineService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Machine(), nil
	}

	return NewControllerAPI(
		stdCtx,
		st,
		pool,
		authorizer,
		resources,
		hub,
		ctx.Logger().Child("controller"),
		domainServices.ControllerConfig(),
		domainServices.ExternalController(),
		domainServices.Cloud(),
		domainServices.Credential(),
		domainServices.Upgrade(),
		domainServices.Access(),
		machineServiceGetter,
		domainServices.Model(),
		domainServices.ModelInfo(),
		domainServices.BlockCommand(),
		applicationServiceGetter,
		statusServiceGetter,
		modelAgentServiceGetter,
		modelConfigServiceGetter,
		blockCommandServiceGetter,
		domainServices.Proxy(),
		func(c context.Context, modelUUID model.UUID, legacyState facade.LegacyStateExporter) (ModelExporter, error) {
			return ctx.ModelExporter(c, modelUUID, legacyState)
		},
		ctx.ObjectStore(),
		ctx.ControllerUUID(),
		ctx.ModelUUID(),
	)
}
