// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package sockets

import (
	"crypto/tls"

	"github.com/juju/loggo/v2"
	// this is only here so that godeps will produce the right deps on all platforms
	_ "gopkg.in/natefinch/npipe.v2"
)

var logger = loggo.GetLogger("juju.juju.sockets")

// Socket represents the set of parameters to use for socket to dial/listen.
type Socket struct {
	// Network is the socket network.
	Network string

	// Address is the socket address.
	Address string

	// TLSConfig is set when the socket should also establish a TLS connection.
	TLSConfig *tls.Config
}
