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

	serviceFactory := ctx.ServiceFactory()

	configGetter := stateenvirons.EnvironConfigGetter{
		Model:             model,
		CloudService:      serviceFactory.Cloud(),
		CredentialService: serviceFactory.Credential(),
	}
	newEnviron := common.EnvironFuncForModel(model, serviceFactory.Cloud(), serviceFactory.Credential(), configGetter)

	ctrlModel, err := ctlrSt.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	configSchemaSource := environs.ProviderConfigSchemaSource(serviceFactory.Cloud())

	controllerConfigService := serviceFactory.ControllerConfig()

	urlGetter := common.NewToolsURLGetter(modelUUID, systemState)
	toolsFinder := common.NewToolsFinder(controllerConfigService, st, urlGetter, newEnviron, ctx.ControllerObjectStore())

	apiUser, _ := auth.GetAuthTag().(names.UserTag)
	backend := common.NewUserAwareModelManagerBackend(configSchemaSource, model, pool, apiUser)

	secretBackendService := serviceFactory.SecretBackend()
	return NewModelManagerAPI(
		stdCtx,
		backend.(StateBackend),
		ctx.ModelExporter(backend),
		common.NewModelManagerBackend(configSchemaSource, ctrlModel, pool),
		controllerUUID,
		Services{
			ServiceFactoryGetter: serviceFactoryGetter{ctx: ctx},
			CloudService:         serviceFactory.Cloud(),
			CredentialService:    serviceFactory.Credential(),
			ModelService:         serviceFactory.Model(),
			ModelDefaultsService: serviceFactory.ModelDefaults(),
			AccessService:        serviceFactory.Access(),
			ObjectStore:          ctx.ObjectStore(),
			SecretBackendService: secretBackendService,
			NetworkService:       serviceFactory.Network(),
		},
		configSchemaSource,
		toolsFinder,
		caas.New,
		common.NewBlockChecker(backend),
		auth,
		model,
	)
}
