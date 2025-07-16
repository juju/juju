// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("AgentLifeFlag", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*Facade)(nil)))
}

// newFacade is for API registration.
func newFacade(ctx facade.ModelContext) (*Facade, error) {
	domainServices := ctx.DomainServices()

	return NewFacade(
		domainServices.Application(),
		domainServices.Machine(),
		ctx.WatcherRegistry(),
		ctx.Auth(),
		ctx.Logger(),
	)
}
