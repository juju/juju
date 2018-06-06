// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/errors"
	"github.com/juju/utils/os"
	"github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared"
)

// osSupport is the list of operating system types for which Juju supports
// communicating with LXD via a local socket, by default.
var osSupport = []os.OSType{os.Ubuntu}

// HasSupport defines if the provider has support for a host OS
func HasSupport() bool {
	t := os.HostOS()
	for _, v := range osSupport {
		if v == t {
			return true
		}
	}
	return false
}

// Server extends the upstream LXD container server.
type Server struct {
	lxd.ContainerServer

	name      string
	clustered bool

	networkAPISupport bool
	clusterAPISupport bool

	localBridgeName string
}

// MaybeNewLocalServer returns a Server based on a local socket connection,
// if running on an OS supporting LXD containers by default.
// Otherwise a nil server is returned.
func MaybeNewLocalServer() (*Server, error) {
	if !HasSupport() {
		return nil, nil
	}
	svr, err := NewLocalServer()
	return svr, errors.Trace(err)
}

// NewLocalServer returns a Server based on a local socket connection.
func NewLocalServer() (*Server, error) {
	cSvr, err := ConnectLocal()
	if err != nil {
		return nil, errors.Trace(err)
	}
	svr, err := NewServer(cSvr)
	return svr, errors.Trace(err)
}

// NewServer builds and returns a Server for high-level interaction with the
// input LXD container server.
func NewServer(svr lxd.ContainerServer) (*Server, error) {
	info, _, err := svr.GetServer()
	if err != nil {
		return nil, errors.Trace(err)
	}

	apiExt := info.APIExtensions

	name := info.Environment.ServerName
	clustered := info.Environment.ServerClustered
	if clustered {
		logger.Debugf("creating LXD server for cluster node %q", name)
	}

	return &Server{
		ContainerServer:   svr,
		name:              name,
		clustered:         clustered,
		networkAPISupport: shared.StringInSlice("network", apiExt),
		clusterAPISupport: shared.StringInSlice("clustering", apiExt),
	}, nil
}

// UpdateServerConfig updates the server configuration with the input values.
func (s *Server) UpdateServerConfig(cfg map[string]string) error {
	svr, eTag, err := s.GetServer()
	if err != nil {
		return errors.Trace(err)
	}
	if svr.Config == nil {
		svr.Config = make(map[string]interface{})
	}
	for k, v := range cfg {
		svr.Config[k] = v
	}
	return errors.Trace(s.UpdateServer(svr.Writable(), eTag))
}

// UpdateContainerConfig updates the configuration for the container with the
// input name, using the input values.
func (s *Server) UpdateContainerConfig(name string, cfg map[string]string) error {
	container, eTag, err := s.GetContainer(name)
	if err != nil {
		return errors.Trace(err)
	}
	if container.Config == nil {
		container.Config = make(map[string]string)
	}
	for k, v := range cfg {
		container.Config[k] = v
	}

	resp, err := s.UpdateContainer(name, container.Writable(), eTag)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(resp.Wait())
}

// CreateClientCertificate adds the input certificate to the server,
// indicating that is for use in client communication.
func (s *Server) CreateClientCertificate(cert *Certificate) error {
	req, err := cert.AsCreateRequest()
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(s.CreateCertificate(req))
}

// IsLXDNotFound checks if an error from the LXD API indicates that a requested
// entity was not found.
func IsLXDNotFound(err error) bool {
	return err != nil && err.Error() == "not found"
}
