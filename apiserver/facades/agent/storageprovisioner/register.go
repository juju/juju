// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/v3/apiserver/facade"
	"github.com/juju/juju/v3/caas"
	"github.com/juju/juju/v3/environs"
	"github.com/juju/juju/v3/state"
	"github.com/juju/juju/v3/state/stateenvirons"
	"github.com/juju/juju/v3/storage/poolmanager"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("StorageProvisioner", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV4(ctx)
	}, reflect.TypeOf((*StorageProvisionerAPIv4)(nil)))
}

// newFacadeV4 provides the signature required for facade registration.
func newFacadeV4(ctx facade.Context) (*StorageProvisionerAPIv4, error) {
	st := ctx.State()
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	registry, err := stateenvirons.NewStorageProviderRegistryForModel(
		model,
		stateenvirons.GetNewEnvironFunc(environs.New),
		stateenvirons.GetNewCAASBrokerFunc(caas.New),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	pm := poolmanager.New(state.NewStateSettings(st), registry)

	backend, storageBackend, err := NewStateBackends(st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewStorageProvisionerAPIv4(backend, storageBackend, ctx.Resources(), ctx.Auth(), registry, pm)
}
