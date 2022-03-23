// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/storage/poolmanager"
)

// Registry describes the API facades exposed by some API server.
type Registry interface {
	// MustRegister adds a single named facade at a given version to the
	// registry.
	// Factory will be called when someone wants to instantiate an object of
	// this facade, and facadeType defines the concrete type that the returned
	// object will be.
	// The Type information is used to define what methods will be exported in
	// the API, and it must exactly match the actual object returned by the
	// factory.
	MustRegister(string, int, facade.Factory, reflect.Type)
}

// Register is called to expose a package of facades onto a given registry.
func Register(registry Registry) {
	registry.MustRegister("StorageProvisioner", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV3(ctx)
	}, reflect.TypeOf((*StorageProvisionerAPIv3)(nil)))
	registry.MustRegister("StorageProvisioner", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV4(ctx)
	}, reflect.TypeOf((*StorageProvisionerAPIv4)(nil)))
}

// newFacadeV3 provides the signature required for facade registration.
func newFacadeV3(ctx facade.Context) (*StorageProvisionerAPIv3, error) {
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
		return nil, errors.Annotate(err, "getting backend")
	}
	return NewStorageProvisionerAPIv3(backend, storageBackend, ctx.Resources(), ctx.Auth(), registry, pm)
}

// newFacadeV4 provides the signature required for facade registration.
func newFacadeV4(ctx facade.Context) (*StorageProvisionerAPIv4, error) {
	v3, err := newFacadeV3(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewStorageProvisionerAPIv4(v3), nil
}
