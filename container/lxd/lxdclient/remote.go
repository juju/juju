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
var Local = NewRemote(RemoteInfo{
	Name: remoteLocalName,
})

// RemoteInfo describes a LXD "remote" server for a client. In
// particular it holds the information needed for the client
// to connect to the remote.
type RemoteInfo struct {
	// Name is a label for this remote.
	Name string

	// Host identifies the host to which the client should connect.
	// An empty string is interpreted as:
	//   "localhost over a unix socket (unencrypted)".
	Host string

	// Cert holds the TLS certificate data for the client to use.
	Cert *Certificate
}

// SetDefaults updates a copy of the remote with default values
// where needed.
func (ri RemoteInfo) SetDefaults() (RemoteInfo, error) {
	if ri.Host == "" {
		return ri.setLocalDefaults(), nil
	}

	if ri.Cert == nil {
		cert, err := GenerateCertificate(genCertAndKey)
		if err != nil {
			return ri, errors.Trace(err)
		}
		ri.Cert = cert
	}

	return ri, nil
}

func (ri RemoteInfo) setLocalDefaults() RemoteInfo {
	if ri.Name == "" {
		ri.Name = remoteLocalName
	}

	// TODO(ericsnow) Set ri.Cert to nil?

	return ri
}

// Validate checks the RemoteInfo fields for invalid values.
func (ri RemoteInfo) Validate() error {
	if ri.Name == "" {
		return errors.NotValidf("remote missing name,")
	}

	if ri.Host == "" {
		if err := ri.validateLocal(); err != nil {
			return errors.Trace(err)
		}
		return nil
	}

	// TODO(ericsnow) Ensure the host is a valid hostname or address?

	if ri.Cert == nil {
		return errors.NotValidf("remote without cert")
	} else if err := ri.Cert.Validate(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (ri RemoteInfo) validateLocal() error {
	if ri.Cert != nil {
		return errors.NotValidf("hostless remote with cert")
	}

	return nil
}

// Remote represents a single LXD remote server.
type Remote struct {
	info RemoteInfo
}

// NewRemote builds a new Remote from the provided info.
func NewRemote(info RemoteInfo) Remote {
	return Remote{
		info: info,
	}
}

// setDefaults updates a copy of the remote with default values
// where needed.
func (r Remote) setDefaults() (Remote, error) {
	info, err := r.info.SetDefaults()
	if err != nil {
		return r, errors.Trace(err)
	}
	r.info = info

	return r, nil
}

// Validate ensures that the Remote is valid.
func (r Remote) Validate() error {
	if err := r.info.Validate(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// ID identifies the remote to the raw LXD client code.
func (r Remote) ID() string {
	if r.info.Host == "" {
		return remoteIDForLocal
	}
	return r.info.Name
}

// Cert returns the certificate the client should use.
func (r Remote) Cert() Certificate {
	if r.info.Cert == nil {
		return Certificate{}
	}
	return *r.info.Cert
}

// TODO(ericsnow) Add a "Connect(Config)" method that connects
// to the remote and returns the corresponding Client.

// TODO(ericsnow) Add a "Register" method to Client that adds the remote
// to the client's remote?
