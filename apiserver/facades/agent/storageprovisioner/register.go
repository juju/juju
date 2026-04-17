// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/internal/errors"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister(
		"StorageProvisioner", 4,
		func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
			return newFacadeV4(stdCtx, ctx)
		},
		reflect.TypeFor[*StorageProvisionerAPIv4](),
	)
	registry.MustRegister(
		"StorageProvisioner", 5,
		func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
			return newFacadeV5(stdCtx, ctx)
		},
		reflect.TypeFor[*StorageProvisionerAPIv5](),
	)
	registry.MustRegister(
		"StorageProvisioner", 6,
		func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
			return newFacadeV6(stdCtx, ctx)
		},
		reflect.TypeFor[*StorageProvisionerAPIv6](),
	)
	registry.MustRegister(
		"StorageProvisioner", 7,
		func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
			return newFacadeV7(stdCtx, ctx)
		},
		reflect.TypeFor[*StorageProvisionerAPI](),
	)

	registry.MustRegister(
		"VolumeAttachmentsWatcher", 2,
		newMachineStorageIdsWatcherFromContext, reflect.TypeFor[*machineStorageIdsWatcher](),
	)
	registry.MustRegister(
		"VolumeAttachmentPlansWatcher", 1,
		newMachineStorageIdsWatcherFromContext, reflect.TypeFor[*machineStorageIdsWatcher](),
	)
	registry.MustRegister(
		"FilesystemAttachmentsWatcher", 2,
		newMachineStorageIdsWatcherFromContext, reflect.TypeFor[*machineStorageIdsWatcher](),
	)
}

// newFacadeV7 uses
func newFacadeV7(stdCtx context.Context, ctx facade.ModelContext) (*StorageProvisionerAPI, error) {
	domainServices := ctx.DomainServices()

	return NewStorageProvisionerAPI(
		stdCtx,
		ctx.WatcherRegistry(),
		ctx.Clock(),
		domainServices.BlockDevice(),
		domainServices.Machine(),
		domainServices.Application(),
		domainServices.Removal(),
		ctx.Auth(),
		domainServices.Status(),
		domainServices.StorageProvisioning(),
		ctx.Logger().Child("storageprovisioner"),
		ctx.ModelUUID(),
		ctx.ControllerUUID(),
	)
}

// newFacadeV6 provides the signature required for facade registration.
func newFacadeV6(stdCtx context.Context, ctx facade.ModelContext) (*StorageProvisionerAPIv6, error) {
	v7, err := newFacadeV7(stdCtx, ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return &StorageProvisionerAPIv6{
		StorageProvisionerAPI: v7,
	}, nil
}

// newFacadeV5 provides the signature required for facade registration.
func newFacadeV5(stdCtx context.Context, ctx facade.ModelContext) (*StorageProvisionerAPIv5, error) {
	v6, err := newFacadeV6(stdCtx, ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return &StorageProvisionerAPIv5{
		StorageProvisionerAPIv6: v6,
	}, nil
}

// newFacadeV4 provides the signature required for facade registration.
func newFacadeV4(stdCtx context.Context, ctx facade.ModelContext) (*StorageProvisionerAPIv4, error) {
	v5, err := newFacadeV5(stdCtx, ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return &StorageProvisionerAPIv4{
		StorageProvisionerAPIv5: v5,
	}, nil
}
