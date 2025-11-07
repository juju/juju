// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !linux

package sockets

import (
	"crypto/tls"
	"net"
	"net/rpc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// Socket represents the set of parameters to use for socket to dial/listen.
type Socket struct {
	// Network is the socket network.
	Network string

	// Address is the socket address.
	Address string

	// TLSConfig is set when the socket should also establish a TLS connection.
	TLSConfig *tls.Config
}

// Dialer creates a connection based on the provided socket parameters.
func Dialer(soc Socket) (net.Conn, error) {
	return nil, errors.Errorf("dialer not implemented for non-linux systems").Add(coreerrors.NotImplemented)
}

// Dial creates an RPC client based on the provided socket parameters.
func Dial(soc Socket) (*rpc.Client, error) {
	return nil, errors.Errorf("dial not implemented for non-linux systems").Add(coreerrors.NotImplemented)
}

// Listen creates a listener based on the provided socket parameters.
func Listen(soc Socket) (net.Listener, error) {
	return nil, errors.Errorf("lisent not implemented for non-linux systems").Add(coreerrors.NotImplemented)
}
