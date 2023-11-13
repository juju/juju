// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"reflect"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Logger", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newLoggerAPI(ctx)
	}, reflect.TypeOf((*LoggerAPI)(nil)))
}

// newLoggerAPI creates a new server-side logger API end point.
func newLoggerAPI(ctx facade.Context) (*LoggerAPI, error) {
	st := ctx.State()
	resources := ctx.Resources()
	authorizer := ctx.Auth()

	if !authorizer.AuthMachineAgent() &&
		!authorizer.AuthUnitAgent() &&
		!authorizer.AuthApplicationAgent() &&
		!authorizer.AuthModelAgent() {
		return nil, apiservererrors.ErrPerm
	}
	m, err := ctx.Controller().Model(st.ModelUUID())
	if err != nil {
		return nil, err
	}

	return &LoggerAPI{
		controller: ctx.Controller(),
		model:      m,
		resources:  resources,
		authorizer: authorizer,
	}, nil
}
