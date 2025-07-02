// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// APIAddressAccessor describes methods that allow agents to maintain
// up-to-date information on how to connect to the Juju API server.
type APIAddressAccessor interface {
	// GetAPIHostPortsForAgents returns API HostPorts that are available for
	// agents. HostPorts are grouped by controller node, though each specific
	// controller is not identified.
	GetAPIHostPortsForAgents(ctx context.Context) ([]network.HostPorts, error)

	// GetAllAPIAddressesForAgents returns a string of api
	// addresses available for agents ordered to prefer local-cloud scoped
	// addresses and IPv4 over IPv6 for each machine.
	GetAllAPIAddressesForAgents(ctx context.Context) ([]string, error)

	// WatchControllerAPIAddresses returns a watcher that observes changes to the
	// controller ip addresses.
	WatchControllerAPIAddresses(context.Context) (watcher.NotifyWatcher, error)
}

// APIAddresser implements the APIAddresses method.
// Note that the apiAddressAccessor backing for this implies that it is
// suitable for use by agents, which are bound by the configured controller
// management space. It is not suitable for callers requiring *all* available
// API addresses.
type APIAddresser struct {
	apiAddressAccessor APIAddressAccessor
	watcherRegistry    facade.WatcherRegistry
}

// NewAPIAddresser returns a new APIAddresser that uses the given apiAddressAccessor to
// fetch its addresses.
func NewAPIAddresser(getter APIAddressAccessor, watcherRegistry facade.WatcherRegistry) *APIAddresser {
	return &APIAddresser{
		apiAddressAccessor: getter,
		watcherRegistry:    watcherRegistry,
	}
}

// APIHostPorts returns the API server addresses.
func (a *APIAddresser) APIHostPorts(ctx context.Context) (params.APIHostPortsResult, error) {
	sSvrs, err := a.apiAddressAccessor.GetAPIHostPortsForAgents(ctx)
	if err != nil {
		return params.APIHostPortsResult{}, err
	}

	return params.APIHostPortsResult{
		Servers: params.FromHostsPorts(sSvrs),
	}, nil
}

// WatchAPIHostPorts watches the API server addresses.
func (a *APIAddresser) WatchAPIHostPorts(ctx context.Context) (params.NotifyWatchResult, error) {
	var result params.NotifyWatchResult
	notifyWatcher, err := a.apiAddressAccessor.WatchControllerAPIAddresses(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}
	result.NotifyWatcherId, _, err = internal.EnsureRegisterWatcher(ctx, a.watcherRegistry, notifyWatcher)
	return result, err
}

// APIAddresses returns the list of addresses used to connect to the API.
func (a *APIAddresser) APIAddresses(ctx context.Context) (params.StringsResult, error) {
	addrs, err := a.apiAddressAccessor.GetAllAPIAddressesForAgents(ctx)
	if err != nil {
		return params.StringsResult{}, err
	}
	return params.StringsResult{
		Result: addrs,
	}, nil
}
