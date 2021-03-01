// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"net"
)

var netListen = net.Listen

// SupportsIPv6 reports whether the platform supports IPv6 networking
// functionality.
//
// Source: https://github.com/golang/net/blob/master/internal/nettest/stack.go
func SupportsIPv6() bool {
	ln, err := netListen("tcp6", "[::1]:0")
	if err != nil {
		return false
	}
	ln.Close()
	return true
}
