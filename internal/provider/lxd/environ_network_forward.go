// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"context"
	"fmt"
	"maps"
	"net"
	"slices"
	"time"

	"github.com/canonical/lxd/shared/api"
	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/retry"

	"github.com/juju/juju/internal/container/lxd"
)

const (
	jujuInstanceForwardKey = lxd.UserNamespacePrefix + "juju-instance"
	jujuDeviceForwardKey   = lxd.UserNamespacePrefix + "juju-device"
)

// ensureOVNNetworkForwards allocates an external IPv4 address for each OVN
// interface. Ownership tags allow repeated starts to reuse or replace forwards.
func ensureOVNNetworkForwards(ctx context.Context, srv Server, container *lxd.Container, clk clock.Clock) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// ExpandedDevices includes NICs inherited from profiles. Avoid querying
	// networks when the instance has no NICs attached to a named network.
	nics := make(map[string]map[string]string)
	for name, device := range container.ExpandedDevices {
		if device["type"] == "nic" && lxd.NetworkName(device) != "" {
			nics[name] = device
		}
	}
	if len(nics) == 0 {
		return nil
	}
	networks, err := srv.GetNetworks()
	if err != nil {
		return errors.Annotate(err, "retrieving networks")
	}
	ovnNetworks := make(map[string]bool)
	for _, network := range networks {
		if network.Type == networkTypeOVN {
			ovnNetworks[network.Name] = true
		}
	}
	for name, device := range nics {
		if !ovnNetworks[lxd.NetworkName(device)] {
			delete(nics, name)
		}
	}
	if len(nics) == 0 {
		return nil
	}

	// Wait for all guest addresses before allocating any external addresses.
	var state *api.InstanceState
	err = retry.Call(retry.CallArgs{
		Clock:    clk,
		Stop:     ctx.Done(),
		Delay:    time.Second,
		Attempts: 60,
		IsFatalError: func(err error) bool {
			return !errors.Is(err, errors.NotAssigned)
		},
		Func: func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			var err error
			state, _, err = srv.GetInstanceState(container.Name)
			if err != nil {
				return errors.Trace(err)
			}
			for name, device := range nics {
				iface := ovnInterfaceName(name, device)
				if interfaceIPv4Address(state.Network[iface]) == "" {
					return errors.NotAssignedf("IPv4 address for OVN interface %q", iface)
				}
			}
			return nil
		},
	})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return errors.Trace(err)
	}
	for _, name := range slices.Sorted(maps.Keys(nics)) {
		device := nics[name]
		iface := ovnInterfaceName(name, device)
		address := interfaceIPv4Address(state.Network[iface])
		if err := ensureOVNNetworkForward(ctx, srv, lxd.NetworkName(device), container.Name, iface, address); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func ovnInterfaceName(name string, device map[string]string) string {
	if iface := device["name"]; iface != "" {
		return iface
	}
	return name
}

func interfaceIPv4Address(state api.InstanceStateNetwork) string {
	for _, address := range state.Addresses {
		ip := net.ParseIP(address.Address)
		if address.Family == "inet" && ip.To4() != nil && ip.IsGlobalUnicast() {
			return address.Address
		}
	}
	return ""
}

func ensureOVNNetworkForward(ctx context.Context, srv Server, networkName, instanceName, iface, address string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	forwards, err := srv.GetNetworkForwards(networkName)
	if err != nil {
		return errors.Annotatef(err, "retrieving forwards for OVN network %q", networkName)
	}
	var found bool
	for _, forward := range forwards {
		if forward.Config[jujuInstanceForwardKey] != instanceName ||
			forward.Config[jujuDeviceForwardKey] != iface {
			continue
		}
		if !found && forward.Config["target_address"] == address {
			found = true
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		op, err := srv.DeleteNetworkForward(networkName, forward.ListenAddress)
		if err == nil {
			err = op.WaitContext(ctx)
		}
		if err != nil && !lxd.IsLXDNotFound(errors.Cause(err)) {
			return errors.Annotatef(err, "deleting stale forward %q on network %q", forward.ListenAddress, networkName)
		}
	}
	if found {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	forward := api.NetworkForwardsPost{
		ListenAddress: "0.0.0.0",
		NetworkForwardPut: api.NetworkForwardPut{
			Description: fmt.Sprintf("Juju instance %s interface %s", instanceName, iface),
			Config: map[string]string{
				"target_address":       address,
				jujuInstanceForwardKey: instanceName,
				jujuDeviceForwardKey:   iface,
			},
		},
	}
	op, err := srv.CreateNetworkForward(networkName, forward)
	if err == nil {
		err = op.WaitContext(ctx)
	}
	return errors.Annotatef(err, "creating forward for OVN interface %q on network %q", iface, networkName)
}

// removeInstances cleans up owned forwards before deleting provider instances.
// A cleanup failure leaves the instances available for a subsequent retry.
func removeInstances(ctx context.Context, srv Server, names []string) error {
	if err := removeInstanceNetworkForwards(ctx, srv, names); err != nil {
		return errors.Annotate(err, "removing instance network forwards")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return errors.Trace(srv.RemoveContainers(names))
}

func removeInstanceNetworkForwards(ctx context.Context, srv Server, names []string) error {
	if len(names) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !srv.HasExtension("network_forward") {
		return nil
	}
	// Scan by ownership, so cleanup also works after an instance or one of
	// its NICs has disappeared. Only forwards for the requested names qualify.
	owners := set.NewStrings(names...)
	networks, err := srv.GetNetworks()
	if err != nil {
		return errors.Annotate(err, "retrieving networks")
	}
	for _, network := range networks {
		if network.Type != networkTypeOVN {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		forwards, err := srv.GetNetworkForwards(network.Name)
		if err != nil {
			if lxd.IsLXDNotFound(errors.Cause(err)) {
				continue
			}
			return errors.Annotatef(err, "retrieving forwards for network %q", network.Name)
		}
		for _, forward := range forwards {
			owner := forward.Config[jujuInstanceForwardKey]
			if !owners.Contains(owner) {
				continue
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			op, err := srv.DeleteNetworkForward(network.Name, forward.ListenAddress)
			if err == nil {
				err = op.WaitContext(ctx)
			}
			if err != nil && !lxd.IsLXDNotFound(errors.Cause(err)) {
				return errors.Annotatef(err, "deleting forward %q for instance %q on network %q", forward.ListenAddress, owner, network.Name)
			}
		}
	}
	return nil
}
