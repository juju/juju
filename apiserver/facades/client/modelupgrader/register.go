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
	"github.com/juju/juju/internal/docker/registry"
	"github.com/juju/juju/state/stateenvirons"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("ModelUpgrader", 1, func(stdCtx context.Context, ctx facade.Context) (facade.Facade, error) {
		return newFacadeV1(ctx)
	}, reflect.TypeOf((*ModelUpgraderAPI)(nil)))
}

// newFacadeV1 is used for API registration.
func newFacadeV1(ctx facade.Context) (*ModelUpgraderAPI, error) {
	st := ctx.State()
	pool := ctx.StatePool()
	auth := ctx.Auth()

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
		Model: model, CloudService: ctx.ServiceFactory().Cloud(), CredentialService: serviceFactory.Credential()}
	cloudService := serviceFactory.Cloud()
	credentialService := serviceFactory.Credential()
	newEnviron := common.EnvironFuncForModel(model, cloudService, credentialService, configGetter)

	controllerConfigGetter := serviceFactory.ControllerConfig()

	urlGetter := common.NewToolsURLGetter(modelUUID, systemState)
	toolsFinder := common.NewToolsFinder(controllerConfigGetter, configGetter, st, urlGetter, newEnviron, ctx.ControllerObjectStore())
	environscloudspecGetter := cloudspec.MakeCloudSpecGetter(pool, cloudService, credentialService)

	// Since we know this is a user tag (because AuthClient is true),
	// we just do the type assertion to the UserTag.
	if !auth.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	apiUser, _ := auth.GetAuthTag().(names.UserTag)
	backend := common.NewUserAwareModelManagerBackend(model, pool, apiUser)
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
		serviceFactory.Upgrade(),
		ctx.Logger().Child("modelupgrader"),
	)
}
