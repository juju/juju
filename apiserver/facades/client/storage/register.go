// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"
	"reflect"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Storage", 6, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newStorageAPI(stdCtx, ctx) // modify Remove to support force and maxWait; add DetachStorage to support force and maxWait.
	}, reflect.TypeOf((*StorageAPI)(nil)))
}

// newStorageAPI returns a new storage API facade.
func newStorageAPI(stdCtx context.Context, ctx facade.ModelContext) (*StorageAPI, error) {
	domainServices := ctx.DomainServices()

	authorizer := ctx.Auth()
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	storageService := domainServices.Storage()
	return NewStorageAPI(
		ctx.ControllerUUID(),
		ctx.ModelUUID(),
		domainServices.BlockDevice(),
		storageService,
		domainServices.Application(),
		storageService.GetStorageRegistry,
		authorizer,
		domainServices.BlockCommand()), nil
}
