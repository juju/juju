// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"sort"

	"google.golang.org/api/compute/v1"

	"github.com/juju/juju/core/network"
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
// If allocatePublicIP is false the interface will not have a public IP.
// Such interfaces can not access the public internet unless a facility like
// Cloud NAT is recruited by the VPC where they reside.
// See: https://cloud.google.com/nat/docs/using-nat#gcloud_11
func (ns *NetworkSpec) newInterface(name string, allocatePublicIP bool) *compute.NetworkInterface {
	nic := &compute.NetworkInterface{
		Network: ns.Path(),
	}

	if allocatePublicIP {
		nic.AccessConfigs = []*compute.AccessConfig{{
			Name: name,
			Type: NetworkAccessOneToOneNAT,
		}}
	}

	return nic
}

// firewallSpec expands a port range set in to compute.FirewallAllowed
// and returns a compute.Firewall for the provided name.
func firewallSpec(name, target string, sourceCIDRs []string, ports protocolPorts) *compute.Firewall {
	if len(sourceCIDRs) == 0 {
		sourceCIDRs = []string{"0.0.0.0/0"}
	}
	firewall := compute.Firewall{
		// Allowed is set below.
		// Description is not set.
		Name: name,
		// Network: (defaults to global)
		// SourceTags is not set.
		TargetTags:   []string{target},
		SourceRanges: sourceCIDRs,
	}

	var sortedProtocols []string
	for protocol := range ports {
		sortedProtocols = append(sortedProtocols, protocol)
	}
	sort.Strings(sortedProtocols)

	for _, protocol := range sortedProtocols {
		allowed := compute.FirewallAllowed{
			IPProtocol: protocol,
			Ports:      ports.portStrings(protocol),
		}
		firewall.Allowed = append(firewall.Allowed, &allowed)
	}
	return &firewall
}

func extractAddresses(interfaces ...*compute.NetworkInterface) []network.ProviderAddress {
	var addresses []network.ProviderAddress

	for _, netif := range interfaces {
		// Add public addresses.
		for _, accessConfig := range netif.AccessConfigs {
			if accessConfig.NatIP == "" {
				continue
			}
			address := network.ProviderAddress{
				MachineAddress: network.MachineAddress{
					Value: accessConfig.NatIP,
					Type:  network.IPv4Address,
					Scope: network.ScopePublic,
				},
			}
			addresses = append(addresses, address)
		}

		// Add private address.
		if netif.NetworkIP == "" {
			continue
		}
		address := network.ProviderAddress{
			MachineAddress: network.MachineAddress{
				Value: netif.NetworkIP,
				Type:  network.IPv4Address,
				Scope: network.ScopeCloudLocal,
			},
		}
		addresses = append(addresses, address)
	}

	return addresses
}
