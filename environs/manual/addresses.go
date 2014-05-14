// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"net"

	"launchpad.net/juju-core/instance"
)

var netLookupHost = net.LookupHost

// HostAddress returns an instance.Address for the specified
// hostname, depending on whether it is an IP or a resolvable
// hostname. The address is given public scope.
func HostAddress(hostname string) (instance.Address, error) {
	if ip := net.ParseIP(hostname); ip != nil {
		addr := instance.Address{
			Value:        ip.String(),
			Type:         instance.DeriveAddressType(ip.String()),
			NetworkScope: instance.NetworkPublic,
		}
		return addr, nil
	}
	// Only a resolvable hostname may be used as a public address.
	if _, err := netLookupHost(hostname); err != nil {
		return instance.Address{}, err
	}
	addr := instance.Address{
		Value:        hostname,
		Type:         instance.HostName,
		NetworkScope: instance.NetworkPublic,
	}
	return addr, nil
}
