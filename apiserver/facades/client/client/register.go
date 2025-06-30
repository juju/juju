// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"context"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Client", 8, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacadeV8(ctx)
	}, reflect.TypeOf((*Client)(nil)))
}

// newFacadeV8 returns a new Client facade (v8).
func newFacadeV8(ctx facade.ModelContext) (*Client, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	leadershipReader, err := ctx.LeadershipReader()
	if err != nil {
		return nil, errors.Trace(err)
	}

	st := ctx.State()
	storageAccessor, err := getStorageState(st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	domainServices := ctx.DomainServices()
	client := &Client{
		logDir: ctx.LogDir(),
		clock:  ctx.Clock(),

		controllerTag: names.NewControllerTag(ctx.ControllerUUID()),
		modelTag:      names.NewModelTag(ctx.ModelUUID().String()),
		stateAccessor: &stateShim{
			State: st,
		},
		storageAccessor:  storageAccessor,
		auth:             authorizer,
		leadershipReader: leadershipReader,

		applicationService: domainServices.Application(),
		statusService:      domainServices.Status(),
		blockDeviceService: domainServices.BlockDevice(),
		machineService:     domainServices.Machine(),
		modelInfoService:   domainServices.ModelInfo(),
		networkService:     domainServices.Network(),
		portService:        domainServices.Port(),
		relationService:    domainServices.Relation(),

		isControllerModel: ctx.IsControllerModelScoped(),
	}
	return client, nil
}
