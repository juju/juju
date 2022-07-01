// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion

import (
	"reflect"

	"github.com/juju/juju/v3/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("MigrationMinion", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*API)(nil)))
}

// newFacade provides the signature required for facade registration.
func newFacade(ctx facade.Context) (*API, error) {
	return NewAPI(ctx.State(), ctx.Resources(), ctx.Auth())
}
