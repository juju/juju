// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	lxd "github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared"
)

type Protocol string

const (
	LXDProtocol           Protocol = "lxd"
	SimpleStreamsProtocol Protocol = "simplestreams"
)

// ServerSpec describes the location and connection details for a
// server utilized in LXD workflows.
type ServerSpec struct {
	Name           string
	Host           string
	Protocol       Protocol
	Project        string
	connectionArgs *lxd.ConnectionArgs
}

// ProxyFunc defines a function that can act as a proxy for requests
type ProxyFunc func(*http.Request) (*url.URL, error)

// NewServerSpec creates a ServerSpec with default values where needed.
// It also ensures the HTTPS for the host implicitly
func NewServerSpec(host, project string, serverCert string, clientCert *Certificate) ServerSpec {
	return ServerSpec{
		Host:    EnsureHTTPS(host),
		Project: project,
		connectionArgs: &lxd.ConnectionArgs{
			TLSServerCert: serverCert,
			TLSClientCert: string(clientCert.CertPEM),
			TLSClientKey:  string(clientCert.KeyPEM),
		},
	}
}

// WithProxy adds the optional proxy to the server spec.
// Returns the ServerSpec to enable chaining of optional values
func (s ServerSpec) WithProxy(proxy ProxyFunc) ServerSpec {
	s.connectionArgs.Proxy = proxy
	return s
}

// WithClientCertificate adds the optional client Certificate to the server
// spec.
// Returns the ServerSpec to enable chaining of optional values
func (s ServerSpec) WithClientCertificate(clientCert *Certificate) ServerSpec {
	s.connectionArgs.TLSClientCert = string(clientCert.CertPEM)
	s.connectionArgs.TLSClientKey = string(clientCert.KeyPEM)
	return s
}

// WithSkipGetServer adds the option skipping of the get server verification to
// the server spec.
func (s ServerSpec) WithSkipGetServer(b bool) ServerSpec {
	s.connectionArgs.SkipGetServer = b
	return s
}

// NewInsecureServerSpec creates a ServerSpec without certificate requirements,
// which also bypasses the TLS verification.
// It also ensures the HTTPS for the host implicitly
func NewInsecureServerSpec(host string) ServerSpec {
	return ServerSpec{
		Host: EnsureHTTPS(host),
		connectionArgs: &lxd.ConnectionArgs{
			InsecureSkipVerify: true,
		},
	}
}

// MakeSimpleStreamsServerSpec creates a ServerSpec for the SimpleStreams
// protocol, ensuring that the host is HTTPS
func MakeSimpleStreamsServerSpec(name, host string) ServerSpec {
	return ServerSpec{
		Name:     name,
		Host:     EnsureHTTPS(host),
		Protocol: SimpleStreamsProtocol,
	}
}

// Validate ensures that the ServerSpec is valid.
func (s *ServerSpec) Validate() error {
	return nil
}

// CloudImagesRemote hosts releases blessed by the Canonical team.
var CloudImagesRemote = ServerSpec{
	Name:     "cloud-images.ubuntu.com",
	Host:     "https://cloud-images.ubuntu.com/releases",
	Protocol: SimpleStreamsProtocol,
}

// CloudImagesDailyRemote hosts images from daily package builds.
// These images have not been independently tested, but should be sound for
// use, being build from packages in the released archive.
var CloudImagesDailyRemote = ServerSpec{
	Name:     "cloud-images.ubuntu.com",
	Host:     "https://cloud-images.ubuntu.com/daily",
	Protocol: SimpleStreamsProtocol,
}

// ConnectImageRemote connects to a remote ImageServer using specified protocol.
var ConnectImageRemote = connectImageRemote

func connectImageRemote(remote ServerSpec) (lxd.ImageServer, error) {
	switch remote.Protocol {
	case LXDProtocol:
		return lxd.ConnectPublicLXD(remote.Host, remote.connectionArgs)
	case SimpleStreamsProtocol:
		return lxd.ConnectSimpleStreams(remote.Host, remote.connectionArgs)
	}
	return nil, fmt.Errorf("bad protocol supplied for connection: %v", remote.Protocol)
}

func connectLocal() (lxd.ContainerServer, error) {
	client, err := lxd.ConnectLXDUnix(SocketPath(nil), nil)
	return client, errors.Trace(err)
}

// ConnectRemote connects to LXD on a remote socket.
func ConnectRemote(spec ServerSpec) (lxd.ContainerServer, error) {
	// Ensure the Port on the Host, if we get an error it is reasonable to
	// assume that the address in the spec is invalid.
	uri, err := EnsureHostPort(spec.Host)
	if err != nil {
		return nil, errors.Trace(err)
	}
	client, err := lxd.ConnectLXD(uri, spec.connectionArgs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if spec.Project != "" {
		client.UseProject(spec.Project)
	}

	return client, nil
}

// SocketPath returns the path to the local LXD socket.
// The following are tried in order of preference:
//   - LXD_DIR environment variable.
//   - Snap socket.
//   - Debian socket.
// We give preference to LXD installed via Snap.
// isSocket defaults to socket detection from the LXD shared package.
// TODO (manadart 2018-04-30) This looks like it can be achieved by using a
// combination of VarPath and HostPath from lxd.shared, in which case this
// can be deprecated in their favour.
func SocketPath(isSocket func(path string) bool) string {
	path := os.Getenv("LXD_DIR")
	if path != "" {
		logger.Debugf("using environment LXD_DIR as socket path: %q", path)
	} else {
		path = filepath.FromSlash("/var/snap/lxd/common/lxd")
		if isSocket == nil {
			isSocket = shared.IsUnixSocket
		}
		if isSocket(filepath.Join(path, "unix.socket")) {
			logger.Debugf("using LXD snap socket: %q", path)
		} else {
			path = filepath.FromSlash("/var/lib/lxd")
			logger.Debugf("LXD snap socket not found, falling back to debian socket: %q", path)
		}
	}
	return filepath.Join(path, "unix.socket")
}

// EnsureHTTPS takes a URI and ensures that it is a HTTPS URL.
// LXD Requires HTTPS.
func EnsureHTTPS(address string) string {
	if strings.HasPrefix(address, "https://") {
		return address
	}
	if strings.HasPrefix(address, "http://") {
		addr := strings.Replace(address, "http://", "https://", 1)
		logger.Debugf("LXD requires https://, using: %s", addr)
		return addr
	}
	return "https://" + address
}

const defaultPort = 8443

// EnsureHostPort takes a URI and ensures that it has a port set, if it doesn't
// then it will ensure that port if added.
// The address supplied for the Host will be validated when parsed and if the
// address is not valid, then it will return an error.
func EnsureHostPort(address string) (string, error) {
	// make sure we ensure a schema, otherwise somewhere:8443 can return a
	// the following //:8443/somewhere
	uri, err := url.Parse(EnsureHTTPS(address))
	if err != nil {
		return "", errors.Trace(err)
	}
	if uri.Port() == "" {
		uri.Host = fmt.Sprintf("%s:%d", uri.Host, defaultPort)
	}
	return strings.TrimRight(uri.String(), "/"), nil
}
