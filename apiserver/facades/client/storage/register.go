// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"reflect"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/v2/apiserver/errors"
	"github.com/juju/juju/v2/apiserver/facade"
	"github.com/juju/juju/v2/caas"
	"github.com/juju/juju/v2/environs"
	"github.com/juju/juju/v2/environs/context"
	"github.com/juju/juju/v2/state"
	"github.com/juju/juju/v2/state/stateenvirons"
	"github.com/juju/juju/v2/storage"
	"github.com/juju/juju/v2/storage/poolmanager"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Storage", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newStorageAPIV3(ctx)
	}, reflect.TypeOf((*StorageAPIv3)(nil)))
	registry.MustRegister("Storage", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newStorageAPIV4(ctx) // changes Destroy() method signature.
	}, reflect.TypeOf((*StorageAPIv4)(nil)))
	registry.MustRegister("Storage", 5, func(ctx facade.Context) (facade.Facade, error) {
		return newStorageAPIV5(ctx) // Update and Delete storage pools and CreatePool bulk calls.
	}, reflect.TypeOf((*StorageAPIv5)(nil)))
	registry.MustRegister("Storage", 6, func(ctx facade.Context) (facade.Facade, error) {
		return newStorageAPI(ctx) // modify Remove to support force and maxWait; add DetachStorage to support force and maxWait.
	}, reflect.TypeOf((*StorageAPI)(nil)))
}

// newStorageAPIV5 returns a new storage v5 API facade.
func newStorageAPIV5(context facade.Context) (*StorageAPIv5, error) {
	storageAPI, err := newStorageAPI(context)
	if err != nil {
		return nil, err
	}
	return &StorageAPIv5{
		StorageAPI: *storageAPI,
	}, nil
}

// newStorageAPIV4 returns a new storage v4 API facade.
func newStorageAPIV4(context facade.Context) (*StorageAPIv4, error) {
	storageAPI, err := newStorageAPIV5(context)
	if err != nil {
		return nil, err
	}
	return &StorageAPIv4{
		StorageAPIv5: *storageAPI,
	}, nil
}

// newStorageAPIV3 returns a new storage v3 API facade.
func newStorageAPIV3(context facade.Context) (*StorageAPIv3, error) {
	storageAPI, err := newStorageAPIV4(context)
	if err != nil {
		return nil, err
	}
	return &StorageAPIv3{
		StorageAPIv4: *storageAPI,
	}, nil
}

// newStorageAPI returns a new storage API facade.
func newStorageAPI(ctx facade.Context) (*StorageAPI, error) {
	st := ctx.State()
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	storageMetadata := func() (poolmanager.PoolManager, storage.ProviderRegistry, error) {
		model, err := st.Model()
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		registry, err := stateenvirons.NewStorageProviderRegistryForModel(
			model,
			stateenvirons.GetNewEnvironFunc(environs.New),
			stateenvirons.GetNewCAASBrokerFunc(caas.New))
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		pm := poolmanager.New(state.NewStateSettings(st), registry)
		return pm, registry, nil
	}
	storageAccessor, err := getStorageAccessor(st)
	if err != nil {
		return nil, errors.Annotate(err, "getting backend")
	}

	authorizer := ctx.Auth()
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	return NewStorageAPI(stateShim{st}, model.Type(), storageAccessor, storageMetadata, authorizer, context.CallContext(st)), nil
}
