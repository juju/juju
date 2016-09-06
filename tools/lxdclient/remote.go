// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"github.com/juju/errors"
	lxdshared "github.com/lxc/lxd/shared"
)

const (
	// remoteLocalName is a specific remote name in the default LXD config.
	// See https://github.com/lxc/lxd/blob/master/config.go:DefaultRemotes.
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
	Protocol:      LXDProtocol,
	Cert:          nil,
	ServerPEMCert: "",
}

type Protocol string

const (
	LXDProtocol           Protocol = "lxd"
	SimplestreamsProtocol Protocol = "simplestreams"
)

var CloudImagesRemote = Remote{
	Name:          "cloud-images.ubuntu.com",
	Host:          "https://cloud-images.ubuntu.com/releases",
	Protocol:      SimplestreamsProtocol,
	Cert:          nil,
	ServerPEMCert: "",
}

var generateCertificate = lxdshared.GenerateMemCert
var DefaultImageSources = []Remote{CloudImagesRemote}

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

	// Protocol indicates whether this Remote is accessed via the normal
	// "LXD" protocol, or whether it is a Simplestreams source. The value
	// is only useful for Remotes that are image sources
	Protocol Protocol

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

	if r.Protocol == "" {
		r.Protocol = LXDProtocol
	}

	if r.Cert == nil {
		certPEM, keyPEM, err := generateCertificate()
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
	if r.Protocol == "" {
		r.Protocol = LXDProtocol
	}

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

	if r.Protocol == "" {
		return errors.NotValidf("missing Protocol")
	}
	if r.Protocol != LXDProtocol && r.Protocol != SimplestreamsProtocol {
		return errors.NotValidf("unknown Protocol %q", r.Protocol)
	}

	// r.Cert is allowed to be nil for Public remotes
	if r.Cert != nil {
		if err := r.Cert.Validate(); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

func (r Remote) validateLocal() error {
	if r.Cert != nil {
		return errors.NotValidf("hostless remote with cert")
	}
	if r.Protocol != LXDProtocol {
		return errors.NotValidf("localhost always talks LXD protocol not: %s", r.Protocol)
	}

	return nil
}
