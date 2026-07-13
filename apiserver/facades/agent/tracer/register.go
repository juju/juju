// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tracer

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Tracer", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newTracerAPI(ctx)
	}, reflect.TypeFor[*TracerAPI]())
}

// newTracerAPI creates a new server-side tracer API endpoint.
func newTracerAPI(ctx facade.ModelContext) (*TracerAPI, error) {
	return NewTracerAPI(ctx.Auth(),
		ctx.WatcherRegistry(),
		ctx.DomainServices().Tracing(),
	)
}
