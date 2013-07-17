// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package local

import (
	"net"

	"launchpad.net/juju-core/utils"
)

var getAddressForInterface = getAddressForInterfaceImpl

// getAddressForInterfaceImpl looks for the network interface
// and returns the IPv4 address from the possible addresses.
func getAddressForInterfaceImpl(interfaceName string) (string, error) {
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		logger.Errorf("cannot find network interface %q: %v", interfaceName, err)
		return "", err
	}
	addrs, err := iface.Addrs()
	if err != nil {
		logger.Errorf("cannot get addresses for network interface %q: %v", interfaceName, err)
		return "", err
	}
	return utils.GetIPv4Address(addrs)
}
