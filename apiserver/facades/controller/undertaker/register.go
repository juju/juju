// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Undertaker", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newUndertakerFacade(ctx)
	}, reflect.TypeOf((*UndertakerAPI)(nil)))
}

// newUndertakerFacade creates a new instance of the undertaker API.
func newUndertakerFacade(ctx facade.ModelContext) (*UndertakerAPI, error) {
	st := ctx.State()

	domainServices := ctx.DomainServices()
	modelInfoService := domainServices.ModelInfo()
	backendService := domainServices.SecretBackend()
	cloudSpecService := domainServices.ModelProvider()

	return newUndertakerAPI(
		ctx.ModelUUID(),
		&stateShim{st},
		ctx.Resources(),
		ctx.Auth(),
		cloudSpecService,
		backendService,
		domainServices.Config(),
		modelInfoService,
		ctx.WatcherRegistry(),
	)
}
