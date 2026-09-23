// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"context"
	"net"
	"sort"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/container/lxd"
)

// ovnForwardAddresses returns public addresses by guest interface. Only
// Juju-owned forwards on the instance's attached OVN networks are reported.
func ovnForwardAddresses(ctx context.Context, srv Server, instanceName string) (map[string]network.ProviderAddresses, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !srv.HasExtension("network_forward") {
		return nil, nil
	}
	container, _, err := srv.GetInstance(instanceName)
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
	networks, err := srv.GetNetworks()
	if err != nil {
		return nil, errors.Annotate(err, "retrieving networks")
	}
	addresses := make(map[string]network.ProviderAddresses)
	for _, details := range networks {
		if details.Type != networkTypeOVN || interfaces[details.Name] == nil {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		forwards, err := srv.GetNetworkForwards(details.Name)
		if err != nil {
			return nil, errors.Annotatef(err, "retrieving forwards for OVN network %q", details.Name)
		}
		seen := set.NewStrings()
		for _, forward := range forwards {
			iface := forward.Config[jujuDeviceForwardKey]
			if forward.Config[jujuInstanceForwardKey] != instanceName ||
				!interfaces[details.Name].Contains(iface) {
				continue
			}
			ip := net.ParseIP(forward.ListenAddress)
			if !ip.IsGlobalUnicast() || seen.Contains(ip.String()) {
				continue
			}
			seen.Add(ip.String())
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
		sort.Slice(addrs, func(i, j int) bool { return addrs[i].Value < addrs[j].Value })
	}
	return addresses, nil
}
