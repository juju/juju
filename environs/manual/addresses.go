// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"launchpad.net/juju-core/instance"
)

var instanceHostAddresses = instance.HostAddresses

// HostAddresses returns the addresses for the specified
// hostname, and marks the input address as being public;
// all other addresses have "unknown" scope.
func HostAddresses(hostname string) ([]instance.Address, error) {
	addrs, err := instanceHostAddresses(hostname)
	if err != nil {
		return nil, err
	}
	// The final address is the one we fed in: mark it as public.
	addrs[len(addrs)-1].NetworkScope = instance.NetworkPublic
	return addrs, nil
}
