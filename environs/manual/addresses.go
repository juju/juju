// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"net"

	"github.com/juju/juju/network"
)

var netLookupHost = net.LookupHost

// HostAddress returns an network.Address for the specified
// hostname, depending on whether it is an IP or a resolvable
// hostname. The address is given public scope.
func HostAddress(hostname string) (network.Address, error) {
	if ip := net.ParseIP(hostname); ip != nil {
		addr := network.Address{
			Value: ip.String(),
			Type:  network.DeriveAddressType(ip.String()),
			Scope: network.ScopePublic,
		}
		return addr, nil
	}
	// Only a resolvable hostname may be used as a public address.
	if _, err := netLookupHost(hostname); err != nil {
		return network.Address{}, err
	}
	addr := network.Address{
		Value: hostname,
		Type:  network.HostName,
		Scope: network.ScopePublic,
	}
	return addr, nil
}
