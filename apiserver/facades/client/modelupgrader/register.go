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
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/errors"
)

// UpgradeAPI represents the model upgrader facade. This type exist to sastify
// registration requirements of providing a singular type to must register.
// Behind this struct is a facade implementation that implements the
// [UpgradeAPI] interface.
type UpgradeAPI struct {
	UpgraderAPI
}

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegisterForMultiModel("ModelUpgrader", 2, func(
		stdCtx context.Context,
		ctx facade.MultiModelContext,
	) (facade.Facade, error) {
		return newUpgraderFacadeV2(stdCtx, ctx)
	}, reflect.TypeFor[UpgradeAPI]())
}

// newUpgraderFacadeV2 returns a controller-context upgrader facade that routes
// each request to the target model's service backing.
func newUpgraderFacadeV2(
	stdCtx context.Context,
	ctx facade.MultiModelContext,
) (UpgradeAPI, error) {
	auth := ctx.Auth()
	if !auth.AuthClient() {
		return UpgradeAPI{}, apiservererrors.ErrPerm
	}

	controllerTag := names.NewControllerTag(ctx.ControllerUUID())
	controllerModelTag := names.NewModelTag(ctx.ControllerModelUUID().String())
	controllerDomainServices, err := ctx.DomainServicesForModel(
		stdCtx,
		ctx.ControllerModelUUID(),
	)
	if err != nil {
		return UpgradeAPI{}, errors.Capture(err)
	}
	controllerUpgrader := NewControllerUpgraderAPI(
		controllerTag,
		controllerModelTag,
		auth,
		common.NewBlockChecker(controllerDomainServices.BlockCommand()),
		controllerDomainServices.ControllerUpgrader(),
	)
	modelUpgrader := func(
		stdCtx context.Context,
		modelTag names.ModelTag,
	) (UpgraderAPI, error) {
		domainServices, err := ctx.DomainServicesForModel(
			stdCtx,
			coremodel.UUID(modelTag.Id()),
		)
		if err != nil {
			return nil, errors.Capture(err)
		}
		return newTargetModelUpgraderAPI(
			controllerTag,
			modelTag,
			auth,
			common.NewBlockChecker(domainServices.BlockCommand()),
			domainServices.Agent(),
		), nil
	}

	upgraderAPI := NewModelUpgraderAPI(
		controllerModelTag,
		controllerUpgrader,
		modelUpgrader,
	)
	return UpgradeAPI{UpgraderAPI: upgraderAPI}, nil
}
