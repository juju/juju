// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("LifeFlag", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newExternalFacade(ctx)
	}, reflect.TypeOf((*Facade)(nil)))
}

// newExternalFacade is for API registration.
func newExternalFacade(ctx facade.ModelContext) (*Facade, error) {
	domainServices := ctx.DomainServices()

	return NewFacade(
		ctx.ModelUUID(),
		domainServices.Application(),
		domainServices.Machine(),
		ctx.State(),
		ctx.WatcherRegistry(),
		ctx.Auth(),
		ctx.Logger().Child("lifeflag"),
	)
}
