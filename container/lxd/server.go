// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/errors"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/os"
	"github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared"
	"github.com/lxc/lxd/shared/api"
)

// osSupport is the list of operating system types for which Juju supports
// communicating with LXD via a local socket, by default.
var osSupport = []os.OSType{os.Ubuntu}

// HasSupport returns true if the current OS supports LXD containers by
// default
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

	name              string
	clustered         bool
	serverCertificate string
	hostArch          string

	networkAPISupport bool
	clusterAPISupport bool
	storageAPISupport bool

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

// NewRemoteServer returns a Server based on a remote connection.
func NewRemoteServer(spec ServerSpec) (*Server, error) {
	if err := spec.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	// Skip the get, because we know that we're going to request it
	// when calling new server, preventing the double request.
	spec.connectionArgs.SkipGetServer = true
	cSvr, err := ConnectRemote(spec)
	if err != nil {
		return nil, errors.Trace(err)
	}
	svr, err := NewServer(cSvr)
	return svr, err
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
	if name == "" && !clustered {
		// If the name is set to empty and clustering is false, then it's highly
		// likely that we're on an older version of LXD. So in that case we
		// need to set the name to something and internally LXD sets this type
		// of node to "none".
		// LP:#1786309
		name = "none"
	}
	serverCertificate := info.Environment.Certificate
	hostArch := arch.NormaliseArch(info.Environment.KernelArchitecture)

	return &Server{
		ContainerServer:   svr,
		name:              name,
		clustered:         clustered,
		serverCertificate: serverCertificate,
		hostArch:          hostArch,
		networkAPISupport: shared.StringInSlice("network", apiExt),
		clusterAPISupport: shared.StringInSlice("clustering", apiExt),
		storageAPISupport: shared.StringInSlice("storage", apiExt),
	}, nil
}

// Name returns the name of this LXD server.
func (s *Server) Name() string {
	return s.name
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

// GetContainerProfiles returns the list of profiles that are assocated with a
// container.
func (s *Server) GetContainerProfiles(name string) ([]string, error) {
	container, _, err := s.GetContainer(name)
	if err != nil {
		return []string{}, errors.Trace(err)
	}
	return container.Profiles, nil
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

// HasProfile interrogates the known profile names and returns a boolean
// indicating whether a profile with the input name exists.
func (s *Server) HasProfile(name string) (bool, error) {
	profiles, err := s.GetProfileNames()
	if err != nil {
		return false, errors.Trace(err)
	}
	for _, profile := range profiles {
		if profile == name {
			return true, nil
		}
	}
	return false, nil
}

// CreateProfileWithConfig creates a new profile with the input name and config.
func (s *Server) CreateProfileWithConfig(name string, cfg map[string]string) error {
	req := api.ProfilesPost{
		Name: name,
		ProfilePut: api.ProfilePut{
			Config: cfg,
		},
	}
	return errors.Trace(s.CreateProfile(req))
}

// ServerCertificate returns the current server environment certificate
func (s *Server) ServerCertificate() string {
	return s.serverCertificate
}

// HostArch returns the current host architecture
func (s *Server) HostArch() string {
	return s.hostArch
}

// IsLXDNotFound checks if an error from the LXD API indicates that a requested
// entity was not found.
func IsLXDNotFound(err error) bool {
	return err != nil && err.Error() == "not found"
}
