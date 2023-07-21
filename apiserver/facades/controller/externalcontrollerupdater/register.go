// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater

import (
	"reflect"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("ExternalControllerUpdater", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newStateAPI(ctx)
	}, reflect.TypeOf((*ExternalControllerUpdaterAPI)(nil)))
}

// newStateAPI creates a new server-side ExternalControllerUpdaterAPI API facade
// backed by global state.
func newStateAPI(ctx facade.Context) (*ExternalControllerUpdaterAPI, error) {
	if !ctx.Auth().AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	return NewAPI(
		ctx.Resources(),
		ctx.Services().ExternalController(),
	)
}
