// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions

import (
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Registry describes the API facades exposed by some API server.
type Registry interface {
	// MustRegister adds a single named facade at a given version to the
	// registry.
	// Factory will be called when someone wants to instantiate an object of
	// this facade, and facadeType defines the concrete type that the returned
	// object will be.
	// The Type information is used to define what methods will be exported in
	// the API, and it must exactly match the actual object returned by the
	// factory.
	MustRegister(string, int, facade.Factory, reflect.Type)
}

// Register is called to expose a package of facades onto a given registry.
func Register(registry Registry) {
	registry.MustRegister("MachineActions", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newExternalFacade(ctx)
	}, reflect.TypeOf((*Facade)(nil)))
}

// newExternalFacade is used for API registration.
func newExternalFacade(ctx facade.Context) (*Facade, error) {
	return NewFacade(backendShim{ctx.State()}, ctx.Resources(), ctx.Auth())
}
