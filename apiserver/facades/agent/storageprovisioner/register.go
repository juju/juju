// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("StorageProvisioner", 4, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacadeV4(stdCtx, ctx)
	}, reflect.TypeOf((*StorageProvisionerAPIv4)(nil)))
}

// newFacadeV4 provides the signature required for facade registration.
func newFacadeV4(stdCtx context.Context, ctx facade.ModelContext) (*StorageProvisionerAPIv4, error) {
	st := ctx.State()

	domainServices := ctx.DomainServices()
	storageService := domainServices.Storage()

	registry, err := storageService.GetStorageRegistry(stdCtx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Get model UUID
	modelInfo, err := domainServices.ModelInfo().GetModelInfo(stdCtx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	backend, storageBackend, err := NewStateBackends(st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewStorageProvisionerAPIv4(
		stdCtx,
		ctx.WatcherRegistry(),
		ctx.Clock(),
		backend,
		storageBackend,
		domainServices.BlockDevice(),
		domainServices.Config(),
		domainServices.Machine(),
		domainServices.Application(),
		ctx.Resources(),
		ctx.Auth(),
		registry,
		storageService,
		ctx.Logger().Child("storageprovisioner"),
		modelInfo.UUID,
		ctx.ControllerUUID(),
	)
}
