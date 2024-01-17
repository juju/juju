// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Controller", 11, func(stdCtx context.Context, ctx facade.Context) (facade.Facade, error) {
		return newControllerAPIv11(stdCtx, ctx)
	}, reflect.TypeOf((*ControllerAPI)(nil)))
}

// newControllerAPIv11 creates a new ControllerAPIv11
func newControllerAPIv11(stdCtx context.Context, ctx facade.Context) (*ControllerAPI, error) {
	var (
		st             = ctx.State()
		authorizer     = ctx.Auth()
		pool           = ctx.StatePool()
		resources      = ctx.Resources()
		presence       = ctx.Presence()
		hub            = ctx.Hub()
		serviceFactory = ctx.ServiceFactory()
	)

	return NewControllerAPI(
		stdCtx,
		st,
		pool,
		authorizer,
		resources,
		presence,
		hub,
		ctx.Logger().Child("controller"),
		serviceFactory.ControllerConfig(),
		serviceFactory.ExternalController(),
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
		serviceFactory.Upgrade(),
	)
}
