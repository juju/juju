// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crosscontroller

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CrossController", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newStateCrossControllerAPI(ctx)
	}, reflect.TypeOf((*CrossControllerAPI)(nil)))
}

// newStateCrossControllerAPI creates a new server-side CrossModelRelations API facade
// backed by global state.
func newStateCrossControllerAPI(ctx facade.Context) (*CrossControllerAPI, error) {
	st := ctx.State()
	return NewCrossControllerAPI(
		ctx.Resources(),
		func() ([]string, string, error) {
			return controllerInfo(st)
		},
		func() (string, error) {
			config, err := st.ControllerConfig()
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
	APIHostPortsForClients() ([]network.SpaceHostPorts, error)
	ControllerConfig() (controller.Config, error)
}

// controllerInfo retrieves information required to communicate
// with this controller - API addresses and the CA cert.
func controllerInfo(st controllerInfoGetter) ([]string, string, error) {
	apiHostPorts, err := st.APIHostPortsForClients()
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

	controllerConfig, err := st.ControllerConfig()
	if err != nil {
		return nil, "", errors.Trace(err)
	}

	caCert, _ := controllerConfig.CACert()
	return addrs, caCert, nil
}
