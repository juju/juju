// Copyright 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

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

// GetIPv6Address iterates through the addresses expecting the format from
// func (ifi *net.Interface) Addrs() ([]net.Addr, error) and returns the first
// non-link local address.
func GetIPv6Address(addresses []net.Addr) (string, error) {
	_, llNet, _ := net.ParseCIDR("fe80::/10")
	for _, addr := range addresses {
		ip, _, err := net.ParseCIDR(addr.String())
		if err != nil {
			return "", err
		}
		if ip.To4() == nil && !llNet.Contains(ip) {
			return ip.String(), nil
		}
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

// GetV4OrV6AddressForInterface looks for the network interface
// and returns preferably the IPv4 address, and if it doesn't
// exists then IPv6 address.
func GetV4OrV6AddressForInterface(interfaceName string) (string, error) {
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
	if ip, err := GetIPv4Address(addrs); err == nil {
		return ip, nil
	}
	return GetIPv6Address(addrs)
}
