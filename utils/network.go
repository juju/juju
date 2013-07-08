// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"fmt"
	"net"
)

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
