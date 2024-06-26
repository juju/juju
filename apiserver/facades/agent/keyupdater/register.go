// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyupdater

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("KeyUpdater", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newKeyUpdaterAPI(
			ctx.Auth(),
			ctx.ServiceFactory().KeyUpdater(),
			ctx.WatcherRegistry(),
		)
	}, reflect.TypeOf((*KeyUpdaterAPI)(nil)))
}

func newKeyUpdaterAPI(
	authorizer facade.Authorizer,
	keyUpdaterService KeyUpdaterService,
	watcherRegistery facade.WatcherRegistry,
) (*KeyUpdaterAPI, error) {
	// Only machine agents have access to the keyupdater service.
	if !authorizer.AuthMachineAgent() {
		return nil, apiservererrors.ErrPerm
	}
	// No-one else except the machine itself can only read a machine's own credentials.
	getCanRead := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	return &KeyUpdaterAPI{
		getCanRead:        getCanRead,
		keyUpdaterService: keyUpdaterService,
		watcherRegistery:  watcherRegistery,
	}, nil
}
