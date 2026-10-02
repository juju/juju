// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegisterForMultiModel("Backups", 3, func(stdCtx context.Context, ctx facade.MultiModelContext) (facade.Facade, error) {
		return newFacade(stdCtx, ctx)
	}, reflect.TypeFor[*API]())
}

// newFacade provides the required signature for facade registration.
func newFacade(stdCtx context.Context, ctx facade.MultiModelContext) (*API, error) {
	return NewAPI(
		ctx.Auth(),
		ctx.ControllerUUID(),
		ctx.Logger().Child("backups"),
	)
}
