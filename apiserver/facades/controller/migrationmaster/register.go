// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	"github.com/juju/juju/apiserver/facade"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/internal/migration"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("MigrationMaster", 3, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newMigrationMasterFacade(ctx) // Adds MinionReportTimeout.
	}, reflect.TypeOf((*API)(nil)))
}

// newMigrationMasterFacade exists to provide the required signature for API
// registration, converting st to backend.
func newMigrationMasterFacade(ctx facade.ModelContext) (*API, error) {
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

	modelConfigServiceGetter := func(modelID coremodel.UUID) common.ModelConfigService {
		return domainServices.Config()
	}

	return NewAPI(
		controllerState,
		backend,
		ctx.ModelExporter(ctx.ModelUUID(), backend),
		ctx.ObjectStore(),
		preCheckBackend,
		migration.PoolShim(pool),
		ctx.Resources(),
		ctx.Auth(),
		ctx.Presence(),
		cloudspec.MakeCloudSpecGetter(pool, domainServices.Cloud(), credentialService, modelConfigServiceGetter),
		leadership,
		credentialService,
		domainServices.ControllerConfig(),
		domainServices.Config(),
		domainServices.ModelInfo(),
		domainServices.Model(),
		domainServices.Application(service.NotImplementedSecretService{}),
		domainServices.Upgrade(),
		domainServices.Agent(),
	)
}
