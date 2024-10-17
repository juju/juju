// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/internal/storage/provider"
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
	registry := provider.CommonStorageProviders()

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
		backend,
		storageBackend,
		domainServices.BlockDevice(),
		domainServices.Config(),
		domainServices.Machine(),
		ctx.Resources(),
		ctx.Auth(),
		registry,
		domainServices.Storage(registry),
		ctx.Logger().Child("storageprovisioner"),
		modelInfo.UUID,
		ctx.ControllerUUID(),
	)
}
