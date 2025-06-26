// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"
	"net/netip"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// APIAddressAccessor describes methods that allow agents to maintain
// up-to-date information on how to connect to the Juju API server.
type APIAddressAccessor interface {
	// GetAllAPIAddressesByControllerIDForAgents returns a map of controller IDs to their API
	// addresses that are available for agents. The map is keyed by controller
	// ID, and the values are slices of strings representing the API addresses
	// for each controller node.
	GetAllAPIAddressesByControllerIDForAgents(ctx context.Context) (map[string][]string, error)

	// GetAllAPIAddressesForAgentsInPreferredOrder returns a string of api
	// addresses available for agents ordered to prefer local-cloud scoped
	// addresses and IPv4 over IPv6 for each machine.
	GetAllAPIAddressesForAgentsInPreferredOrder(ctx context.Context) ([]string, error)

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
	srvs, err := a.apiAddressAccessor.GetAllAPIAddressesByControllerIDForAgents(ctx)
	if err != nil {
		return params.APIHostPortsResult{}, err
	}

	// Convert the strings to the HostPorts.
	serverResults := make([][]params.HostPort, 0)
	for _, addrs := range srvs {
		out, err := transformToHostPorts(addrs)
		if err != nil {
			return params.APIHostPortsResult{}, err
		}
		serverResults = append(serverResults, out)
	}

	return params.APIHostPortsResult{
		Servers: serverResults,
	}, nil
}

func transformToHostPorts(input []string) ([]params.HostPort, error) {
	results := make([]params.HostPort, len(input))
	for i, in := range input {
		addrPort, err := netip.ParseAddrPort(in)
		if err != nil {
			return nil, err
		}
		results[i] = params.HostPort{
			Address: params.Address{Value: addrPort.Addr().String()},
			Port:    int(addrPort.Port()),
		}
	}
	return results, nil
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
	addrs, err := a.apiAddressAccessor.GetAllAPIAddressesForAgentsInPreferredOrder(ctx)
	if err != nil {
		return params.StringsResult{}, err
	}
	return params.StringsResult{
		Result: addrs,
	}, nil
}
