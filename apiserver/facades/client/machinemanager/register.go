// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"context"
	"fmt"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/credentialcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/state/stateenvirons"
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
	serviceFactory := ctx.ServiceFactory()

	prechecker, err := stateenvirons.NewInstancePrechecker(st, serviceFactory.Cloud(), serviceFactory.Credential())
	if err != nil {
		return nil, errors.Trace(err)
	}

	backend := &stateShim{
		State:     st,
		prechcker: prechecker,
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

	model, err := ctx.ServiceFactory().ModelInfo().GetModelInfo(stdCtx)
	if err != nil {
		return nil, fmt.Errorf("getting model information for constructing machine manager facade: %w", err)
	}

	return NewMachineManagerAPI(
		model,
		serviceFactory.ControllerConfig(),
		backend,
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
		serviceFactory.Machine(),
		ctx.ObjectStore(),
		ctx.ControllerObjectStore(),
		storageAccess,
		pool,
		ModelAuthorizer{
			ModelTag:   names.NewModelTag(model.UUID.String()),
			Authorizer: ctx.Auth(),
		},
		credentialcommon.CredentialInvalidatorGetter(ctx),
		ctx.Resources(),
		leadership,
		logger,
		ctx.ServiceFactory().Network(),
		ctx.ServiceFactory().KeyUpdater(),
		ctx.ServiceFactory().Config(),
	), nil
}
