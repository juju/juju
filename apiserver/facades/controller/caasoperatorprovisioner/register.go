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

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
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

	systemState, err := ctx.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	api, err := NewCAASOperatorProvisionerAPI(resources, authorizer,
		stateShim{systemState},
		stateShim{ctx.State()},
		pm, registry, broker)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &APIGroup{
		charmInfoAPI:    commonCharmsAPI,
		appCharmInfoAPI: appCharmInfoAPI,
		API:             api,
	}, nil
}
