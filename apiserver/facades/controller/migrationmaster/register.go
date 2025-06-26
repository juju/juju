// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/migration"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegisterForMultiModel("MigrationMaster", 4, func(stdCtx context.Context, ctx facade.MultiModelContext) (facade.Facade, error) {
		return newMigrationMasterFacadeV4(stdCtx, ctx)
	}, reflect.TypeOf((*APIV4)(nil)))
	// v5 handles requests with a model qualifier instead of a model owner.
	registry.MustRegisterForMultiModel("MigrationMaster", 5, func(stdCtx context.Context, ctx facade.MultiModelContext) (facade.Facade, error) {
		return newMigrationMasterFacade(stdCtx, ctx)
	}, reflect.TypeOf((*API)(nil)))
}

func newMigrationMasterFacadeV4(stdCtx context.Context, ctx facade.MultiModelContext) (*APIV4, error) {
	api, err := newMigrationMasterFacade(stdCtx, ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIV4{
		API: api,
	}, nil
}

// newMigrationMasterFacade exists to provide the required signature for API
// registration, converting st to backend.
func newMigrationMasterFacade(stdCtx context.Context, ctx facade.MultiModelContext) (*API, error) {
	pool := ctx.StatePool()
	modelState := ctx.State()

	controllerState, err := pool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}

	preCheckBackend, err := migration.PrecheckShim(modelState, controllerState)
	if err != nil {
		return nil, errors.Annotate(err, "creating precheck backend")
	}

	leadership, err := ctx.LeadershipReader()
	if err != nil {
		return nil, errors.Trace(err)
	}

	backend := newBacked(modelState)

	domainServices := ctx.DomainServices()
	credentialServiceGetter := func(stdctx context.Context, modelUUID model.UUID) (CredentialService, error) {
		domainServices, err := ctx.DomainServicesForModel(stdctx, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return domainServices.Credential(), nil
	}
	upgradeServiceGetter := func(stdctx context.Context, modelUUID model.UUID) (UpgradeService, error) {
		domainServices, err := ctx.DomainServicesForModel(stdctx, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return domainServices.Upgrade(), nil
	}
	applicationServiceGetter := func(stdctx context.Context, modelUUID model.UUID) (ApplicationService, error) {
		domainServices, err := ctx.DomainServicesForModel(stdctx, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return domainServices.Application(), nil
	}
	relationServiceGetter := func(stdctx context.Context, modelUUID model.UUID) (RelationService, error) {
		domainServices, err := ctx.DomainServicesForModel(stdctx, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return domainServices.Relation(), nil
	}
	statusServiceGetter := func(stdctx context.Context, modelUUID model.UUID) (StatusService, error) {
		domainServices, err := ctx.DomainServicesForModel(stdctx, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return domainServices.Status(), nil
	}
	modelAgentServiceGetter := func(stdctx context.Context, modelUUID model.UUID) (ModelAgentService, error) {
		domainServices, err := ctx.DomainServicesForModel(stdctx, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return domainServices.Agent(), nil
	}

	modelExporter, err := ctx.ModelExporter(stdCtx, ctx.ModelUUID(), backend)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return NewAPI(
		controllerState,
		backend,
		modelExporter,
		ctx.ObjectStore(),
		ctx.ControllerModelUUID(),
		preCheckBackend,
		migration.PoolShim(pool),
		ctx.Resources(),
		ctx.Auth(),
		leadership,
		credentialServiceGetter,
		upgradeServiceGetter,
		applicationServiceGetter,
		relationServiceGetter,
		statusServiceGetter,
		modelAgentServiceGetter,
		domainServices.ControllerConfig(),
		domainServices.ModelInfo(),
		domainServices.Model(),
	)
}
