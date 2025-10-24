// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	"context"
	"reflect"

	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
)

// UpgraderAPI holds the common methods for upgrading agents in controllers and models.
// At the moment it is used to dynamically register the facade because the facade names
// are the same for both [ControllerUpgraderAPI] and [ModelUpgraderAPI].
// See [Register] func.
type UpgraderAPI interface {
	AbortModelUpgrade(ctx context.Context, arg params.ModelParam) error
	UpgradeModel(
		ctx context.Context,
		arg params.UpgradeModelParams,
	) (result params.UpgradeModelResult, err error)
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
	registry.MustRegisterForMultiModel("ModelUpgrader", 1, func(
		stdCtx context.Context,
		ctx facade.MultiModelContext,
	) (facade.Facade, error) {
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

	controllerTag := names.NewControllerTag(ctx.ControllerUUID())
	modelTag := names.NewModelTag(ctx.ModelUUID().String())
	domainServices := ctx.DomainServices()
	checker := common.NewBlockChecker(domainServices.BlockCommand())

	if ctx.IsControllerModelScoped() {
		upgraderAPI := NewControllerUpgraderAPI(
			controllerTag,
			modelTag,
			auth,
			checker,
			domainServices.ControllerUpgraderService(),
		)
		return UpgradeAPI{upgraderAPI}, nil
	}

	upgraderAPI := NewModelUpgraderAPI(
		controllerTag,
		modelTag,
		auth,
		checker,
		domainServices.Agent(),
	)
	return UpgradeAPI{UpgraderAPI: upgraderAPI}, nil
}
