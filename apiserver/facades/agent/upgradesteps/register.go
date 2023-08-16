// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("UpgradeSteps", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV2(ctx)
	}, reflect.TypeOf((*UpgradeStepsAPI)(nil)))
}

// newFacadeV2 is used for API registration.
func newFacadeV2(ctx facade.Context) (*UpgradeStepsAPI, error) {
	return NewUpgradeStepsAPI(
		ctx.State(),
		ctx.ServiceFactory().ControllerConfig(),
		ctx.Resources(),
		ctx.Auth(),
		ctx.Logger().Child("upgradesteps"),
	)
}
