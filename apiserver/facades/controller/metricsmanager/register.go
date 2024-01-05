// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsmanager

import (
	"context"
	"reflect"

	"github.com/juju/clock"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("MetricsManager", 1, func(stdCtx context.Context, ctx facade.Context) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*MetricsManagerAPI)(nil)))
}

// newFacade wraps NewMetricsManagerAPI for API registration.
func newFacade(ctx facade.Context) (*MetricsManagerAPI, error) {
	return NewMetricsManagerAPI(
		ctx.State(),
		ctx.Resources(),
		ctx.Auth(),
		ctx.StatePool(),
		clock.WallClock,
	)
}
