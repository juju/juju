// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CAASUnitProvisioner", 2, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newStateFacade(ctx)
	}, reflect.TypeOf((*Facade)(nil)))
}

// newStateFacade provides the signature required for facade registration.
func newStateFacade(ctx facade.ModelContext) (*Facade, error) {
	applicationService := ctx.DomainServices().Application()
	return NewFacade(
		ctx.WatcherRegistry(),
		ctx.Resources(),
		ctx.Auth(),
		applicationService,
		stateShim{ctx.State()},
		ctx.Clock(),
		ctx.Logger().Child("caasunitprovisioner"),
	)
}
