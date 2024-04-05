// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("UpgradeSteps", 3, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacadeV3(ctx)
	}, reflect.TypeOf((*UpgradeStepsAPI)(nil)))
	registry.MustRegister("UpgradeSteps", 2, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacadeV2(ctx)
	}, reflect.TypeOf((*UpgradeStepsAPIV2)(nil)))
}

// newFacadeV2 is used for API registration.
func newFacadeV2(ctx facade.ModelContext) (*UpgradeStepsAPIV2, error) {
	api, err := newFacadeV3(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &UpgradeStepsAPIV2{UpgradeStepsAPI: api}, nil
}

// newFacadeV3 is used for API registration.
func newFacadeV3(ctx facade.ModelContext) (*UpgradeStepsAPI, error) {
	return NewUpgradeStepsAPI(
		ctx.State(),
		ctx.ServiceFactory().ControllerConfig(),
		ctx.Resources(),
		ctx.Auth(),
		ctx.Logger().Child("upgradesteps"),
	)
}
