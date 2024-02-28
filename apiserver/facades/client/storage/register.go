// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common/credentialcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/poolmanager"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Storage", 6, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newStorageAPI(ctx) // modify Remove to support force and maxWait; add DetachStorage to support force and maxWait.
	}, reflect.TypeOf((*StorageAPI)(nil)))
}

// newStorageAPI returns a new storage API facade.
func newStorageAPI(ctx facade.ModelContext) (*StorageAPI, error) {
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
			ctx.ServiceFactory().Cloud(),
			ctx.ServiceFactory().Credential(),
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

	return NewStorageAPI(
		stateShim{st}, model.Type(),
		storageAccessor, ctx.ServiceFactory().BlockDevice(), storageMetadata, authorizer,
		credentialcommon.CredentialInvalidatorGetter(ctx)), nil
}
