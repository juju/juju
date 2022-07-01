// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineundertaker

import (
	"reflect"

	"github.com/juju/juju/v2/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("MachineUndertaker", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*API)(nil)))
}

// newFacade provides the signature required for facade registration.
func newFacade(ctx facade.Context) (*API, error) {
	return NewAPI(&backendShim{ctx.State()}, ctx.Resources(), ctx.Auth())
}
