// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/domain"
	ccservice "github.com/juju/juju/domain/controllerconfig/service"
	ccstate "github.com/juju/juju/domain/controllerconfig/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CAASModelOperator", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newAPIFromContext(ctx)
	}, reflect.TypeOf((*API)(nil)))
}

// newAPIFromContext creates a new controller model facade from the supplied
// context.
func newAPIFromContext(ctx facade.Context) (*API, error) {
	authorizer := ctx.Auth()
	resources := ctx.Resources()
	systemState, err := ctx.StatePool().SystemState()
	ctrlConfigService := ccservice.NewService(
		ccstate.NewState(changestream.NewTxnRunnerFactory(ctx.ControllerDB)),
		domain.NewWatcherFactory(
			ctx.ControllerDB,
			ctx.Logger().Child("controllerconfig"),
		),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewAPI(authorizer, resources,
		stateShim{systemState},
		stateShim{ctx.State()},
		ctx.Logger().Child("caasmodeloperator"),
		ctrlConfigService,
	)
}
