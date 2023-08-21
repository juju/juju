// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Provisioner", 11, func(ctx facade.Context) (facade.Facade, error) {
		return NewProvisionerFacade(ctx) // Relies on agent-set origin in SetHostMachineNetworkConfig.
	}, reflect.TypeOf((*ProvisionerAPIV11)(nil)))
}

// NewProvisionerFacade creates a new server-side Provisioner API facade.
func NewProvisionerFacade(ctx facade.Context) (*ProvisionerAPI, error) {
	controllerConfigGetter := ctx.ServiceFactory().ControllerConfig()
	authorizer := ctx.Auth()
	st := ctx.State()

	systemState, err := ctx.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}

	resources := ctx.Resources()
	externalController := ctx.ServiceFactory().ExternalController()
	logger := ctx.Logger().Child("provisioner")

	return NewProvisionerAPI(controllerConfigGetter, authorizer, st, systemState, resources, externalController, logger)
}
