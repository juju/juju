// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"reflect"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/v3/apiserver/errors"
	"github.com/juju/juju/v3/apiserver/facade"
	"github.com/juju/juju/v3/caas"
	"github.com/juju/juju/v3/environs"
	"github.com/juju/juju/v3/environs/context"
	"github.com/juju/juju/v3/state"
	"github.com/juju/juju/v3/state/stateenvirons"
	"github.com/juju/juju/v3/storage"
	"github.com/juju/juju/v3/storage/poolmanager"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Storage", 6, func(ctx facade.Context) (facade.Facade, error) {
		return newStorageAPI(ctx) // modify Remove to support force and maxWait; add DetachStorage to support force and maxWait.
	}, reflect.TypeOf((*StorageAPI)(nil)))
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
