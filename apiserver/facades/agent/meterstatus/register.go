// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus

import (
	"reflect"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/domain"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	controllerconfigstate "github.com/juju/juju/domain/controllerconfig/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("MeterStatus", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newMeterStatusFacade(ctx)
	}, reflect.TypeOf((*MeterStatusAPI)(nil)))
}

// newMeterStatusFacade provides the signature required for facade registration.
func newMeterStatusFacade(ctx facade.Context) (*MeterStatusAPI, error) {
	authorizer := ctx.Auth()
	resources := ctx.Resources()
	controllerConfigGetter := controllerconfigservice.NewService(
		controllerconfigstate.NewState(changestream.NewTxnRunnerFactory(ctx.ControllerDB)),
		domain.NewWatcherFactory(
			ctx.ControllerDB,
			ctx.Logger().Child("controllerconfig"),
		),
	)
	return NewMeterStatusAPI(controllerConfigGetter, ctx.State(), resources, authorizer, ctx.Logger().Child("meterstatus"))
}
