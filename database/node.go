// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/database/app"
	"github.com/juju/juju/database/client"
	"github.com/juju/juju/network"
	"github.com/juju/loggo"
)

const (
	dqliteDataDir = "dqlite"
	dqlitePort    = 17666
)

// DefaultBindAddress is the address that will *always* be returned by
// WithAddressOption. It is used in tests to override address detection.
var DefaultBindAddress = ""

// NodeManager is responsible for interrogating a single Dqlite node,
// and emitting configuration for starting its Dqlite `App` based on
// operational requirements and controller agent config.
type NodeManager struct {
	cfg    agent.Config
	port   int
	logger Logger

	dataDir     string
	bindAddress string
}

// NewNodeManager returns a new NodeManager reference
// based on the input agent configuration.
func NewNodeManager(cfg agent.Config, logger Logger) *NodeManager {
	return &NodeManager{
		cfg:         cfg,
		port:        dqlitePort,
		logger:      logger,
		bindAddress: DefaultBindAddress,
	}
}

// IsExistingNode returns true if this machine or container has
// ever started a Dqlite `App` before. Specifically, this is whether
// the Dqlite data directory is empty.
func (m *NodeManager) IsExistingNode() (bool, error) {
	if _, err := m.EnsureDataDir(); err != nil {
		return false, errors.Annotate(err, "ensuring Dqlite data directory")
	}

	dir, err := os.Open(m.dataDir)
	if err != nil {
		return false, errors.Annotate(err, "opening Dqlite data directory")
	}

	_, err = dir.Readdirnames(1)
	switch err {
	case nil:
		return true, nil
	case io.EOF:
		return false, nil
	default:
		return false, errors.Annotate(err, "reading Dqlite data directory")
	}
}

// EnsureDataDir ensures that a directory for Dqlite data exists at
// a path determined by the agent config, then returns that path.
func (m *NodeManager) EnsureDataDir() (string, error) {
	if m.dataDir == "" {
		dir := filepath.Join(m.cfg.DataDir(), dqliteDataDir)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return "", errors.Annotatef(err, "creating directory for Dqlite data")
		}
		m.dataDir = dir
	}
	return m.dataDir, nil
}

// WithLogFuncOption returns a Dqlite application Option that will proxy Dqlite
// log output via this factory's logger where the level is recognised.
func (m *NodeManager) WithLogFuncOption() app.Option {
	return app.WithLogFunc(func(level client.LogLevel, msg string, args ...interface{}) {
		if actualLevel, known := loggo.ParseLevel(level.String()); known {
			m.logger.Logf(actualLevel, msg, args...)
		}
	})
}

// WithAddressOption returns a Dqlite application Option
// for specifying the local address:port to use.
// See the comment for ensureBindAddress below.
func (m *NodeManager) WithAddressOption() (app.Option, error) {
	if err := m.ensureBindAddress(); err != nil {
		return nil, errors.Annotate(err, "ensuring Dqlite bind address")
	}

	return app.WithAddress(fmt.Sprintf("%s:%d", m.bindAddress, m.port)), nil
}

// WithTLSOption returns a Dqlite application Option for TLS encryption
// of traffic between clients and clustered application nodes.
func (m *NodeManager) WithTLSOption() (app.Option, error) {
	stateInfo, ok := m.cfg.StateServingInfo()
	if !ok {
		return nil, errors.NotSupportedf("Dqlite node initialisation on non-controller machine/container")
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM([]byte(m.cfg.CACert()))

	controllerCert, err := tls.X509KeyPair([]byte(stateInfo.Cert), []byte(stateInfo.PrivateKey))
	if err != nil {
		return nil, errors.Annotate(err, "parsing controller certificate")
	}

	listen := &tls.Config{
		ClientCAs:    caCertPool,
		Certificates: []tls.Certificate{controllerCert},
	}

	dial := &tls.Config{
		RootCAs:      caCertPool,
		Certificates: []tls.Certificate{controllerCert},
		// We cannot provide a ServerName value here, so we rely on the
		// server validating the controller's client certificate.
		InsecureSkipVerify: true,
	}

	return app.WithTLS(listen, dial), nil
}

// WithClusterOption returns a Dqlite application Option for initialising
// Dqlite as the member of a cluster with peers representing other controllers.
// TODO (manadart 2022-09-30): As with WithAddressOption, this relies on each
// controller having a unique local-cloud address *and* that we can simply
// use those addresses that aren't ours to determine peers.
// This will need revision in the context of juju-ha-space as well.
// Furthermore, relying on agent config for API addresses implicitly makes this
// affected by a configured juju-ctrl-space, which might be undesired.
func (m *NodeManager) WithClusterOption() (app.Option, error) {
	if err := m.ensureBindAddress(); err != nil {
		return nil, errors.Annotate(err, "ensuring Dqlite bind address")
	}

	apiAddrs, err := m.cfg.APIAddresses()
	if err != nil {
		return nil, errors.Annotate(err, "retrieving API addresses")
	}

	for i, addr := range apiAddrs {
		apiAddrs[i] = strings.Split(addr, ":")[0]
	}

	apiAddrs = corenetwork.NewMachineAddresses(apiAddrs).AllMatchingScope(corenetwork.ScopeMatchCloudLocal).Values()

	// Using this option with no addresses works fine.
	// In fact, we only need a single other address to join a cluster.
	// Just ensure that our address is not one of the peers.
	var peerAddrs []string
	for _, addr := range apiAddrs {
		if addr != m.bindAddress && addr != "localhost" {
			peerAddrs = append(peerAddrs, fmt.Sprintf("%s:%d", addr, m.port))
		}
	}

	m.logger.Debugf("determined Dqlite cluster members: %v", peerAddrs)
	return app.WithCluster(peerAddrs), nil
}

// ensureBindAddress sets the bind address, used by clients and peers.
// We will need to revisit this because at present it is not influenced
// by a configured juju-ha-space.
func (m *NodeManager) ensureBindAddress() error {
	if m.bindAddress != "" {
		return nil
	}

	nics, err := net.Interfaces()
	if err != nil {
		return errors.Annotate(err, "querying local network interfaces")
	}

	var addrs corenetwork.MachineAddresses
	for _, nic := range nics {
		if ignoreInterface(nic) {
			continue
		}

		sysAddrs, err := nic.Addrs()
		if err != nil || len(sysAddrs) == 0 {
			continue
		}

		for _, addr := range sysAddrs {
			switch v := addr.(type) {
			case *net.IPNet:
				addrs = append(addrs, corenetwork.NewMachineAddress(v.IP.String()))
			case *net.IPAddr:
				addrs = append(addrs, corenetwork.NewMachineAddress(v.IP.String()))
			default:
			}
		}
	}

	cloudLocal := addrs.AllMatchingScope(corenetwork.ScopeMatchCloudLocal).Values()

	if len(cloudLocal) == 0 {
		return errors.NotFoundf("suitable local address for advertising to Dqlite peers")
	}

	sort.Strings(cloudLocal)
	m.bindAddress = cloudLocal[0]
	m.logger.Debugf("chose Dqlite bind address %v from %v", m.bindAddress, cloudLocal)
	return nil
}

// ignoreInterface returns true if we should discount the input
// interface as one suitable for binding Dqlite to.
// Such interfaces are loopback devices and the default bridges
// for LXD/KVM/Docker.
func ignoreInterface(nic net.Interface) bool {
	return int(nic.Flags&net.FlagLoopback) > 0 ||
		nic.Name == network.DefaultLXDBridge ||
		nic.Name == network.DefaultKVMBridge ||
		nic.Name == network.DefaultDockerBridge
}
