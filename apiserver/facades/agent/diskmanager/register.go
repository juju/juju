// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager

import (
	"context"
	"reflect"

	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("DiskManager", 2, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newDiskManagerAPI(ctx)
	}, reflect.TypeOf((*DiskManagerAPI)(nil)))
}

// newDiskManagerAPI creates a new server-side DiskManager API facade.
func newDiskManagerAPI(ctx facade.ModelContext) (*DiskManagerAPI, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthMachineAgent() {
		return nil, apiservererrors.ErrPerm
	}

	authEntityTag := authorizer.GetAuthTag()
	getAuthFunc := func(context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			// A machine agent can always access its own machine.
			return tag == authEntityTag
		}, nil
	}

	return &DiskManagerAPI{
		blockDeviceUpdater: ctx.DomainServices().BlockDevice(),
		authorizer:         authorizer,
		getAuthFunc:        getAuthFunc,
	}, nil
}
