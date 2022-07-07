// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Controller", 11, func(ctx facade.Context) (facade.Facade, error) {
		return newControllerAPIv11(ctx)
	}, reflect.TypeOf((*ControllerAPI)(nil)))
}

// newControllerAPIv11 creates a new ControllerAPIv11
func newControllerAPIv11(ctx facade.Context) (*ControllerAPI, error) {
	st := ctx.State()
	authorizer := ctx.Auth()
	pool := ctx.StatePool()
	resources := ctx.Resources()
	presence := ctx.Presence()
	hub := ctx.Hub()
	factory := ctx.MultiwatcherFactory()
	controller := ctx.Controller()

	return NewControllerAPI(
		st,
		pool,
		authorizer,
		resources,
		presence,
		hub,
		factory,
		controller,
	)
}
