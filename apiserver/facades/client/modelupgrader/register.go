// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	coremodel "github.com/juju/juju/core/model"
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

	domainServices := ctx.DomainServices()
	cloudService := domainServices.Cloud()
	credentialService := domainServices.Credential()
	modelConfigService := domainServices.Config()

	configGetter := stateenvirons.EnvironConfigGetter{
		Model:              model,
		CloudService:       cloudService,
		CredentialService:  credentialService,
		ModelConfigService: modelConfigService,
	}
	newEnviron := common.EnvironFuncForModel(model, cloudService, credentialService, configGetter)

	controllerConfigService := domainServices.ControllerConfig()
	controllerAgentService := domainServices.Agent()

	urlGetter := common.NewToolsURLGetter(modelUUID, systemState)
	toolsFinder := common.NewToolsFinder(controllerConfigService, st, urlGetter, newEnviron, ctx.ControllerObjectStore())

	modelAgentServiceGetter := func(modelID coremodel.UUID) ModelAgentService {
		return domainServices.Agent()
	}
	modelConfigServiceGetter := func(ctx context.Context, modelID coremodel.UUID) (cloudspec.ModelConfigService, error) {
		return domainServices.Config(), nil
	}
	environsCloudSpecGetter := cloudspec.MakeCloudSpecGetter(pool, cloudService, credentialService, modelConfigServiceGetter)

	return NewModelUpgraderAPI(
		systemState.ControllerTag(),
		statePoolShim{StatePool: pool},
		toolsFinder,
		newEnviron,
		common.NewBlockChecker(domainServices.BlockCommand()),
		auth,
		registry.New,
		environsCloudSpecGetter,
		modelAgentServiceGetter,
		controllerAgentService,
		controllerConfigService,
		domainServices.Upgrade(),
		ctx.Logger().Child("modelupgrader"),
	)
}
