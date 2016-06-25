// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tls

import (
	"crypto/tls"
	"net"
	"time"

	"github.com/juju/errors"
)

// DialOpts holds the options that will be used to open a TLS connection.
type DialOpts struct {
	// Address is the net address of the remote host.
	Address string

	// TLSConfig is the config to use when connecting.
	TLSConfig Config

	// ConnectTimeout is how long to wait before timing out the
	// connection attempt.
	ConnectTimeout time.Duration
}

// DialTCP opens a TLS connection over TCP.
func DialTCP(opts DialOpts) (*tls.Conn, error) {
	tlsConfig, err := opts.TLSConfig.TLS()
	if err != nil {
		return nil, errors.Trace(err)
	}

	if opts.ConnectTimeout == 0 {
		return dial(tlsConfig, "tcp", opts.Address)
	} else {
		return dialTimeout(tlsConfig, "tcp", opts.Address, opts.ConnectTimeout)
	}
}

func dial(cfg *tls.Config, network, address string) (*tls.Conn, error) {
	conn, err := tls.Dial(network, address, cfg)
	return conn, errors.Trace(err)
}

func dialTimeout(cfg *tls.Config, network, address string, timeout time.Duration) (*tls.Conn, error) {
	dialer := &net.Dialer{
		Timeout: timeout,
	}
	conn, err := tls.DialWithDialer(dialer, network, address, cfg)
	return conn, errors.Trace(err)
}
