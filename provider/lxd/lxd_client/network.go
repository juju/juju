// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_client

import (
	"github.com/juju/juju/network"
)

// inspired by GCE's compute.Firewall
type fwSpec struct {
	name string
	//description string
	//network string
	//sourceTags []string
	//targetTags []string
	sourceRanges []string // or []network.PortSet?
	allowed      []network.PortSet
}

// firewallSpec expands a port range set in to compute.FirewallAllowed
// and returns a compute.Firewall for the provided name.
func firewallSpec(name string, ps ...network.PortSet) *fwSpec {
	spec := fwSpec{
		// Allowed is set below.
		// Description is not set.
		name:         name,
		sourceRanges: []string{"0.0.0.0/0"},
		allowed:      ps,
	}
	return &spec
}
