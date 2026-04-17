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
	"github.com/juju/juju/internal/errors"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("DiskManager", 2, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newDiskManagerAPIV2(ctx)
	}, reflect.TypeFor[*DiskManagerAPIV2]())
	registry.MustRegister("DiskManager", 3, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newDiskManagerAPI(ctx)
	}, reflect.TypeFor[*DiskManagerAPI]())
}

// newDiskManagerAPIV2 creates a new server-side DiskManager API V2 facade.
func newDiskManagerAPIV2(ctx facade.ModelContext) (*DiskManagerAPIV2, error) {
	dm, err := newDiskManagerAPI(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return &DiskManagerAPIV2{
		DiskManagerAPI: dm,
	}, nil
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
		machineService:     ctx.DomainServices().Machine(),
		blockDeviceService: ctx.DomainServices().BlockDevice(),
		authorizer:         authorizer,
		getAuthFunc:        getAuthFunc,
	}, nil
}
