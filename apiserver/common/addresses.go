// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// APIAddressAccessor describes methods that allow agents to maintain
// up-to-date information on how to connect to the Juju API server.
type APIAddressAccessor interface {
	APIHostPortsForAgents(controller.Config) ([]network.SpaceHostPorts, error)
	WatchAPIHostPortsForAgents() state.NotifyWatcher
}

// APIAddresser implements the APIAddresses method.
// Note that the getter backing for this implies that it is suitable for use by
// agents, which are bound by the configured controller management space.
// It is not suitable for callers requiring *all* available API addresses.
type APIAddresser struct {
	resources         facade.Resources
	getter            APIAddressAccessor
	ctrlConfigService ControllerConfigGetter
}

// NewAPIAddresser returns a new APIAddresser that uses the given getter to
// fetch its addresses.
func NewAPIAddresser(getter APIAddressAccessor, resources facade.Resources, ctrlConfigService ControllerConfigGetter) *APIAddresser {
	return &APIAddresser{
		getter:            getter,
		resources:         resources,
		ctrlConfigService: ctrlConfigService,
	}
}

// APIHostPorts returns the API server addresses.
func (a *APIAddresser) APIHostPorts() (params.APIHostPortsResult, error) {
	controllerConfig, err := a.ctrlConfigService.ControllerConfig(context.TODO())
	if err != nil {
		return params.APIHostPortsResult{}, err
	}
	sSvrs, err := a.getter.APIHostPortsForAgents(controllerConfig)
	if err != nil {
		return params.APIHostPortsResult{}, err
	}

	// Convert the SpaceHostPorts to the HostPorts indirection.
	pSvrs := make([]network.HostPorts, len(sSvrs))
	for i, sHPs := range sSvrs {
		pSvrs[i] = sHPs.HostPorts()
	}

	return params.APIHostPortsResult{
		Servers: params.FromHostsPorts(pSvrs),
	}, nil
}

// WatchAPIHostPorts watches the API server addresses.
func (a *APIAddresser) WatchAPIHostPorts() (params.NotifyWatchResult, error) {
	watch := a.getter.WatchAPIHostPortsForAgents()
	if _, ok := <-watch.Changes(); ok {
		return params.NotifyWatchResult{
			NotifyWatcherId: a.resources.Register(watch),
		}, nil
	}
	return params.NotifyWatchResult{}, watcher.EnsureErr(watch)
}

// APIAddresses returns the list of addresses used to connect to the API.
func (a *APIAddresser) APIAddresses() (params.StringsResult, error) {
	addrs, err := apiAddresses(a.getter, a.ctrlConfigService)
	if err != nil {
		return params.StringsResult{}, err
	}
	return params.StringsResult{
		Result: addrs,
	}, nil
}

func apiAddresses(getter APIHostPortsForAgentsGetter, ctrlConfigService ControllerConfigGetter) ([]string, error) {
	controllerConfig, err := ctrlConfigService.ControllerConfig(context.TODO())
	if err != nil {
		return nil, err
	}
	apiHostPorts, err := getter.APIHostPortsForAgents(controllerConfig)
	if err != nil {
		return nil, err
	}
	var addrs = make([]string, 0, len(apiHostPorts))
	for _, hostPorts := range apiHostPorts {
		ordered := hostPorts.HostPorts().PrioritizedForScope(network.ScopeMatchCloudLocal)
		for _, addr := range ordered {
			if addr != "" {
				addrs = append(addrs, addr)
			}
		}
	}
	return addrs, nil
}
