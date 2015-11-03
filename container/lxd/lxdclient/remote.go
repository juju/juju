// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdclient

import (
	"github.com/juju/errors"
)

const (
	remoteLocalName   = "local"
	remoteDefaultName = remoteLocalName

	// TODO(ericsnow) This may be changing to "local"
	remoteIDForLocal = ""
)

// Local is LXD's default "remote". Essentially it is an unencrypted,
// unauthenticated connection to localhost over a unix socket.
var Local = Remote{
	Name: remoteLocalName,
	Host: "", // The LXD API turns this into the local unix socket.
	Cert: nil,
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
}

// isLocal determines if the remote is the implicit "local" remote,
// an unencrypted, unauthenticated unix socket to a locally running LXD.
func (r Remote) isLocal() bool {
	if Local.Host != "" {
		logger.Errorf("%#v", Local)
	}
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

// SetDefaults updates a copy of the remote with default values
// where needed.
func (r Remote) SetDefaults() (Remote, error) {
	if r.isLocal() {
		return r.setLocalDefaults(), nil
	}

	if r.Cert == nil {
		certPEM, keyPEM, err := genCertAndKey()
		if err != nil {
			return r, errors.Trace(err)
		}
		r.Cert = NewCert(certPEM, keyPEM)
	}

	return r, nil
}

func (r Remote) setLocalDefaults() Remote {
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

// TODO(ericsnow) Add a "Connect(Config)" method that connects
// to the remote and returns the corresponding Client.

// TODO(ericsnow) Add a "Register" method to Client that adds the remote
// to the client's remote?
