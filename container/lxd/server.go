// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"net/http"
	"strings"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"github.com/juju/clock"
	"github.com/juju/errors"

	"github.com/juju/juju/core/arch"
)

// Server extends the upstream LXD container server.
type Server struct {
	lxd.InstanceServer

	name              string
	clustered         bool
	serverCertificate string
	hostArch          string
	supportedArches   []string
	serverVersion     string

	networkAPISupport bool
	clusterAPISupport bool
	storageAPISupport bool

	localBridgeName string

	clock clock.Clock
}

// NewLocalServer returns a Server based on a local socket connection.
func NewLocalServer() (*Server, error) {
	cSvr, err := connectLocal()
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
func NewServer(svr lxd.InstanceServer) (*Server, error) {
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
	supportedArches := []string{}
	for _, entry := range info.Environment.Architectures {
		supportedArches = append(supportedArches, arch.NormaliseArch(entry))
	}
	if len(supportedArches) == 0 {
		supportedArches = []string{hostArch}
	}

	return &Server{
		InstanceServer:    svr,
		name:              name,
		clustered:         clustered,
		serverCertificate: serverCertificate,
		hostArch:          hostArch,
		supportedArches:   supportedArches,
		networkAPISupport: inSlice("network", apiExt),
		clusterAPISupport: inSlice("clustering", apiExt),
		storageAPISupport: inSlice("storage", apiExt),
		serverVersion:     info.Environment.ServerVersion,
		clock:             clock.WallClock,
	}, nil
}

// Name returns the name of this LXD server.
func (s *Server) Name() string {
	return s.name
}

func (s *Server) ServerVersion() string {
	return s.serverVersion
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
	container, eTag, err := s.GetInstance(name)
	if err != nil {
		return errors.Trace(err)
	}
	if container.Config == nil {
		container.Config = make(map[string]string)
	}
	for k, v := range cfg {
		container.Config[k] = v
	}

	resp, err := s.UpdateInstance(name, container.Writable(), eTag)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(resp.Wait())
}

// GetContainerProfiles returns the list of profiles that are associated with a
// container.
func (s *Server) GetContainerProfiles(name string) ([]string, error) {
	container, _, err := s.GetInstance(name)
	if err != nil {
		return []string{}, errors.Trace(err)
	}
	return container.Profiles, nil
}

// UseProject ensures that this server will use the input project.
// See: https://documentation.ubuntu.com/lxd/en/latest/projects.
func (s *Server) UseProject(project string) {
	s.InstanceServer = s.InstanceServer.UseProject(project)
}

// ReplaceOrAddContainerProfile updates the profiles for the container with the
// input name, using the input values.
// TODO: HML 2-apr-2019
// remove when provisioner_task processProfileChanges() is
// removed.
func (s *Server) ReplaceOrAddContainerProfile(name, oldProfile, newProfile string) error {
	container, eTag, err := s.GetInstance(name)
	if err != nil {
		return errors.Trace(errors.Annotatef(err, "failed to get container %q", name))
	}
	profiles := addRemoveReplaceProfileName(container.Profiles, oldProfile, newProfile)

	container.Profiles = profiles
	resp, err := s.UpdateInstance(name, container.Writable(), eTag)
	if err != nil {
		return errors.Trace(errors.Annotatef(err, "failed to updated container %q", name))
	}

	op := resp.Get()
	logger.Debugf("updated container, waiting on %s", op.Description)
	err = resp.Wait()
	if err != nil {
		logger.Tracef("updating container failed on %q", err)
	}
	return errors.Trace(err)
}

func addRemoveReplaceProfileName(profiles []string, oldProfile, newProfile string) []string {
	if oldProfile == "" {
		// add profile
		profiles = append(profiles, newProfile)
	} else {
		for i, pName := range profiles {
			if pName == oldProfile {
				if newProfile == "" {
					// remove profile
					profiles = append(profiles[:i], profiles[i+1:]...)
				} else {
					// replace profile
					profiles[i] = newProfile
				}
				break
			}
		}
	}
	return profiles
}

// UpdateContainerProfiles applies the given profiles (by name) to the
// named container.  It is assumed the profiles have all been added to
// the server before hand.
func (s *Server) UpdateContainerProfiles(name string, profiles []string) error {
	container, eTag, err := s.GetInstance(name)
	if err != nil {
		return errors.Trace(errors.Annotatef(err, "failed to get %q", name))
	}

	container.Profiles = profiles
	resp, err := s.UpdateInstance(name, container.Writable(), eTag)
	if err != nil {
		return errors.Trace(errors.Annotatef(err, "failed to update %q with profiles", name))
	}

	op := resp.Get()
	logger.Debugf("updated %q profiles, waiting on %s", name, op.Description)
	err = resp.Wait()
	return errors.Trace(errors.Annotatef(err, "update failed"))
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

// SupportedArches returns all supported arches
func (s *Server) SupportedArches() []string {
	return s.supportedArches
}

// IsLXDNotFound checks if an error from the LXD API indicates that a requested
// entity was not found.
func IsLXDNotFound(err error) bool {
	if err == nil {
		return false
	}

	if _, match := api.StatusErrorMatch(err, http.StatusNotFound); match {
		return true
	}

	return strings.Contains(strings.ToLower(err.Error()), "not found")
}

// IsLXDAlreadyExists checks if an error from the LXD API indicates that a
// requested entity already exists.
func IsLXDAlreadyExists(err error) bool {
	if err == nil {
		return false
	}

	if _, match := api.StatusErrorMatch(err, http.StatusConflict); match {
		return true
	}

	return strings.Contains(strings.ToLower(err.Error()), "already exists")
}

func inSlice[T comparable](key T, list []T) bool {
	for _, entry := range list {
		if entry == key {
			return true
		}
	}
	return false
}
