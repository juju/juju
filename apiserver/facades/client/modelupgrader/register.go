// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	"context"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	"github.com/juju/juju/apiserver/common/credentialcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/docker/registry"
	"github.com/juju/juju/state/stateenvirons"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("ModelUpgrader", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacadeV1(ctx)
	}, reflect.TypeOf((*ModelUpgraderAPI)(nil)))
}

// newFacadeV1 is used for API registration.
func newFacadeV1(ctx facade.ModelContext) (*ModelUpgraderAPI, error) {
	auth := ctx.Auth()

	// Since we know this is a user tag (because AuthClient is true),
	// we just do the type assertion to the UserTag.
	if !auth.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	st := ctx.State()
	pool := ctx.StatePool()
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
	cloudService := serviceFactory.Cloud()
	credentialService := serviceFactory.Credential()

	configGetter := stateenvirons.EnvironConfigGetter{
		Model:             model,
		CloudService:      cloudService,
		CredentialService: credentialService,
	}
	newEnviron := common.EnvironFuncForModel(model, cloudService, credentialService, configGetter)

	controllerConfigService := serviceFactory.ControllerConfig()

	urlGetter := common.NewToolsURLGetter(modelUUID, systemState)
	toolsFinder := common.NewToolsFinder(controllerConfigService, configGetter, st, urlGetter, newEnviron, ctx.ControllerObjectStore())
	environscloudspecGetter := cloudspec.MakeCloudSpecGetter(pool, cloudService, credentialService)

	configSchemaSource := environs.ProviderConfigSchemaSource(cloudService)

	apiUser, _ := auth.GetAuthTag().(names.UserTag)
	backend := common.NewUserAwareModelManagerBackend(configSchemaSource, model, pool, apiUser)
	return NewModelUpgraderAPI(
		systemState.ControllerTag(),
		statePoolShim{StatePool: pool},
		toolsFinder,
		newEnviron,
		common.NewBlockChecker(backend),
		auth,
		credentialcommon.CredentialInvalidatorGetter(ctx),
		registry.New,
		environscloudspecGetter,
		controllerConfigService,
		serviceFactory.Upgrade(),
		ctx.Logger().Child("modelupgrader"),
	)
}
