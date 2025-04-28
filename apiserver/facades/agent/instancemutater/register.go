// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"context"
	"reflect"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("InstanceMutater", 3, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacadeV3(ctx)
	}, reflect.TypeOf((*InstanceMutaterAPI)(nil)))
}

// newFacadeV3 is used for API registration.
func newFacadeV3(ctx facade.ModelContext) (*InstanceMutaterAPI, error) {
	if !ctx.Auth().AuthMachineAgent() && !ctx.Auth().AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	st := &instanceMutaterStateShim{State: ctx.State()}

	machineService := ctx.DomainServices().Machine()
	applicationService := ctx.DomainServices().Application()
	modelInfoService := ctx.DomainServices().ModelInfo()
	watcher := &instanceMutatorWatcher{
		st:                 st,
		machineService:     machineService,
		applicationService: applicationService,
	}
	return NewInstanceMutaterAPI(
		st,
		machineService,
		applicationService,
		modelInfoService,
		watcher,
		ctx.Resources(),
		ctx.Auth(),
		ctx.Logger().Child("instancemutater"),
	), nil
}
