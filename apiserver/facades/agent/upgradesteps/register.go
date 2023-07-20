// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"reflect"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/domain"
	ccservice "github.com/juju/juju/domain/controllerconfig/service"
	ccstate "github.com/juju/juju/domain/controllerconfig/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("UpgradeSteps", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV2(ctx)
	}, reflect.TypeOf((*UpgradeStepsAPI)(nil)))
}

// newFacadeV2 is used for API registration.
func newFacadeV2(ctx facade.Context) (*UpgradeStepsAPI, error) {
	st := &upgradeStepsStateShim{State: ctx.State()}
	ctrlConfigService := ccservice.NewService(
		ccstate.NewState(changestream.NewTxnRunnerFactory(ctx.ControllerDB)),
		domain.NewWatcherFactory(
			ctx.ControllerDB,
			ctx.Logger().Child("controllerconfig"),
		),
	)
	return NewUpgradeStepsAPI(st, ctx.Resources(), ctx.Auth(), ctx.Logger().Child("upgradesteps"), ctrlConfigService)
}
