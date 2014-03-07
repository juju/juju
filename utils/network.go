// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"fmt"
	"net"

	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.utils")

// GetIPv4Address iterates through the addresses expecting the format from
// func (ifi *net.Interface) Addrs() ([]net.Addr, error)
func GetIPv4Address(addresses []net.Addr) (string, error) {
	for _, addr := range addresses {
		ip, _, err := net.ParseCIDR(addr.String())
		if err != nil {
			return "", err
		}
		ipv4 := ip.To4()
		if ipv4 == nil {
			continue
		}
		return ipv4.String(), nil
	}
	return "", fmt.Errorf("no addresses match")
}

// GetAddressForInterface looks for the network interface
// and returns the IPv4 address from the possible addresses.
func GetAddressForInterface(interfaceName string) (string, error) {
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
	return GetIPv4Address(addrs)
}
