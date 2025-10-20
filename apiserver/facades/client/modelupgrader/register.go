// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	"context"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/docker/registry"
	"github.com/juju/juju/rpc/params"
)

// UpgraderAPI holds the common methods for upgrading agents in controllers and models.
// At the moment it is used to dynamically register the facade because the facade names
// are the same for both [ControllerUpgraderAPI] and [ModelUpgraderAPI].
// See [Register] func.
type UpgraderAPI interface {
	AbortModelUpgrade(ctx context.Context, arg params.ModelParam) error
	UpgradeModel(ctx context.Context, arg params.UpgradeModelParams) (result params.UpgradeModelResult, err error)
}

// UpgradeAPI represents the model upgrader facade. This type exist to sastify
// registration requirements of providing a singular type to must register.
// Behind this struct is a facade implementation that implements the
// [UpgradeAPI] interface.
type UpgradeAPI struct {
	UpgraderAPI
}

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegisterForMultiModel("ModelUpgrader", 1, func(stdCtx context.Context, ctx facade.MultiModelContext) (facade.Facade, error) {
		return newUpgraderFacadeV1(ctx)
	}, reflect.TypeOf(UpgradeAPI{}))
}

// newUpgraderFacadeV1 returns which facade to register.
// It will return a [ControllerUpgraderAPI] if the current model hosts the controller.
// Otherwise, it defaults to [ModelUpgraderAPI].
func newUpgraderFacadeV1(ctx facade.MultiModelContext) (UpgradeAPI, error) {
	auth := ctx.Auth()
	if !auth.AuthClient() {
		return UpgradeAPI{}, apiservererrors.ErrPerm
	}
	var (
		upgraderAPI UpgraderAPI
		err         error
	)

	if ctx.IsControllerModelScoped() {
		domainServices := ctx.DomainServices()
		upgraderAPI, err = NewControllerUpgraderAPI(
			names.NewControllerTag(ctx.ControllerUUID()),
			names.NewModelTag(ctx.ModelUUID().String()),
			auth,
			common.NewBlockChecker(domainServices.BlockCommand()),
			domainServices.ControllerUpgraderService(),
		), nil
	} else {
		upgraderAPI, err = newFacadeV1(ctx)
	}

	if err != nil {
		return UpgradeAPI{}, errors.Trace(err)
	}

	return UpgradeAPI{UpgraderAPI: upgraderAPI}, nil
}

// newFacadeV1 is used for API registration.
func newFacadeV1(ctx facade.MultiModelContext) (*ModelUpgraderAPI, error) {
	auth := ctx.Auth()

	// Since we know this is a user tag (because AuthClient is true),
	// we just do the type assertion to the UserTag.
	if !auth.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

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
		domainServices.Model(),
		domainServices.Upgrade(),
		ctx.Logger().Child("modelupgrader"),
	)
}
