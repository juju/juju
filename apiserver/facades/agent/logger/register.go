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
	}, reflect.TypeFor[*LoggerAPI]())
	registry.MustRegister("Logger", 2, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newLoggerAPIV2(ctx)
	}, reflect.TypeFor[*LoggerAPIV2]())
}

// newLoggerAPIV1 creates a new server-side logger API end point.
func newLoggerAPIV1(ctx facade.ModelContext) (*LoggerAPI, error) {
	return NewLoggerAPI(ctx.Auth(),
		ctx.WatcherRegistry(),
		ctx.DomainServices().Config())
}

// newLoggerAPIV2 creates a new server-side logger API end point.
func newLoggerAPIV2(ctx facade.ModelContext) (*LoggerAPIV2, error) {
	return NewLoggerAPIV2(ctx.Auth(),
		ctx.WatcherRegistry(),
		ctx.DomainServices().Config(),
		ctx.DomainServices().Logging())
}
