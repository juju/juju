// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gceapi

import (
	"code.google.com/p/google-api-go-client/compute/v1"

	"github.com/juju/juju/network"
)

const (
	networkDefaultName       = "default"
	networkPathRoot          = "global/networks/"
	networkAccessOneToOneNAT = "ONE_TO_ONE_NAT"
)

type NetworkSpec struct {
	Name string
	// TODO(ericsnow) support a CIDR for internal IP addr range?
}

func (ns *NetworkSpec) path() string {
	name := ns.Name
	if name == "" {
		name = networkDefaultName
	}
	return networkPathRoot + name
}

func (ns *NetworkSpec) newInterface(name string) *compute.NetworkInterface {
	var access []*compute.AccessConfig
	if name != "" {
		// This interface has an internet connection.
		access = append(access, &compute.AccessConfig{
			Name: name,
			Type: networkAccessOneToOneNAT, // the default
			// NatIP (only set if using a reserved public IP)
		})
		// TODO(ericsnow) Will we need to support more access configs?
	}
	return &compute.NetworkInterface{
		Network:       ns.path(),
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
