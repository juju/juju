// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyupdater

import (
	"context"
	"fmt"
	"reflect"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("KeyUpdater", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		api, err := makeKeyUpdaterAPI(ctx)
		if err != nil {
			return nil, fmt.Errorf("making KeyUpdater api: %w", err)
		}
		return api, nil
	}, reflect.TypeOf((*KeyUpdaterAPI)(nil)))
}

// makeKeyUpdaterAPI is responsible for making a new KeyUpdaterAPI from a model
// context.
func makeKeyUpdaterAPI(
	ctx facade.ModelContext,
) (*KeyUpdaterAPI, error) {
	authorizer := ctx.Auth()

	// Only machine agents have access to the keyupdater service.
	if !authorizer.AuthMachineAgent() {
		return nil, apiservererrors.ErrPerm
	}
	// No-one else except the machine itself can only read a machine's own credentials.
	getCanRead := func(context.Context) (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	return newKeyUpdaterAPI(
		getCanRead,
		ctx.DomainServices().KeyUpdater(),
		ctx.WatcherRegistry(),
	), nil
}
