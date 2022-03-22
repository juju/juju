// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner

import (
	"reflect"

	"github.com/juju/errors"

	charmscommon "github.com/juju/juju/apiserver/common/charms"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
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
	registry.MustRegister("CAASOperatorProvisioner", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newStateCAASOperatorProvisionerAPI(ctx)
	}, reflect.TypeOf((*APIGroup)(nil)))
}

// newStateCAASOperatorProvisionerAPI provides the signature required for facade registration.
func newStateCAASOperatorProvisionerAPI(ctx facade.Context) (*APIGroup, error) {
	authorizer := ctx.Auth()
	resources := ctx.Resources()

	st := ctx.State()
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	broker, err := stateenvirons.GetNewCAASBrokerFunc(caas.New)(model)
	if err != nil {
		return nil, errors.Annotate(err, "getting caas client")
	}
	registry := stateenvirons.NewStorageProviderRegistry(broker)
	pm := poolmanager.New(state.NewStateSettings(ctx.State()), registry)

	commonState := &charmscommon.StateShim{st}
	commonCharmsAPI, err := charmscommon.NewCharmInfoAPI(commonState, authorizer)
	if err != nil {
		return nil, errors.Trace(err)
	}
	appCharmInfoAPI, err := charmscommon.NewApplicationCharmInfoAPI(commonState, authorizer)
	if err != nil {
		return nil, errors.Trace(err)
	}

	api, err := NewCAASOperatorProvisionerAPI(resources, authorizer,
		stateShim{ctx.StatePool().SystemState()},
		stateShim{ctx.State()},
		pm, registry)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &APIGroup{
		charmInfoAPI:    commonCharmsAPI,
		appCharmInfoAPI: appCharmInfoAPI,
		API:             api,
	}, nil
}
