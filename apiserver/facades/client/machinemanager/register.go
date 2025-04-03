// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"context"
	"fmt"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("MachineManager", 11, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		api, err := makeFacadeV11(stdCtx, ctx)
		if err != nil {
			return nil, fmt.Errorf("cannot register machine manager facade: %w", err)
		}
		return api, nil
	}, reflect.TypeOf((*MachineManagerAPI)(nil)))
}

// makeMachineManagerFacadeV11 create a new server-side MachineManager API
// facade. This is used for facade registration.
func makeFacadeV11(stdCtx context.Context, ctx facade.ModelContext) (*MachineManagerAPI, error) {
	// Check the the user is authenticated for this API before creating.
	if !ctx.Auth().AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	st := ctx.State()
	domainServices := ctx.DomainServices()

	backend := &stateShim{
		State: st,
	}
	storageAccess, err := getStorageState(st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	pool := &poolShim{ctx.StatePool()}

	var leadership Leadership
	leadership, err = common.NewLeadershipPinningFromContext(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	logger := ctx.Logger().Child("machinemanager")

	model, err := ctx.DomainServices().ModelInfo().GetModelInfo(stdCtx)
	if err != nil {
		return nil, fmt.Errorf("getting model information for constructing machine manager facade: %w", err)
	}

	return NewMachineManagerAPI(
		model,
		domainServices.ControllerConfig(),
		domainServices.Agent(),
		backend,
		domainServices.Cloud(),
		domainServices.Machine(),
		ctx.ObjectStore(),
		ctx.ControllerObjectStore(),
		storageAccess,
		pool,
		ModelAuthorizer{
			ModelTag:   names.NewModelTag(model.UUID.String()),
			Authorizer: ctx.Auth(),
		},
		ctx.Resources(),
		leadership,
		logger,
		domainServices.Network(),
		domainServices.KeyUpdater(),
		domainServices.Config(),
		domainServices.BlockCommand(),
	), nil
}
