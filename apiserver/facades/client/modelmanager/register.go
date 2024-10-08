// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"context"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/state/stateenvirons"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegisterForMultiModel("ModelManager", 10, func(stdCtx context.Context, ctx facade.MultiModelContext) (facade.Facade, error) {
		return newFacadeV10(stdCtx, ctx)
	}, reflect.TypeOf((*ModelManagerAPI)(nil)))
}

// newFacadeV10 is used for API registration.
func newFacadeV10(stdCtx context.Context, ctx facade.MultiModelContext) (*ModelManagerAPI, error) {
	st := ctx.State()
	pool := ctx.StatePool()
	ctlrSt, err := pool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	auth := ctx.Auth()

	// Since we know this is a user tag (because AuthClient is true),
	// we just do the type assertion to the UserTag.
	if !auth.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	controllerUUID, err := uuid.UUIDFromString(ctx.ControllerUUID())
	if err != nil {
		return nil, errors.Trace(err)
	}

	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelUUID := model.UUID()

	systemState, err := ctx.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}

	domainServices := ctx.DomainServices()

	configGetter := stateenvirons.EnvironConfigGetter{
		Model:              model,
		CloudService:       domainServices.Cloud(),
		CredentialService:  domainServices.Credential(),
		ModelConfigService: domainServices.Config(),
	}
	newEnviron := common.EnvironFuncForModel(model, domainServices.Cloud(), domainServices.Credential(), configGetter)

	ctrlModel, err := ctlrSt.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	configSchemaSource := environs.ProviderConfigSchemaSource(domainServices.Cloud())

	controllerConfigService := domainServices.ControllerConfig()

	urlGetter := common.NewToolsURLGetter(modelUUID, systemState)
	toolsFinder := common.NewToolsFinder(controllerConfigService, st, urlGetter, newEnviron, ctx.ControllerObjectStore())

	apiUser, _ := auth.GetAuthTag().(names.UserTag)
	backend := common.NewUserAwareModelManagerBackend(configSchemaSource, model, pool, apiUser)

	secretBackendService := domainServices.SecretBackend()
	return NewModelManagerAPI(
		stdCtx,
		backend.(StateBackend),
		func(modelUUID coremodel.UUID, legacyState facade.LegacyStateExporter) ModelExporter {
			return ctx.ModelExporter(modelUUID, legacyState)
		},
		common.NewModelManagerBackend(configSchemaSource, ctrlModel, pool),
		controllerUUID,
		Services{
			DomainServicesGetter: domainServicesGetter{ctx: ctx},
			CloudService:         domainServices.Cloud(),
			CredentialService:    domainServices.Credential(),
			ModelService:         domainServices.Model(),
			ModelDefaultsService: domainServices.ModelDefaults(),
			AccessService:        domainServices.Access(),
			ObjectStore:          ctx.ObjectStore(),
			SecretBackendService: secretBackendService,
			NetworkService:       domainServices.Network(),
			MachineService:       domainServices.Machine(),
			ApplicationService:   domainServices.Application(service.ApplicationServiceParams{}),
		},
		configSchemaSource,
		toolsFinder,
		caas.New,
		common.NewBlockChecker(backend),
		auth,
		model,
	)
}
