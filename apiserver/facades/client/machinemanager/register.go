// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/state/stateenvirons"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("MachineManager", 11, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return NewFacadeV11(ctx)
	}, reflect.TypeOf((*MachineManagerAPI)(nil)))
}

// MakeMachineManagerFacadeV11 create a new server-side MachineManager API
// facade. This is used for facade registration.
func NewFacadeV11(ctx facade.ModelContext) (*MachineManagerAPI, error) {
	st := ctx.State()
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
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

	controllerConfigService := serviceFactory.ControllerConfig()

	return NewMachineManagerAPI(
		controllerConfigService,
		backend,
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
		serviceFactory.Machine(),
		ctx.ObjectStore(),
		ctx.ControllerObjectStore(),
		storageAccess,
		pool,
		ModelAuthorizer{
			ModelTag:   model.ModelTag(),
			Authorizer: ctx.Auth(),
		},
		credentialcommon.CredentialInvalidatorGetter(ctx),
		ctx.Resources(),
		leadership,
		logger,
		ctx.ServiceFactory().Network(),
		ctx.ServiceFactory().KeyUpdater(),
	)
}
