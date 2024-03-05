// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Reboot", 2, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*RebootAPI)(nil)))
}

// newFacade creates a new server-side RebootAPI facade.
func newFacade(ctx facade.ModelContext) (*RebootAPI, error) {
	return NewRebootAPI(ctx)
}
