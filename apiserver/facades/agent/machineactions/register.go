// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("MachineActions", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newExternalFacade(ctx)
	}, reflect.TypeOf((*Facade)(nil)))
}

// newExternalFacade is used for API registration.
func newExternalFacade(ctx facade.ModelContext) (*Facade, error) {
	return NewFacade(backendShim{st: ctx.State()}, ctx.WatcherRegistry(), ctx.Auth())
}
