// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/internal/errors"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Storage", 6, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newStorageAPIV6(stdCtx, ctx) // modify Remove to support force and maxWait;
	}, reflect.TypeOf((*StorageAPIv6)(nil)))

	registry.MustRegister("Storage", 7, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newStorageAPI(stdCtx, ctx) // support force option on import-fileystem.
	}, reflect.TypeOf((*StorageAPI)(nil)))
}

func newStorageAPIV6(stdCtx context.Context, ctx facade.ModelContext) (*StorageAPIv6, error) {
	storageAPI, err := newStorageAPI(stdCtx, ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return &StorageAPIv6{
		storageAPI,
	}, nil
}

// newStorageAPI returns a new storage API facade.
func newStorageAPI(stdCtx context.Context, ctx facade.ModelContext) (*StorageAPI, error) {
	domainServices := ctx.DomainServices()

	authorizer := ctx.Auth()
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	modelType, err := domainServices.ModelInfo().GetModelType(stdCtx)
	if err != nil {
		return nil, errors.Errorf("getting model type: %w", err)
	}

	return NewStorageAPI(
		ctx.ControllerUUID(),
		ctx.ModelUUID(),
		modelType,
		authorizer,
		ctx.Logger().Child("storage"),
		common.NewBlockChecker(domainServices.BlockCommand()),
		domainServices.Application(),
		domainServices.Removal(),
		domainServices.Storage(),
	), nil
}
