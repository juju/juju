// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/facades"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/errors"
)

// Register is called to expose a package of facades onto a given registry.
func Register(requiredMigrationFacadeVersions facades.FacadeVersions) func(registry facade.FacadeRegistry) {
	return func(registry facade.FacadeRegistry) {
		registry.MustRegisterForMultiModel("MigrationTarget", 4, func(stdCtx context.Context, ctx facade.MultiModelContext) (facade.Facade, error) {
			api, err := makeFacadeV4(stdCtx, ctx, requiredMigrationFacadeVersions)
			if err != nil {
				return nil, errors.Errorf("making migration target version 4: %w", err)
			}
			return api, nil
		}, reflect.TypeOf((*APIV4)(nil)))
		// Bumped to version 5 for the addition of the token field in
		// the Migration.TargetInfo struct.
		registry.MustRegisterForMultiModel("MigrationTarget", 5, func(stdCtx context.Context, ctx facade.MultiModelContext) (facade.Facade, error) {
			api, err := makeFacadeV5(stdCtx, ctx, requiredMigrationFacadeVersions)
			if err != nil {
				return nil, errors.Errorf("making migration target version 5: %w", err)
			}
			return api, nil
		}, reflect.TypeOf((*APIV5)(nil)))
		// Bumped to version 6 for the addition of the skipUserChecks field in
		// the Migration.TargetInfo struct.
		registry.MustRegisterForMultiModel("MigrationTarget", 6, func(stdCtx context.Context, ctx facade.MultiModelContext) (facade.Facade, error) {
			api, err := makeFacadeV6(stdCtx, ctx, requiredMigrationFacadeVersions)
			if err != nil {
				return nil, errors.Errorf("making migration target version 6: %w", err)
			}
			return api, nil
		}, reflect.TypeOf((*APIV6)(nil)))
		// v7 handles requests with a model qualifier instead of a model owner.
		registry.MustRegisterForMultiModel("MigrationTarget", 7, func(stdCtx context.Context, ctx facade.MultiModelContext) (facade.Facade, error) {
			api, err := makeFacade(stdCtx, ctx, requiredMigrationFacadeVersions)
			if err != nil {
				return nil, errors.Errorf("making migration target version 7: %w", err)
			}
			return api, nil
		}, reflect.TypeOf((*API)(nil)))
	}
}

func makeFacadeV4(
	stdCtx context.Context,
	ctx facade.MultiModelContext,
	facadeVersions facades.FacadeVersions,
) (*APIV4, error) {
	api, err := makeFacadeV5(stdCtx, ctx, facadeVersions)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return &APIV4{APIV5: api}, err
}

func makeFacadeV5(
	stdCtx context.Context,
	ctx facade.MultiModelContext,
	facadeVersions facades.FacadeVersions,
) (*APIV5, error) {
	api, err := makeFacadeV6(stdCtx, ctx, facadeVersions)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return &APIV5{APIV6: api}, err
}

func makeFacadeV6(
	stdCtx context.Context,
	ctx facade.MultiModelContext,
	facadeVersions facades.FacadeVersions,
) (*APIV6, error) {
	api, err := makeFacade(stdCtx, ctx, facadeVersions)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return &APIV6{API: api}, err
}

// makeFacade is responsible for constructing a new migration target facade and
// its dependencies.
func makeFacade(
	stdCtx context.Context,
	ctx facade.MultiModelContext,
	facadeVersions facades.FacadeVersions,
) (*API, error) {
	auth := ctx.Auth()
	st := ctx.State()
	if err := checkAuth(stdCtx, auth, st); err != nil {
		return nil, err
	}

	domainServices := ctx.DomainServices()

	modelMigrationServiceGetter := func(c context.Context, modelId model.UUID) (ModelMigrationService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelId)
		if err != nil {
			return nil, errors.Errorf("retrieving domain services for model %q: %w", modelId, err)
		}
		return svc.ModelMigration(), nil
	}
	modelAgentServiceGetter := func(c context.Context, modelId model.UUID) (ModelAgentService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelId)
		if err != nil {
			return nil, errors.Errorf("retrieving domain services for model %q: %w", modelId, err)
		}
		return svc.Agent(), nil
	}

	return NewAPI(
		ctx,
		auth,
		domainServices.ControllerConfig(),
		domainServices.ExternalController(),
		domainServices.Model(),
		domainServices.Upgrade(),
		domainServices.Status(),
		domainServices.Machine(),
		modelAgentServiceGetter,
		modelMigrationServiceGetter,
		facadeVersions,
		ctx.LogDir(),
	)
}
