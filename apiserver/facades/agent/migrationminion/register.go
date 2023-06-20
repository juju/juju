// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion

import (
	"reflect"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("MigrationMinion", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*API)(nil)))
}

// newFacade provides the signature required for facade registration.
func newFacade(ctx facade.Context) (*API, error) {
	authorizer := ctx.Auth()
	if !(authorizer.AuthMachineAgent() || authorizer.AuthUnitAgent() || authorizer.AuthApplicationAgent()) {
		return nil, apiservererrors.ErrPerm
	}
	return NewAPI(ctx.State(), ctx.WatcherRegistry(), authorizer)
}
