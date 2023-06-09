// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsmanager

import (
	"reflect"

	"github.com/juju/clock"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/domain"
	ccservice "github.com/juju/juju/domain/controllerconfig/service"
	ccstate "github.com/juju/juju/domain/controllerconfig/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("MetricsManager", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*MetricsManagerAPI)(nil)))
}

// newFacade wraps NewMetricsManagerAPI for API registration.
func newFacade(ctx facade.Context) (*MetricsManagerAPI, error) {
	ctrlConfigService := ccservice.NewService(
		ccstate.NewState(domain.NewTxnRunnerFactory(ctx.ControllerDB)),
		domain.NewWatcherFactory(
			ctx.ControllerDB,
			ctx.Logger().Child("controllerconfig"),
		),
	)
	return NewMetricsManagerAPI(
		ctx.State(),
		ctx.Resources(),
		ctx.Auth(),
		ctx.StatePool(),
		clock.WallClock,
		ctrlConfigService,
	)
}
