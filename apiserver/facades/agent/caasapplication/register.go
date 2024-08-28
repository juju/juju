// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplication

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/state/stateenvirons"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CAASApplication", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newStateFacade(stdCtx, ctx)
	}, reflect.TypeOf((*Facade)(nil)))
}

// newStateFacade provides the signature required for facade registration.
func newStateFacade(stdCtx context.Context, ctx facade.ModelContext) (*Facade, error) {
	authorizer := ctx.Auth()
	resources := ctx.Resources()
	st := ctx.State()
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	serviceFactory := ctx.ServiceFactory()

	broker, err := stateenvirons.GetNewCAASBrokerFunc(caas.New)(model, serviceFactory.Cloud(), serviceFactory.Credential())
	if err != nil {
		return nil, errors.Annotate(err, "getting caas client")
	}
	systemState, err := ctx.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}

	tracker, err := ctx.GetProvider(stdCtx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	registry := storage.NewChainedProviderRegistry(tracker)
	return NewFacade(
		resources,
		authorizer,
		systemState,
		&stateShim{State: st},
		serviceFactory.ControllerConfig(),
		serviceFactory.Application(registry),
		broker,
		ctx.StatePool().Clock(),
		ctx.Logger().Child("caasapplication"),
	)
}
