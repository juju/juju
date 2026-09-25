// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"cmp"
	"context"
	"net"
	"slices"

	"github.com/canonical/lxd/shared/api"
	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/container/lxd"
)

// ovnForwardAddresses returns public addresses by guest interface. Only
// Juju-owned forwards on the instance's attached OVN networks are reported.
func ovnForwardAddresses(ctx context.Context, srv Server, instanceName string) (map[string]network.ProviderAddresses, error) {
	lookup := ovnForwardAddressLookup{srv: srv}
	return lookup.addresses(ctx, instanceName)
}

// ovnForwardAddressLookup shares network and forward reads within one poll.
// It is local to the caller so subsequent polls always observe fresh state.
type ovnForwardAddressLookup struct {
	srv              Server
	extensionChecked bool
	supported        bool
	networks         set.Strings
	forwards         map[string]map[string][]api.NetworkForward
}

func (l *ovnForwardAddressLookup) addresses(ctx context.Context, instanceName string) (map[string]network.ProviderAddresses, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !l.extensionChecked {
		l.supported = l.srv.HasExtension("network_forward")
		l.extensionChecked = true
	}
	if !l.supported {
		return nil, nil
	}
	if l.networks == nil {
		networks, err := l.srv.GetNetworks()
		if err != nil {
			return nil, errors.Annotate(err, "retrieving networks")
		}
		l.networks = set.NewStrings()
		for _, details := range networks {
			if details.Type == networkTypeOVN {
				l.networks.Add(details.Name)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Avoid per-instance lookups when the project has no OVN networks.
	if len(l.networks) == 0 {
		return nil, nil
	}
	container, _, err := l.srv.GetInstance(instanceName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	interfaces := make(map[string]set.Strings)
	for name, device := range container.ExpandedDevices {
		networkName := lxd.NetworkName(device)
		if device["type"] != "nic" || networkName == "" {
			continue
		}
		if interfaces[networkName] == nil {
			interfaces[networkName] = set.NewStrings()
		}
		interfaces[networkName].Add(ovnInterfaceName(name, device))
	}
	if len(interfaces) == 0 {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	addresses := make(map[string]network.ProviderAddresses)
	seen := make(map[string]set.Strings)
	for _, name := range l.networks.SortedValues() {
		if interfaces[name] == nil {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		forwards, err := l.networkForwards(name)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, forward := range forwards[instanceName] {
			iface := forward.Config[jujuDeviceForwardKey]
			if !interfaces[name].Contains(iface) {
				continue
			}
			if seen[iface] == nil {
				seen[iface] = set.NewStrings()
			}
			ip := net.ParseIP(forward.ListenAddress)
			if !ip.IsGlobalUnicast() || seen[iface].Contains(ip.String()) {
				continue
			}
			seen[iface].Add(ip.String())
			// The forward is an ingress address, even on a private uplink.
			// It must not be selected for services binding inside the guest.
			addresses[iface] = append(addresses[iface], network.NewMachineAddress(
				ip.String(), network.WithScope(network.ScopePublic),
			).AsProviderAddress())
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, addrs := range addresses {
		slices.SortFunc(addrs, func(a, b network.ProviderAddress) int {
			return cmp.Compare(a.Value, b.Value)
		})
	}
	return addresses, nil
}

func (l *ovnForwardAddressLookup) networkForwards(name string) (map[string][]api.NetworkForward, error) {
	if forwards, ok := l.forwards[name]; ok {
		return forwards, nil
	}
	forwards, err := l.srv.GetNetworkForwards(name)
	if err != nil {
		return nil, errors.Annotatef(err, "retrieving forwards for OVN network %q", name)
	}
	byInstance := make(map[string][]api.NetworkForward)
	for _, forward := range forwards {
		owner := forward.Config[jujuInstanceForwardKey]
		if owner != "" {
			byInstance[owner] = append(byInstance[owner], forward)
		}
	}
	if l.forwards == nil {
		l.forwards = make(map[string]map[string][]api.NetworkForward)
	}
	l.forwards[name] = byInstance
	return byInstance, nil
}
