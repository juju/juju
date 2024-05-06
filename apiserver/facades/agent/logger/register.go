// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Logger", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newLoggerAPIV1(ctx)
	}, reflect.TypeOf((*LoggerAPI)(nil)))
}

// newLoggerAPIV1 creates a new server-side logger API end point.
func newLoggerAPIV1(ctx facade.ModelContext) (*LoggerAPI, error) {
	return NewLoggerAPI(ctx.Auth(),
		ctx.WatcherRegistry(),
		ctx.ServiceFactory().Config())
}
