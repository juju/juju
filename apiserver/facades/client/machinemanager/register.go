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
	internalerrors "github.com/juju/juju/internal/errors"
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
	pool := &poolShim{ctx.StatePool()}

	var leadership Leadership
	leadership, err := common.NewLeadershipPinningFromContext(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	logger := ctx.Logger().Child("machinemanager")

	modelType, err := ctx.DomainServices().ModelInfo().GetModelType(stdCtx)
	if err != nil {
		return nil, internalerrors.Errorf(
			"getting model type for constructing machine manager facade: %w",
			err,
		)
	}

	storageAccess, err := getStorageState(st, modelType)
	if err != nil {
		return nil, errors.Trace(err)
	}

	services := Services{
		AgentBinaryService:      domainServices.AgentBinary(),
		AgentPasswordService:    domainServices.AgentPassword(),
		ApplicationService:      domainServices.Application(),
		BlockCommandService:     domainServices.BlockCommand(),
		ControllerConfigService: domainServices.ControllerConfig(),
		ControllerNodeService:   domainServices.ControllerNode(),
		CloudService:            domainServices.Cloud(),
		KeyUpdaterService:       domainServices.KeyUpdater(),
		MachineService:          domainServices.Machine(),
		ModelConfigService:      domainServices.Config(),
		NetworkService:          domainServices.Network(),
	}

	return NewMachineManagerAPI(
		ctx.ModelUUID(),
		backend,
		ctx.ObjectStore(),
		ctx.ControllerObjectStore(),
		storageAccess,
		pool,
		ModelAuthorizer{
			ModelTag:   names.NewModelTag(ctx.ModelUUID().String()),
			Authorizer: ctx.Auth(),
		},
		ctx.Resources(),
		leadership,
		logger,
		services,
	), nil
}
