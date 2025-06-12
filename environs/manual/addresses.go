// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"github.com/juju/juju/core/network"
)

// HostAddress returns an network.Address for the specified
// hostname, depending on whether it is an IP or a resolvable
// hostname. The address is given public scope.
func HostAddress(hostname string) (network.ProviderAddress, error) {
	if ip := network.DeriveNetIP(hostname); ip != nil {
		addr := network.ProviderAddress{
			MachineAddress: network.MachineAddress{
				Value: ip.String(),
				Type:  network.DeriveAddressType(ip.String()),
				Scope: network.ScopePublic,
			},
		}
		return addr, nil
	}

	// Only a resolvable hostname may be used as a public address.
	if _, err := netLookupHost(hostname); err != nil {
		return network.ProviderAddress{}, err
	}
	addr := network.ProviderAddress{
		MachineAddress: network.MachineAddress{
			Value: hostname,
			Type:  network.HostName,
			Scope: network.ScopePublic,
		},
	}
	return addr, nil
}
