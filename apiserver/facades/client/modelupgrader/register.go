// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/docker/registry"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegisterForMultiModel("ModelUpgrader", 1, func(stdCtx context.Context, ctx facade.MultiModelContext) (facade.Facade, error) {
		return newFacadeV1(ctx)
	}, reflect.TypeOf((*ModelUpgraderAPI)(nil)))
}

// newFacadeV1 is used for API registration.
func newFacadeV1(ctx facade.MultiModelContext) (*ModelUpgraderAPI, error) {
	auth := ctx.Auth()

	// Since we know this is a user tag (because AuthClient is true),
	// we just do the type assertion to the UserTag.
	if !auth.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	pool := ctx.StatePool()

	domainServices := ctx.DomainServices()
	controllerConfigService := domainServices.ControllerConfig()
	controllerAgentService := domainServices.Agent()

	urlGetter := common.NewToolsURLGetter(ctx.ModelUUID().String(), domainServices.ControllerNode())
	toolsFinder := common.NewToolsFinder(
		urlGetter,
		ctx.ControllerObjectStore(),
		domainServices.AgentBinary(),
	)

	modelAgentServiceGetter := func(c context.Context, modelUUID coremodel.UUID) (ModelAgentService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Agent(), nil
	}
	machineServiceGetter := func(c context.Context, modelUUID coremodel.UUID) (MachineService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Machine(), nil
	}

	return NewModelUpgraderAPI(
		ctx.ControllerUUID(),
		statePoolShim{StatePool: pool},
		toolsFinder,
		common.NewBlockChecker(domainServices.BlockCommand()),
		auth,
		registry.New,
		modelAgentServiceGetter,
		machineServiceGetter,
		controllerAgentService,
		controllerConfigService,
		domainServices.Agent(),
		domainServices.Machine(),
		domainServices.ModelInfo(),
		domainServices.Upgrade(),
		ctx.Logger().Child("modelupgrader"),
	)
}
