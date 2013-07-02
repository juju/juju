// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"fmt"
	"net"
	"strings"
)

// GetIPv4Address iterates through the addresses expecting the format from
// func (ifi *net.Interface) Addrs() ([]net.Addr, error)
func GetIPv4Address(addresses []net.Addr) (string, error) {
	for _, addr := range addresses {
		// IPv4 look like this: "10.0.3.1/24"
		// IPv6 look like this: "fe80::90cf:9dff:fe6e:ece/64"
		bits := strings.Split(addr.String(), "/")
		if len(bits) != 2 {
			continue
		}
		ip := net.ParseIP(bits[0])
		if ip == nil {
			continue
		}
		ipv4 := ip.To4()
		if ipv4 == nil {
			continue
		}
		return ipv4.String(), nil
	}
	return "", fmt.Errorf("no addresses match")
}
