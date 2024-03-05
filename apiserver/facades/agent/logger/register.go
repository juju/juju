// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Logger", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newLoggerAPI(ctx)
	}, reflect.TypeOf((*LoggerAPI)(nil)))
}

// newLoggerAPI creates a new server-side logger API end point.
func newLoggerAPI(ctx facade.ModelContext) (*LoggerAPI, error) {
	model, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	resources := ctx.Resources()
	authorizer := ctx.Auth()

	if !authorizer.AuthMachineAgent() &&
		!authorizer.AuthUnitAgent() &&
		!authorizer.AuthApplicationAgent() &&
		!authorizer.AuthModelAgent() {
		return nil, apiservererrors.ErrPerm
	}

	return &LoggerAPI{
		model:      model,
		resources:  resources,
		authorizer: authorizer,
	}, nil
}
