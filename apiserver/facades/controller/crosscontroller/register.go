// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crosscontroller

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
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
	serviceFactory := ctx.ServiceFactory()
	return NewCrossControllerAPI(
		ctx.Resources(),
		func(ctx context.Context) ([]string, string, error) {
			controllerConfig := serviceFactory.ControllerConfig()
			config, err := controllerConfig.ControllerConfig(ctx)
			if err != nil {
				return nil, "", errors.Trace(err)
			}
			return controllerInfo(st, config)
		},
		func(ctx context.Context) (string, error) {
			controllerConfig := serviceFactory.ControllerConfig()
			config, err := controllerConfig.ControllerConfig(ctx)
			if err != nil {
				return "", errors.Trace(err)
			}
			return config.PublicDNSAddress(), nil
		},
		st.WatchAPIHostPortsForClients,
	)
}

// controllerInfoGetter indirects state for retrieving information
// required for cross-controller communication.
type controllerInfoGetter interface {
	APIHostPortsForClients(config controller.Config) ([]network.SpaceHostPorts, error)
}

// controllerInfo retrieves information required to communicate
// with this controller - API addresses and the CA cert.
func controllerInfo(st controllerInfoGetter, config controller.Config) ([]string, string, error) {
	apiHostPorts, err := st.APIHostPortsForClients(config)
	if err != nil {
		return nil, "", errors.Trace(err)
	}

	var addrs []string
	for _, hostPorts := range apiHostPorts {
		ordered := hostPorts.HostPorts().PrioritizedForScope(network.ScopeMatchPublic)
		for _, addr := range ordered {
			if addr != "" {
				addrs = append(addrs, addr)
			}
		}
	}

	caCert, _ := config.CACert()
	return addrs, caCert, nil
}
