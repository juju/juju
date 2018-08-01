// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

//+build go1.7

package utils

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

var ctxtDialer = &net.Dialer{
	Timeout:   30 * time.Second,
	KeepAlive: 30 * time.Second,
}

// installHTTPDialShim patches the default HTTP transport so
// that it fails when an attempt is made to dial a non-local
// host.
//
// Note that this is Go version dependent because in Go 1.7 and above,
// the DialContext field was introduced (and set in http.DefaultTransport)
// which overrides the Dial field.
func installHTTPDialShim(t *http.Transport) {
	t.DialContext = func(ctxt context.Context, network, addr string) (net.Conn, error) {
		if !OutgoingAccessAllowed && !isLocalAddr(addr) {
			return nil, fmt.Errorf("access to address %q not allowed", addr)
		}
		return ctxtDialer.DialContext(ctxt, network, addr)
	}
}
