// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"crypto/tls"
	"crypto/x509"
	stderrors "errors"
	"fmt"
	"net"
	"time"

	"labix.org/v2/mgo"

	"github.com/juju/juju/cert"
)

// SocketTimeout should be long enough that
// even a slow mongo server will respond in that
// length of time. Since mongo servers ping themselves
// every 10 seconds, we use a value of just over 2
// ping periods to allow for delayed pings due to
// issues such as CPU starvation etc.
const SocketTimeout = 21 * time.Second

// defaultDialTimeout should be representative of
// the upper bound of time taken to dial a mongo
// server from within the same cloud/private network.
const defaultDialTimeout = 30 * time.Second

// DialOpts holds configuration parameters that control the
// Dialing behavior when connecting to a state server.
type DialOpts struct {
	// Timeout is the amount of time to wait contacting
	// a state server.
	Timeout time.Duration

	// Direct informs whether to establish connections only with the
	// specified seed servers, or to obtain information for the whole
	// cluster and establish connections with further servers too.
	Direct bool
}

// DefaultDialOpts returns a DialOpts representing the default
// parameters for contacting a state server.
func DefaultDialOpts() DialOpts {
	return DialOpts{Timeout: defaultDialTimeout}
}

// Info encapsulates information about cluster of
// mongo servers and can be used to make a
// connection to that cluster.
type Info struct {
	// Addrs gives the addresses of the MongoDB servers for the state.
	// Each address should be in the form address:port.
	Addrs []string

	// CACert holds the CA certificate that will be used
	// to validate the state server's certificate, in PEM format.
	CACert string
}

// DialInfo returns information on how to dial
// the state's mongo server with the given info
// and dial options.
func DialInfo(info Info, opts DialOpts) (*mgo.DialInfo, error) {
	if len(info.Addrs) == 0 {
		return nil, stderrors.New("no mongo addresses")
	}
	if len(info.CACert) == 0 {
		return nil, stderrors.New("missing CA certificate")
	}
	xcert, err := cert.ParseCert(info.CACert)
	if err != nil {
		return nil, fmt.Errorf("cannot parse CA certificate: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(xcert)
	tlsConfig := &tls.Config{
		RootCAs:    pool,
		ServerName: "anything",
	}
	dial := func(addr net.Addr) (net.Conn, error) {
		c, err := net.Dial("tcp", addr.String())
		if err != nil {
			logger.Debugf("connection failed, will retry: %v", err)
			return nil, err
		}
		cc := tls.Client(c, tlsConfig)
		if err := cc.Handshake(); err != nil {
			logger.Debugf("TLS handshake failed: %v", err)
			return nil, err
		}
		logger.Infof("dialled mongo successfully on address %q", addr)
		return cc, nil
	}

	return &mgo.DialInfo{
		Addrs:   info.Addrs,
		Timeout: opts.Timeout,
		Dial:    dial,
		Direct:  opts.Direct,
	}, nil
}
