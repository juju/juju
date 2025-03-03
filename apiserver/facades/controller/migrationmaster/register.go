// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common/cloudspec"
	"github.com/juju/juju/apiserver/facade"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/migration"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("MigrationMaster", 4, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newMigrationMasterFacade(stdCtx, ctx)
	}, reflect.TypeOf((*API)(nil)))
}

// newMigrationMasterFacade exists to provide the required signature for API
// registration, converting st to backend.
func newMigrationMasterFacade(stdCtx context.Context, ctx facade.ModelContext) (*API, error) {
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
	credentialService := domainServices.Credential()

	modelConfigServiceGetter := func(ctx context.Context, modelID coremodel.UUID) (cloudspec.ModelConfigService, error) {
		return domainServices.Config(), nil
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
		preCheckBackend,
		migration.PoolShim(pool),
		ctx.Resources(),
		ctx.Auth(),
		cloudspec.MakeCloudSpecGetter(pool, domainServices.Cloud(), credentialService, modelConfigServiceGetter),
		leadership,
		credentialService,
		domainServices.ControllerConfig(),
		domainServices.Config(),
		domainServices.ModelInfo(),
		domainServices.Model(),
		domainServices.Application(),
		domainServices.Upgrade(),
		domainServices.Agent(),
	)
}
