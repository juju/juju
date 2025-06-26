// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"fmt"
	"reflect"

	"github.com/juju/errors"

	commonmodel "github.com/juju/juju/apiserver/common/model"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/model"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegisterForMultiModel("Controller", 12, func(stdCtx context.Context, ctx facade.MultiModelContext) (facade.Facade, error) {
		api, err := makeControllerAPIV12(stdCtx, ctx)
		if err != nil {
			return nil, fmt.Errorf("creating Controller facade v12: %w", err)
		}
		return api, nil
	}, reflect.TypeOf((*ControllerAPIV12)(nil)))
	// v13 handles requests with a model qualifier instead of a model owner.
	registry.MustRegisterForMultiModel("Controller", 13, func(stdCtx context.Context, ctx facade.MultiModelContext) (facade.Facade, error) {
		api, err := makeControllerAPI(stdCtx, ctx)
		if err != nil {
			return nil, fmt.Errorf("creating Controller facade v13: %w", err)
		}
		return api, nil
	}, reflect.TypeOf((*ControllerAPI)(nil)))
}

func makeControllerAPIV12(stdCtx context.Context, ctx facade.MultiModelContext) (*ControllerAPIV12, error) {
	api, err := makeControllerAPI(stdCtx, ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ControllerAPIV12{
		ControllerAPI: api,
	}, nil
}

// makeControllerAPI creates a new ControllerAPI.
func makeControllerAPI(stdCtx context.Context, ctx facade.MultiModelContext) (*ControllerAPI, error) {
	var (
		st             = ctx.State()
		authorizer     = ctx.Auth()
		pool           = ctx.StatePool()
		resources      = ctx.Resources()
		domainServices = ctx.DomainServices()
	)

	credentialServiceGetter := func(c context.Context, modelUUID model.UUID) (CredentialService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Credential(), nil
	}
	upgradeServiceGetter := func(c context.Context, modelUUID model.UUID) (UpgradeService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Upgrade(), nil
	}
	modelAgentServiceGetter := func(c context.Context, modelUUID model.UUID) (ModelAgentService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Agent(), nil
	}
	modelConfigServiceGetter := func(c context.Context, modelUUID model.UUID) (ModelConfigService, error) {
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
	relationServiceGetter := func(c context.Context, modelUUID model.UUID) (RelationService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Relation(), nil
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
	cloudSpecServiceGetter := func(c context.Context, modelUUID model.UUID) (ModelProviderService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.ModelProvider(), nil
	}

	return NewControllerAPI(
		stdCtx,
		st,
		pool,
		authorizer,
		resources,
		ctx.Logger().Child("controller"),
		domainServices.ControllerConfig(),
		domainServices.ControllerNode(),
		domainServices.ExternalController(),
		domainServices.Access(),
		machineServiceGetter,
		domainServices.Model(),
		domainServices.ModelInfo(),
		domainServices.BlockCommand(),
		credentialServiceGetter,
		upgradeServiceGetter,
		applicationServiceGetter,
		relationServiceGetter,
		statusServiceGetter,
		modelAgentServiceGetter,
		modelConfigServiceGetter,
		blockCommandServiceGetter,
		cloudSpecServiceGetter,
		domainServices.Proxy(),
		func(c context.Context, modelUUID model.UUID, legacyState facade.LegacyStateExporter) (ModelExporter, error) {
			return ctx.ModelExporter(c, modelUUID, legacyState)
		},
		ctx.ObjectStore(),
		ctx.ControllerModelUUID(),
		ctx.ControllerUUID(),
	)
}
