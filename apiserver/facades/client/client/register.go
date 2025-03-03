// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"context"
	"reflect"

	"github.com/juju/errors"

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

	st := ctx.State()
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	leadershipReader, err := ctx.LeadershipReader()
	if err != nil {
		return nil, errors.Trace(err)
	}

	storageAccessor, err := getStorageState(st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	domainServices := ctx.DomainServices()
	client := &Client{
		stateAccessor: &stateShim{
			State:   st,
			model:   model,
			session: nil,
		},
		storageAccessor:    storageAccessor,
		blockDeviceService: domainServices.BlockDevice(),
		auth:               authorizer,
		leadershipReader:   leadershipReader,
		networkService:     domainServices.Network(),
		modelInfoService:   domainServices.ModelInfo(),
		machineService:     domainServices.Machine(),
		applicationService: domainServices.Application(),
		portService:        domainServices.Port(),
	}
	return client, nil
}
