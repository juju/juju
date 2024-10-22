// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"context"
	"reflect"

	"github.com/juju/clock"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/domain/application/service"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CAASUnitProvisioner", 2, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newStateFacade(ctx)
	}, reflect.TypeOf((*Facade)(nil)))
}

// newStateFacade provides the signature required for facade registration.
func newStateFacade(ctx facade.ModelContext) (*Facade, error) {
	applicationService := ctx.DomainServices().Application(service.NotImplementedSecretService{})
	return NewFacade(
		ctx.WatcherRegistry(),
		ctx.Resources(),
		ctx.Auth(),
		ctx.DomainServices().Network(),
		applicationService,
		stateShim{ctx.State()},
		clock.WallClock,
		ctx.Logger().Child("caasunitprovisioner"),
	)
}
