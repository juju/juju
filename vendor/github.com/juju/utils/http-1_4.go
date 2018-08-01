// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

//+build !go1.7

package utils

import (
	"fmt"
	"net"
	"net/http"
)

// installHTTPDialShim patches the default HTTP transport so
// that it fails when an attempt is made to dial a non-local
// host.
func installHTTPDialShim(t *http.Transport) {
	t.Dial = func(network, addr string) (net.Conn, error) {
		if !OutgoingAccessAllowed && !isLocalAddr(addr) {
			return nil, fmt.Errorf("access to address %q not allowed", addr)
		}
		return net.Dial(network, addr)
	}
}
