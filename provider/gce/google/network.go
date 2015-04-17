// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"google.golang.org/api/compute/v1"

	"github.com/juju/juju/network"
)

const (
	networkDefaultName = "default"
	networkPathRoot    = "global/networks/"
)

// The different kinds of network access.
const (
	NetworkAccessOneToOneNAT = "ONE_TO_ONE_NAT" // the default
)

// NetworkSpec holds all the information needed to identify and create
// a GCE network.
type NetworkSpec struct {
	// Name is the unqualified name of the network.
	Name string
	// TODO(ericsnow) support a CIDR for internal IP addr range?
}

// Path returns the qualified name of the network.
func (ns *NetworkSpec) Path() string {
	name := ns.Name
	if name == "" {
		name = networkDefaultName
	}
	return networkPathRoot + name
}

// newInterface builds up all the data needed by the GCE API to create
// a new interface connected to the network.
func (ns *NetworkSpec) newInterface(name string) *compute.NetworkInterface {
	var access []*compute.AccessConfig
	if name != "" {
		// This interface has an internet connection.
		access = append(access, &compute.AccessConfig{
			Name: name,
			Type: NetworkAccessOneToOneNAT,
			// NatIP (only set if using a reserved public IP)
		})
		// TODO(ericsnow) Will we need to support more access configs?
	}
	return &compute.NetworkInterface{
		Network:       ns.Path(),
		AccessConfigs: access,
	}
}

// firewallSpec expands a port range set in to compute.FirewallAllowed
// and returns a compute.Firewall for the provided name.
func firewallSpec(name string, ps network.PortSet) *compute.Firewall {
	firewall := compute.Firewall{
		// Allowed is set below.
		// Description is not set.
		Name: name,
		// Network: (defaults to global)
		// SourceTags is not set.
		TargetTags:   []string{name},
		SourceRanges: []string{"0.0.0.0/0"},
	}

	for _, protocol := range ps.Protocols() {
		allowed := compute.FirewallAllowed{
			IPProtocol: protocol,
			Ports:      ps.PortStrings(protocol),
		}
		firewall.Allowed = append(firewall.Allowed, &allowed)
	}
	return &firewall
}

func extractAddresses(interfaces ...*compute.NetworkInterface) []network.Address {
	var addresses []network.Address

	for _, netif := range interfaces {
		// Add public addresses.
		for _, accessConfig := range netif.AccessConfigs {
			if accessConfig.NatIP == "" {
				continue
			}
			address := network.Address{
				Value: accessConfig.NatIP,
				Type:  network.IPv4Address,
				Scope: network.ScopePublic,
			}
			addresses = append(addresses, address)

		}

		// Add private address.
		if netif.NetworkIP == "" {
			continue
		}
		address := network.Address{
			Value: netif.NetworkIP,
			Type:  network.IPv4Address,
			Scope: network.ScopeCloudLocal,
		}
		addresses = append(addresses, address)
	}

	return addresses
}
