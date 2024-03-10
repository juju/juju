// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner

import (
	"context"
	"reflect"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/internal/storage/provider"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Cleaner", 2, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newCleanerAPI(ctx)
	}, reflect.TypeOf((*CleanerAPI)(nil)))
}

// newCleanerAPI creates a new instance of the Cleaner API.
func newCleanerAPI(ctx facade.ModelContext) (*CleanerAPI, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}

	serviceFactory := ctx.ServiceFactory()
	// For removing applications, we don't need a storage registry.
	// TODO(storage) - implement a "no op registry"
	registry := provider.CommonStorageProviders()

	return &CleanerAPI{
		st:             getState(ctx.State()),
		resources:      ctx.Resources(),
		objectStore:    ctx.ObjectStore(),
		machineRemover: serviceFactory.Machine(),
		appRemover:     serviceFactory.Application(registry),
		unitRemover:    serviceFactory.Unit(),
	}, nil
}
