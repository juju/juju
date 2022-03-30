// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater

import (
	"reflect"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("ExternalControllerUpdater", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newStateAPI(ctx)
	}, reflect.TypeOf((*ExternalControllerUpdaterAPI)(nil)))
}

// newStateAPI creates a new server-side CrossModelRelationsAPI API facade
// backed by global state.
func newStateAPI(ctx facade.Context) (*ExternalControllerUpdaterAPI, error) {
	return NewAPI(
		ctx.Auth(),
		ctx.Resources(),
		state.NewExternalControllers(ctx.State()),
	)
}
