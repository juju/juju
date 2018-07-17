// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/juju/errors"
	"github.com/juju/retry"
	"github.com/juju/utils"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/container/lxd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
	"github.com/juju/juju/utils/proxy"

	lxdclient "github.com/lxc/lxd/client"
	lxdapi "github.com/lxc/lxd/shared/api"
)

// Server defines an interface of all localized methods that the environment
// and provider utilizes.
//go:generate mockgen -package lxd -destination server_mock_test.go github.com/juju/juju/provider/lxd Server,ServerFactory,InterfaceAddress
type Server interface {
	FindImage(string, string, []lxd.ServerSpec, bool, environs.StatusCallbackFunc) (lxd.SourcedImage, error)
	GetServer() (server *lxdapi.Server, ETag string, err error)
	GetConnectionInfo() (info *lxdclient.ConnectionInfo, err error)
	UpdateServerConfig(map[string]string) error
	UpdateContainerConfig(string, map[string]string) error
	CreateCertificate(lxdapi.CertificatesPost) error
	GetCertificate(fingerprint string) (certificate *lxdapi.Certificate, ETag string, err error)
	DeleteCertificate(fingerprint string) (err error)
	CreateClientCertificate(certificate *lxd.Certificate) error
	LocalBridgeName() string
	AliveContainers(prefix string) ([]lxd.Container, error)
	ContainerAddresses(name string) ([]network.Address, error)
	RemoveContainer(name string) error
	RemoveContainers(names []string) error
	FilterContainers(prefix string, statuses ...string) ([]lxd.Container, error)
	CreateContainerFromSpec(spec lxd.ContainerSpec) (*lxd.Container, error)
	WriteContainer(*lxd.Container) error
	CreateProfileWithConfig(string, map[string]string) error
	GetProfile(string) (*lxdapi.Profile, string, error)
	HasProfile(string) (bool, error)
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
	EnableHTTPSListener() error
	GetNICsFromProfile(profName string) (map[string]map[string]string, error)
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
	RemoteServer(environs.CloudSpec) (Server, error)

	// InsecureRemoteServer creates a new server that connect to a remote lxd
	// server in a insecure mannor.
	// If the cloudSpec endpoint is nil or empty, it will assume that you want
	// to connection to a local server and will instead use that one.
	InsecureRemoteServer(environs.CloudSpec) (Server, error)
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

type serverFactory struct {
	newLocalServerFunc  func() (Server, error)
	newRemoteServerFunc func(lxd.ServerSpec) (Server, error)
	localServer         Server
	localServerAddress  string
	interfaceAddress    InterfaceAddress
	clock               clock.Clock
	mutex               sync.Mutex
}

// NewServerFactory creates a new ServerFactory with sane defaults.
func NewServerFactory() ServerFactory {
	return &serverFactory{
		newLocalServerFunc: func() (Server, error) {
			return lxd.NewLocalServer()
		},
		newRemoteServerFunc: func(spec lxd.ServerSpec) (Server, error) {
			return lxd.NewRemoteServer(spec)
		},
		interfaceAddress: interfaceAddress{},
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
	svr, profile, err := s.initLocalServer()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// bootstrap a new local server, this ensures that all connections to and
	// from the local server are connected and setup correctly.
	var hostName string
	svr, hostName, err = s.bootstrapLocalServer(svr, profile)
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

func (s *serverFactory) RemoteServer(spec environs.CloudSpec) (Server, error) {
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
	serverSpec := lxd.NewServerSpec(spec.Endpoint,
		serverCert,
		clientCert,
	)
	serverSpec.WithProxy(proxy.DefaultConfig.GetProxy)
	svr, err := s.newRemoteServerFunc(serverSpec)
	if err == nil {
		err = s.bootstrapRemoteServer(svr)
	}
	return svr, errors.Trace(err)
}

func (s *serverFactory) InsecureRemoteServer(spec environs.CloudSpec) (Server, error) {
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

	serverSpec := lxd.NewInsecureServerSpec(spec.Endpoint)
	serverSpec.
		WithClientCertificate(clientCert).
		WithSkipGetServer(true)
	svr, err := s.newRemoteServerFunc(serverSpec)
	return svr, errors.Trace(err)
}

type apiProfile struct {
	Profile *lxdapi.Profile
	ETag    string
}

func (s *serverFactory) initLocalServer() (Server, apiProfile, error) {
	svr, err := s.newLocalServerFunc()
	if err != nil {
		return nil, apiProfile{}, errors.Trace(hoistLocalConnectErr(err))
	}

	// We need to get a default profile, so that the local bridge name
	// can be discovered correctly to then get the host address.
	defaultProfile, profileETag, err := svr.GetProfile("default")
	if err != nil {
		return nil, apiProfile{}, errors.Trace(err)
	}

	// If this is the LXD provider on the localhost, let's do an extra check to
	// make sure the default profile has a correctly configured bridge, and
	// which one is it.
	if err := svr.VerifyNetworkDevice(defaultProfile, profileETag); err != nil {
		return nil, apiProfile{}, errors.Trace(err)
	}

	// LXD itself reports the host:ports that it listens on.
	// Cross-check the address we have with the values reported by LXD.
	err = svr.EnableHTTPSListener()
	if err != nil {
		return nil, apiProfile{}, errors.Annotate(err, "enabling HTTPS listener")
	}
	return svr, apiProfile{
		Profile: defaultProfile,
		ETag:    profileETag,
	}, nil
}

func (s *serverFactory) bootstrapLocalServer(svr Server, profile apiProfile) (Server, string, error) {
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
			if svr, profile, err = s.initLocalServer(); err != nil {
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
	if err := s.validateServer(svr, profile); err != nil {
		return nil, "", errors.Trace(err)
	}

	return svr, hostAddress, nil
}

func (s *serverFactory) bootstrapRemoteServer(svr Server) error {
	// We need to get a default profile, so that the local bridge name
	// can be discovered correctly to then get the host address.
	defaultProfile, profileETag, err := svr.GetProfile("default")
	if err != nil {
		return errors.Trace(err)
	}

	// If this is the LXD provider on the localhost, let's do an extra check to
	// make sure the default profile has a correctly configured bridge, and
	// which one is it.
	if err := svr.VerifyNetworkDevice(defaultProfile, profileETag); err != nil {
		return errors.Trace(err)
	}

	if err := s.validateServer(svr, apiProfile{
		Profile: defaultProfile,
		ETag:    profileETag,
	}); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *serverFactory) validateServer(svr Server, profile apiProfile) error {
	// If the storage API is supported, let's make sure the LXD has a
	// default pool; we'll just use dir backend for now.
	if svr.StorageSupported() {
		if err := svr.EnsureDefaultStorage(profile.Profile, profile.ETag); err != nil {
			return errors.Trace(err)
		}
	}

	// One final request, to make sure we grab the server information for
	// validating the api version
	serverInfo, _, err := svr.GetServer()
	if err != nil {
		return errors.Trace(err)
	}

	apiVersion := serverInfo.APIVersion
	if msg, ok := isSupportedAPIVersion(apiVersion); !ok {
		logger.Warningf(msg)
		logger.Warningf("trying to use unsupported LXD API version %q", apiVersion)
	} else {
		logger.Infof("using LXD API version %q", apiVersion)
	}

	return nil
}

func (s *serverFactory) Clock() clock.Clock {
	if s.clock == nil {
		return clock.WallClock
	}
	return s.clock
}

// isSupportedAPIVersion defines what API versions we support.
func isSupportedAPIVersion(version string) (msg string, ok bool) {
	versionParts := strings.Split(version, ".")
	if len(versionParts) < 2 {
		return fmt.Sprintf("LXD API version %q: expected format <major>.<minor>", version), false
	}

	major, err := strconv.Atoi(versionParts[0])
	if err != nil {
		return fmt.Sprintf("LXD API version %q: unexpected major number: %v", version, err), false
	}

	if major < 1 {
		return fmt.Sprintf("LXD API version %q: expected major version 1 or later", version), false
	}

	return "", true
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
