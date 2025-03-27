// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	lxdclient "github.com/canonical/lxd/client"
	lxdapi "github.com/canonical/lxd/shared/api"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"github.com/juju/utils/v4"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/container/lxd"
	proxy "github.com/juju/juju/internal/proxy/config"
)

// Server defines an interface of all localized methods that the environment
// and provider utilizes.
type Server interface {
	FindImage(context.Context, corebase.Base, string, instance.VirtType, []lxd.ServerSpec, bool, environs.StatusCallbackFunc) (lxd.SourcedImage, error)
	GetServer() (server *lxdapi.Server, ETag string, err error)
	ServerVersion() string
	GetConnectionInfo() (info *lxdclient.ConnectionInfo, err error)
	UpdateServerConfig(map[string]string) error
	UpdateContainerConfig(string, map[string]string) error
	CreateCertificate(lxdapi.CertificatesPost) error
	GetCertificate(fingerprint string) (certificate *lxdapi.Certificate, ETag string, err error)
	DeleteCertificate(fingerprint string) (err error)
	CreateClientCertificate(certificate *lxd.Certificate) error
	LocalBridgeName() string
	AliveContainers(prefix string) ([]lxd.Container, error)
	ContainerAddresses(name string) ([]network.ProviderAddress, error)
	RemoveContainer(name string) error
	RemoveContainers(names []string) error
	FilterContainers(prefix string, statuses ...string) ([]lxd.Container, error)
	CreateContainerFromSpec(spec lxd.ContainerSpec) (*lxd.Container, error)
	WriteContainer(*lxd.Container) error
	CreateProfileWithConfig(string, map[string]string) error
	GetProfile(string) (*lxdapi.Profile, string, error)
	GetContainerProfiles(string) ([]string, error)
	HasProfile(string) (bool, error)
	CreateProfile(post lxdapi.ProfilesPost) (err error)
	DeleteProfile(string) (err error)
	ReplaceOrAddContainerProfile(string, string, string) error
	UpdateContainerProfiles(name string, profiles []string) error
	VerifyNetworkDevice(*lxdapi.Profile, string) error
	EnsureDefaultStorage(*lxdapi.Profile, string) error
	StorageSupported() bool
	GetStoragePool(name string) (pool *lxdapi.StoragePool, ETag string, err error)
	GetStoragePools() (pools []lxdapi.StoragePool, err error)
	CreatePool(name, driver string, attrs map[string]string) error
	GetStoragePoolVolume(pool string, volType string, name string) (*lxdapi.StorageVolume, string, error)
	GetStoragePoolVolumes(pool string) (volumes []lxdapi.StorageVolume, err error)
	CreateVolume(pool, name string, config map[string]string) error
	UpdateStoragePoolVolume(pool string, volType string, name string, volume lxdapi.StorageVolumePut, ETag string) error
	DeleteStoragePoolVolume(pool string, volType string, name string) (err error)
	ServerCertificate() string
	HostArch() string
	SupportedArches() []string
	EnableHTTPSListener() error
	GetNICsFromProfile(profName string) (map[string]map[string]string, error)
	IsClustered() bool
	UseTargetServer(name string) (*lxd.Server, error)
	GetClusterMembers() (members []lxdapi.ClusterMember, err error)
	Name() string
	HasExtension(extension string) (exists bool)
	GetNetworks() ([]lxdapi.Network, error)
	GetNetworkState(name string) (*lxdapi.NetworkState, error)
	GetInstance(name string) (*lxdapi.Instance, string, error)
	GetInstanceState(name string) (*lxdapi.InstanceState, string, error)

	// UseProject ensures that this server will use the input project.
	// See: https://documentation.ubuntu.com/lxd/en/latest/projects.
	UseProject(string)
}

// CloudSpec describes the cloud configuration for use with the LXD provider.
type CloudSpec struct {
	environscloudspec.CloudSpec

	// Project specifies the LXD project to target.
	Project string
}

// ServerFactory creates a new factory for creating servers that are required
// by the server.
type ServerFactory interface {
	// LocalServer creates a new lxd server and augments and wraps the lxd
	// server, by ensuring sane defaults exist with network, storage.
	LocalServer() (Server, error)

	// LocalServerAddress returns the local servers address from the factory.
	LocalServerAddress() (string, error)

	// RemoteServer creates a new server that connects to a remote lxd server.
	// If the cloudSpec endpoint is nil or empty, it will assume that you want
	// to connection to a local server and will instead use that one.
	RemoteServer(CloudSpec) (Server, error)

	// InsecureRemoteServer creates a new server that connect to a remote lxd
	// server in a insecure manner.
	// If the cloudSpec endpoint is nil or empty, it will assume that you want
	// to connection to a local server and will instead use that one.
	InsecureRemoteServer(CloudSpec) (Server, error)
}

// InterfaceAddress groups methods that is required to find addresses
// for a given interface
type InterfaceAddress interface {

	// InterfaceAddress looks for the network interface
	// and returns the IPv4 address from the possible addresses.
	// Returns an error if there is an issue locating the interface name or
	// the address associated with it.
	InterfaceAddress(string) (string, error)
}

type interfaceAddress struct{}

func (interfaceAddress) InterfaceAddress(interfaceName string) (string, error) {
	return utils.GetAddressForInterface(interfaceName)
}

// NewHTTPClientFunc is responsible for generating a new http client every time
// it is called.
type NewHTTPClientFunc func() *http.Client

type serverFactory struct {
	newLocalServerFunc  func() (Server, error)
	newRemoteServerFunc func(lxd.ServerSpec) (Server, error)
	localServer         Server
	localServerAddress  string
	interfaceAddress    InterfaceAddress
	clock               clock.Clock
	mutex               sync.Mutex
	newHTTPClientFunc   NewHTTPClientFunc
}

// NewServerFactory creates a new ServerFactory with sane defaults.
// A NewHTTPClientFunc is taken as an argument to address LP2003135. Previously
// we reused the same http client for all LXD connections. This can't happen
// as the LXD client code modifies the HTTP server.
func NewServerFactory(newHttpFn NewHTTPClientFunc) ServerFactory {
	return &serverFactory{
		newLocalServerFunc: func() (Server, error) {
			return lxd.NewLocalServer()
		},
		newRemoteServerFunc: func(spec lxd.ServerSpec) (Server, error) {
			return lxd.NewRemoteServer(spec)
		},
		interfaceAddress:  interfaceAddress{},
		newHTTPClientFunc: newHttpFn,
	}
}

func (s *serverFactory) LocalServer() (Server, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// We have an instantiated localServer, that we can reuse over and over.
	if s.localServer != nil {
		return s.localServer, nil
	}

	// initialize a new local server
	svr, err := s.initLocalServer()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// bootstrap a new local server, this ensures that all connections to and
	// from the local server are connected and setup correctly.
	var hostName string
	svr, hostName, err = s.bootstrapLocalServer(context.TODO(), svr)
	if err == nil {
		s.localServer = svr
		s.localServerAddress = hostName
	}
	return svr, errors.Trace(err)
}

func (s *serverFactory) LocalServerAddress() (string, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.localServer == nil {
		return "", errors.NotAssignedf("local server")
	}

	return s.localServerAddress, nil
}

func (s *serverFactory) RemoteServer(spec CloudSpec) (Server, error) {
	if spec.Endpoint == "" {
		return s.LocalServer()
	}

	cred := spec.Credential
	if cred == nil {
		return nil, errors.NotFoundf("credentials")
	}

	clientCert, serverCert, ok := getCertificates(*cred)
	if !ok {
		return nil, errors.NotValidf("credentials")
	}

	serverSpec := lxd.NewServerSpec(spec.Endpoint, serverCert, clientCert).
		WithProxy(proxy.DefaultConfig.GetProxy).
		WithHTTPClient(s.newHTTPClientFunc())

	svr, err := s.newRemoteServerFunc(serverSpec)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if spec.Project != "" {
		svr.UseProject(spec.Project)
	}

	return svr, errors.Trace(s.bootstrapRemoteServer(context.TODO(), svr))
}

func (s *serverFactory) InsecureRemoteServer(spec CloudSpec) (Server, error) {
	if spec.Endpoint == "" {
		return s.LocalServer()
	}

	cred := spec.Credential
	if cred == nil {
		return nil, errors.NotFoundf("credentials")
	}

	clientCert, ok := getClientCertificates(*cred)
	if !ok {
		return nil, errors.NotValidf("credentials")
	}

	serverSpec := lxd.NewInsecureServerSpec(spec.Endpoint).
		WithClientCertificate(clientCert).
		WithSkipGetServer(true).
		WithHTTPClient(s.newHTTPClientFunc())

	svr, err := s.newRemoteServerFunc(serverSpec)
	if spec.Project != "" {
		svr.UseProject(spec.Project)
	}

	return svr, errors.Trace(err)
}

func (s *serverFactory) initLocalServer() (Server, error) {
	svr, err := s.newLocalServerFunc()
	if err != nil {
		return nil, errors.Trace(hoistLocalConnectErr(err))
	}

	defaultProfile, profileETag, err := svr.GetProfile("default")
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := svr.VerifyNetworkDevice(defaultProfile, profileETag); err != nil {
		return nil, errors.Trace(err)
	}

	// LXD itself reports the host:ports that it listens on.
	// Cross-check the address we have with the values reported by LXD.
	if err := svr.EnableHTTPSListener(); err != nil {
		return nil, errors.Annotate(err, "enabling HTTPS listener")
	}
	return svr, nil
}

func (s *serverFactory) bootstrapLocalServer(ctx context.Context, svr Server) (Server, string, error) {
	// select the server bridge name, so that we can then try and select
	// the hostAddress from the current interfaceAddress
	bridgeName := svr.LocalBridgeName()
	hostAddress, err := s.interfaceAddress.InterfaceAddress(bridgeName)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	hostAddress = lxd.EnsureHTTPS(hostAddress)

	// The following retry mechanism is required for newer LXD versions, where
	// the new lxd client doesn't propagate the EnableHTTPSListener quick enough
	// to get the addresses or on the same existing local provider.

	// connInfoAddresses is really useful for debugging, so let's keep that
	// information around for the debugging errors.
	var connInfoAddresses []string
	errNotExists := errors.New("not-exists")
	retryArgs := retry.CallArgs{
		Clock: s.Clock(),
		IsFatalError: func(err error) bool {
			return errors.Cause(err) != errNotExists
		},
		Func: func() error {
			cInfo, err := svr.GetConnectionInfo()
			if err != nil {
				return errors.Trace(err)
			}

			connInfoAddresses = cInfo.Addresses
			for _, addr := range cInfo.Addresses {
				if strings.HasPrefix(addr, hostAddress+":") {
					hostAddress = addr
					return nil
				}
			}

			// Requesting a NewLocalServer forces a new connection, so that when
			// we GetConnectionInfo it gets the required addresses.
			// Note: this modifies the outer svr server.
			if svr, err = s.initLocalServer(); err != nil {
				return errors.Trace(err)
			}

			return errNotExists
		},
		Delay:    2 * time.Second,
		Attempts: 30,
	}
	if err := retry.Call(retryArgs); err != nil {
		return nil, "", errors.Errorf(
			"LXD is not listening on address %s (reported addresses: %s)",
			hostAddress, connInfoAddresses,
		)
	}

	// If the server is not a simple simple stream server, don't check the
	// API version, but do report for other scenarios
	if err := s.validateServer(ctx, svr); err != nil {
		return nil, "", errors.Trace(err)
	}

	return svr, hostAddress, nil
}

func (s *serverFactory) bootstrapRemoteServer(ctx context.Context, svr Server) error {
	err := s.validateServer(ctx, svr)
	return errors.Trace(err)
}

func (s *serverFactory) validateServer(ctx context.Context, svr Server) error {
	// If the storage API is supported, let's make sure the LXD has a
	// default pool; we'll just use dir backend for now.
	if svr.StorageSupported() {
		// Ensure that the default profile has a network configuration that will
		// allow access to containers that we create.
		profile, eTag, err := svr.GetProfile("default")
		if err != nil {
			return errors.Trace(err)
		}

		if err := svr.EnsureDefaultStorage(profile, eTag); err != nil {
			return errors.Trace(err)
		}
	}

	apiVersion := svr.ServerVersion()
	err := ValidateAPIVersion(apiVersion)
	if errors.Is(err, errors.NotSupported) {
		return errors.Trace(err)
	}
	if err != nil {
		logger.Warningf(ctx, err.Error())
		logger.Warningf(ctx, "trying to use unsupported LXD API version %q", apiVersion)
	} else {
		logger.Tracef(ctx, "using LXD API version %q", apiVersion)
	}

	return nil
}

func (s *serverFactory) Clock() clock.Clock {
	if s.clock == nil {
		return clock.WallClock
	}
	return s.clock
}

// parseAPIVersion parses the LXD API version string.
func parseAPIVersion(s string) (semversion.Number, error) {
	versionParts := strings.Split(s, ".")
	if len(versionParts) < 2 {
		return semversion.Zero, errors.NewNotValid(nil, fmt.Sprintf("LXD API version %q: expected format <major>.<minor>", s))
	}
	major, err := strconv.Atoi(versionParts[0])
	if err != nil {
		return semversion.Zero, errors.NotValidf("major version number  %v", versionParts[0])
	}
	minor, err := strconv.Atoi(versionParts[1])
	if err != nil {
		return semversion.Zero, errors.NotValidf("minor version number  %v", versionParts[1])
	}
	return semversion.Number{Major: major, Minor: minor}, nil
}

// minLXDVersion defines the min version of LXD we support.
var minLXDVersion = semversion.Number{Major: 5, Minor: 0}

// ValidateAPIVersion validates the LXD version.
func ValidateAPIVersion(version string) error {
	ver, err := parseAPIVersion(version)
	if err != nil {
		return err
	}
	logger.Tracef(context.TODO(), "current LXD version %q, min LXD version %q", ver, minLXDVersion)
	if ver.Compare(minLXDVersion) < 0 {
		return errors.NewNotSupported(nil,
			fmt.Sprintf("LXD version has to be at least %q, but current version is only %q", minLXDVersion, ver),
		)
	}
	return nil
}

func getMessageFromErr(err error) (bool, string) {
	msg := err.Error()
	t, ok := errors.Cause(err).(*url.Error)
	if !ok {
		return false, msg
	}

	u, ok := t.Err.(*net.OpError)
	if !ok {
		return false, msg
	}

	if u.Op == "dial" && u.Net == "unix" {
		var lxdErr error

		sysErr, ok := u.Err.(*os.SyscallError)
		if ok {
			lxdErr = sysErr.Err
		} else {
			// Try a syscall.Errno as that is what's returned for CentOS
			errno, ok := u.Err.(syscall.Errno)
			if !ok {
				return false, msg
			}
			lxdErr = errno
		}

		switch lxdErr {
		case syscall.ENOENT:
			return false, "LXD socket not found; is LXD installed & running?"
		case syscall.ECONNREFUSED:
			return true, "LXD refused connections; is LXD running?"
		case syscall.EACCES:
			return true, "Permission denied, are you in the lxd group?"
		}
	}

	return false, msg
}

func hoistLocalConnectErr(err error) error {
	installed, msg := getMessageFromErr(err)

	configureText := `
Please configure LXD by running:
	$ newgrp lxd
	$ lxd init
`

	installText := `
Please install LXD by running:
	$ sudo snap install lxd
and then configure it with:
	$ newgrp lxd
	$ lxd init
`

	hint := installText
	if installed {
		hint = configureText
	}

	return errors.Trace(fmt.Errorf("%s\n%s", msg, hint))
}
