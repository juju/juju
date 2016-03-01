// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"github.com/juju/errors"
	"github.com/juju/utils"
	lxdshared "github.com/lxc/lxd/shared"

	"github.com/juju/juju/container/lxc"
)

const (
	// remoteLocalName is a specific remote name in the default LXD config.
	// See https://github.com/lxc/lxd/blob/master/config.go:defaultRemote.
	remoteLocalName  = "local"
	remoteIDForLocal = remoteLocalName
)

// Local is LXD's default "remote". Essentially it is an unencrypted,
// unauthenticated connection to localhost over a unix socket.
// However it does require users to be in the lxd group.
var Local = Remote{
	Name: remoteLocalName,
	Host: "", // If Host is empty we will translate it into the local Unix socket
	// No certificates are used when connecting to the Unix socket
	Cert:          nil,
	ServerPEMCert: "",
}

// Remote describes a LXD "remote" server for a client. In
// particular it holds the information needed for the client
// to connect to the remote.
type Remote struct {
	// Name is a label for this remote.
	Name string

	// Host identifies the host to which the client should connect.
	// An empty string is interpreted as:
	//   "localhost over a unix socket (unencrypted)".
	Host string

	// Cert holds the TLS certificate data for the client to use.
	Cert *Cert

	// ServerPEMCert is the certificate to be supplied as the acceptable
	// server certificate when connecting to the remote.
	ServerPEMCert string
}

// isLocal determines if the remote is the implicit "local" remote,
// an unencrypted, unauthenticated unix socket to a locally running LXD.
func (r Remote) isLocal() bool {
	return r.Host == Local.Host
}

// ID identifies the remote to the raw LXD client code. For the
// non-local case this is Remote.Name. For the local case it is the
// remote name that LXD special-cases for the local unix socket.
func (r Remote) ID() string {
	if r.isLocal() {
		return remoteIDForLocal
	}
	return r.Name
}

// WithDefaults updates a copy of the remote with default values
// where needed.
func (r Remote) WithDefaults() (Remote, error) {
	// Note that r is a value receiver, so it is an implicit copy.

	if r.isLocal() {
		return r.withLocalDefaults(), nil
	}

	if r.Cert == nil {
		certPEM, keyPEM, err := lxdshared.GenerateMemCert()
		if err != nil {
			return r, errors.Trace(err)
		}
		cert := NewCert(certPEM, keyPEM)
		r.Cert = &cert
	}

	cert, err := r.Cert.WithDefaults()
	if err != nil {
		return r, errors.Trace(err)
	}
	r.Cert = &cert

	return r, nil
}

func (r Remote) withLocalDefaults() Remote {
	if r.Name == "" {
		r.Name = remoteLocalName
	}

	// TODO(ericsnow) Set r.Cert to nil?

	return r
}

// Validate checks the Remote fields for invalid values.
func (r Remote) Validate() error {
	if r.Name == "" {
		return errors.NotValidf("remote missing name,")
	}

	if r.isLocal() {
		if err := r.validateLocal(); err != nil {
			return errors.Trace(err)
		}
		return nil
	}

	// TODO(ericsnow) Ensure the host is a valid hostname or address?

	if r.Cert == nil {
		return errors.NotValidf("remote without cert")
	} else if err := r.Cert.Validate(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (r Remote) validateLocal() error {
	if r.Cert != nil {
		return errors.NotValidf("hostless remote with cert")
	}

	return nil
}

// UsingTCP converts the remote into a non-local version. For
// non-local remotes this is a no-op.
//
// For a "local" remote (see Local), the remote is changed to a one
// with the host set to the IP address of the local lxcbr0 bridge
// interface. The remote is also set up for remote access, setting
// the cert if not already set.
func (r Remote) UsingTCP() (Remote, error) {
	// Note that r is a value receiver, so it is an implicit copy.

	if !r.isLocal() {
		return r, nil
	}

	// TODO: jam 2016-02-25 This should be updated for systems that are
	// 	 space aware, as we may not be just using the default LXC
	// 	 bridge.
	netIF := lxc.DefaultLxcBridge
	addr, err := utils.GetAddressForInterface(netIF)
	if err != nil {
		return r, errors.Trace(err)
	}
	r.Host = addr

	r, err = r.WithDefaults()
	if err != nil {
		return r, errors.Trace(err)
	}

	// TODO(ericsnow) Change r.Name if "local"? Prepend "juju-"?

	return r, nil
}

// TODO(ericsnow) Add a "Connect(Config)" method that connects
// to the remote and returns the corresponding Client.

// TODO(ericsnow) Add a "Register" method to Client that adds the remote
// to the client's remote?
