// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/canonical/go-dqlite/app"
	"github.com/canonical/go-dqlite/client"
	"github.com/juju/errors"
	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/network"
	"github.com/juju/loggo"
)

const (
	dqliteDataDir = "dqlite"
	dqlitePort    = 17666
)

// OptionFactory creates Dqlite `App` initialisation arguments and options.
// These generally depend on a controller's agent config.
type OptionFactory struct {
	cfg            agent.Config
	port           int
	logger         Logger
	interfaceAddrs func() ([]net.Addr, error)

	bindAddress string
}

// NewOptionFactory returns a new OptionFactory reference
// based on the input agent configuration.
func NewOptionFactory(cfg agent.Config, logger Logger) *OptionFactory {
	return &OptionFactory{
		cfg:    cfg,
		port:   dqlitePort,
		logger: logger,

		interfaceAddrs: net.InterfaceAddrs,
	}
}

// EnsureDataDir ensures that a directory for Dqlite data exists at
// a path determined by the agent config, then returns that path.
func (f *OptionFactory) EnsureDataDir() (string, error) {
	dir := filepath.Join(f.cfg.DataDir(), dqliteDataDir)
	err := os.MkdirAll(dir, 0700)
	return dir, errors.Annotatef(err, "creating directory for Dqlite data")
}

// WithLogFuncOption returns a Dqlite application Option that will proxy Dqlite
// log output via this factory's logger where the level is recognised.
func (f *OptionFactory) WithLogFuncOption() app.Option {
	return app.WithLogFunc(func(level client.LogLevel, msg string, args ...interface{}) {
		if actualLevel, known := loggo.ParseLevel(level.String()); known {
			f.logger.Logf(actualLevel, msg, args...)
		}
	})
}

// WithAddressOption returns a Dqlite application Option
// for specifying the local address:port to use.
// See the comment for ensureBindAddress below.
func (f *OptionFactory) WithAddressOption() (app.Option, error) {
	if err := f.ensureBindAddress(); err != nil {
		return nil, errors.Annotate(err, "ensuring Dqlite bind address")
	}

	return app.WithAddress(fmt.Sprintf("%s:%d", f.bindAddress, f.port)), nil
}

// WithTLSOption returns a Dqlite application Option for TLS encryption
// of traffic between clients and clustered application nodes.
func (f *OptionFactory) WithTLSOption() (app.Option, error) {
	stateInfo, ok := f.cfg.StateServingInfo()
	if !ok {
		return nil, errors.NotSupportedf("Dqlite node initialisation on non-controller machine/container")
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM([]byte(f.cfg.CACert()))

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
func (f *OptionFactory) WithClusterOption() (app.Option, error) {
	if err := f.ensureBindAddress(); err != nil {
		return nil, errors.Annotate(err, "ensuring Dqlite bind address")
	}

	apiAddrs, err := f.cfg.APIAddresses()
	if err != nil {
		return nil, errors.Annotate(err, "retrieving API addresses")
	}

	for i, addr := range apiAddrs {
		apiAddrs[i] = strings.Split(addr, ":")[0]
	}

	apiAddrs = network.NewMachineAddresses(apiAddrs).AllMatchingScope(network.ScopeMatchCloudLocal).Values()

	// Using this option with no addresses works fine.
	// In fact, we only need a single other address to join a cluster.
	// Just ensure that our address is not one of the peers.
	var peerAddrs []string
	for _, addr := range apiAddrs {
		if addr != f.bindAddress && addr != "localhost" {
			peerAddrs = append(peerAddrs, fmt.Sprintf("%s:%d", addr, f.port))
		}
	}

	f.logger.Debugf("determined Dqlite cluster members: %v", peerAddrs)
	return app.WithCluster(peerAddrs), nil
}

// ensureBindAddress sets the bind address, used by clients and peers.
// TODO (manadart 2022-09-30): For now, this is *similar* to the peergrouper
// logic in that we require a unique local-cloud IP. We will need to revisit
// this because at present it is not influenced by a configured juju-ha-space.
func (f *OptionFactory) ensureBindAddress() error {
	if f.bindAddress != "" {
		return nil
	}

	sysAddrs, err := f.interfaceAddrs()
	if err != nil {
		return errors.Annotate(err, "querying addresses of system NICs")
	}

	var addrs network.MachineAddresses
	for _, sysAddr := range sysAddrs {
		var host string

		switch v := sysAddr.(type) {
		case *net.IPNet:
			host = v.IP.String()
		case *net.IPAddr:
			host = v.IP.String()
		default:
			continue
		}

		addrs = append(addrs, network.NewMachineAddress(host))
	}

	cloudLocal := addrs.AllMatchingScope(network.ScopeMatchCloudLocal)
	if len(cloudLocal) == 0 {
		return errors.NotFoundf("suitable local address for advertising to Dqlite peers")
	}
	if len(cloudLocal) > 1 {
		return errors.Errorf(
			"multiple local-cloud addresses detected. Dqlite bootstrap requires a unique address; found %v", cloudLocal)
	}

	f.bindAddress = cloudLocal.Values()[0]
	f.logger.Debugf("determined Dqlite bind address: %s", f.bindAddress)
	return nil
}
