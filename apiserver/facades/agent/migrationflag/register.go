// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag

import (
	"reflect"

	"github.com/juju/errors"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("MigrationFlag", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*Facade)(nil)))
}

// newFacade wraps New to express the supplied *state.State as a Backend.
func newFacade(ctx facade.Context) (*Facade, error) {
	auth := ctx.Auth()
	if !auth.AuthMachineAgent() && !auth.AuthUnitAgent() && !auth.AuthApplicationAgent() {
		return nil, apiservererrors.ErrPerm
	}

	facade, err := New(&backend{ctx.State()}, ctx.WatcherRegistry(), auth)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return facade, nil
}
