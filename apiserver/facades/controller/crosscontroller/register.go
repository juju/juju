// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crosscontroller

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CrossController", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newStateCrossControllerAPI(ctx)
	}, reflect.TypeOf((*CrossControllerAPI)(nil)))
}

// newStateCrossControllerAPI creates a new server-side CrossModelRelations API facade
// backed by global state.
func newStateCrossControllerAPI(ctx facade.ModelContext) (*CrossControllerAPI, error) {
	st := ctx.State()
	domainServices := ctx.DomainServices()
	return NewCrossControllerAPI(
		ctx.Resources(),
		func(ctx context.Context) ([]string, string, error) {
			controllerConfig := domainServices.ControllerConfig()
			config, err := controllerConfig.ControllerConfig(ctx)
			if err != nil {
				return nil, "", errors.Trace(err)
			}
			return controllerInfo(ctx, domainServices.ControllerNode(), config)
		},
		func(ctx context.Context) (string, error) {
			controllerConfig := domainServices.ControllerConfig()
			config, err := controllerConfig.ControllerConfig(ctx)
			if err != nil {
				return "", errors.Trace(err)
			}
			return config.PublicDNSAddress(), nil
		},
		st.WatchAPIHostPortsForClients,
	)
}

// ControllerInfoGetter indirects state for retrieving information
// required for cross-controller communication.
type ControllerInfoGetter interface {
	// GetAllAPIAddressesForClients returns a string slice of api
	// addresses available for agents.
	GetAllAPIAddressesForClients(ctx context.Context) ([]string, error)
}

// controllerInfo retrieves information required to communicate
// with this controller - API addresses and the CA cert.
func controllerInfo(ctx context.Context, getter ControllerInfoGetter, config controller.Config) ([]string, string, error) {
	apiAddresses, err := getter.GetAllAPIAddressesForClients(ctx)
	if err != nil {
		return nil, "", errors.Trace(err)
	}

	caCert, _ := config.CACert()
	return apiAddresses, caCert, nil
}
