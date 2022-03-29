// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions

import (
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("MachineActions", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newExternalFacade(ctx)
	}, reflect.TypeOf((*Facade)(nil)))
}

// newExternalFacade is used for API registration.
func newExternalFacade(ctx facade.Context) (*Facade, error) {
	return NewFacade(backendShim{ctx.State()}, ctx.Resources(), ctx.Auth())
}
