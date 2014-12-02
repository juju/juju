// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package certupdater

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/cert"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
	"github.com/juju/utils/set"
	"gopkg.in/mgo.v2"
)

var logger = loggo.GetLogger("juju.worker.certificateupdater")

// CertificateUpdater is responsible for generating state server certificates.
//
// In practice, CertificateUpdater is used by a state server's machine agent to watch
// that server's machines addresses in state and write a new certificate to the
// agent's config file.
type CertificateUpdater struct {
	addressWatcher AddressWatcher
	setter         func(info *state.StateServingInfo) error
	environ        Environ
}

// AddressWatcher is an interface that is provided to NewCertificateUpdater
// which can be used to watch for machine address changes.
type AddressWatcher interface {
	WatchAddresses() state.NotifyWatcher
	Addresses() (addresses []network.Address)
}

// Environ is an interface that is provided to NewCertificateUpdater
// which can be used to get environment config and get/save state serving info..
type Environ interface {
	EnvironConfig() (*config.Config, error)
	StateServingInfo() (state.StateServingInfo, error)
	UpdateServerCertificate(info state.StateServingInfo, oldCert string) error
}

// StateServingInfoSetter defines a function that is called to set a
// StateServingInfo value with a newly generated certificate.
type StateServingInfoSetter func(info *state.StateServingInfo) error

// NewCertificateUpdater returns a worker.Worker that watches for changes to
// machine addresses and then generates a new state server certificate with those
// addresses in the certificate's SAN value.
func NewCertificateUpdater(addressWatcher AddressWatcher, environ Environ, setter StateServingInfoSetter) worker.Worker {
	return worker.NewNotifyWorker(&CertificateUpdater{
		addressWatcher: addressWatcher,
		environ:        environ,
		setter:         setter,
	})
}

// SetUp is defined on the NotifyWatchHandler interface.
func (c *CertificateUpdater) SetUp() (watcher.NotifyWatcher, error) {
	return c.addressWatcher.WatchAddresses(), nil
}

// Handle is defined on the NotifyWatchHandler interface.
func (c *CertificateUpdater) Handle() error {
	addresses := c.addressWatcher.Addresses()
	logger.Debugf("new machine addresses: %v", addresses)

	// Older Juju deployments will not have the CA cert private key
	// available.
	stateInfo, err := c.environ.StateServingInfo()
	if errors.IsNotFound(err) || errors.Cause(err) == mgo.ErrNotFound {
		logger.Warningf("no state serving info, cannot regenerate server certificate")
		return nil
	}
	if err != nil {
		return errors.Annotatef(err, "cannot read state serving info")
	}
	caPrivateKey := stateInfo.CAPrivateKey
	if caPrivateKey == "" {
		logger.Warningf("no CA cert private key, cannot regenerate server certificate")
		return nil
	}
	// Grab the env config and update a copy with ca cert private key.
	envConfig, err := c.environ.EnvironConfig()
	if err != nil {
		return errors.Annotatef(err, "cannot read environment config")
	}
	envConfig, err = envConfig.Apply(map[string]interface{}{"ca-private-key": caPrivateKey})
	if err != nil {
		return errors.Annotatef(err, "cannot add CA private key to environment config")
	}
	// We only want to include external IP addresses, so exclude local host.
	var serverAddrs []string
	for _, addr := range addresses {
		if addr.Value == "localhost" {
			continue
		}
		serverAddrs = append(serverAddrs, addr.Value)
	}

	// Generate a new certificate with the new server addresses incorporated, ensuring that any
	// existing addresses in the certificate are retained.
	newStateInfo, err := generateConsistentNewCertificate(c.environ, stateInfo, envConfig, serverAddrs)
	if err != nil {
		return errors.Annotatef(err, "cannot generate new server certificate")
	}

	// Run the callback to pass off the state info containing the newly generated certificate.
	err = c.setter(newStateInfo)
	if err != nil {
		return errors.Annotatef(err, "cannot write agent config")
	}
	logger.Infof("State Server cerificate addresses updated to %q", addresses)
	return nil
}

// generateConsistentNewCertificate creates a new server certificate with the supplied
// addresses. These addresses are merged with any existing addresses.
func generateConsistentNewCertificate(environ Environ, info state.StateServingInfo,
	cfg *config.Config, newServerAddresses []string,
) (*state.StateServingInfo, error) {

	// Extract and merge any existing addresses.
	existingAddrs, err := certificateSANs(info.Cert)
	if err != nil {
		return nil, err
	}
	serverAddrs := set.NewStrings(newServerAddresses...)
	serverAddrs = serverAddrs.Union(set.NewStrings(existingAddrs...))

	// Have several attempts at writing out the new certificate, ensuring that
	// another state server has not also written out a certificate at the same time
	// with different addresses.
	for i := 0; i < 5; i++ {
		cert, key, err := generateNewCertificate(environ, info, cfg, serverAddrs)
		if err == nil {
			info.Cert = string(cert)
			info.PrivateKey = string(key)
			return &info, nil
		}
		if err != state.CertificateConsistencyError {
			return nil, err
		}
		info, err = environ.StateServingInfo()
		if err != nil {
			return nil, errors.Annotatef(err, "cannot re-read state serving info")
		}

	}
	return nil, state.CertificateConsistencyError
}

// generateNewCertificate creates a new server certificate with the supplied addresses
// and writes it back to the environment.
func generateNewCertificate(environ Environ, info state.StateServingInfo, cfg *config.Config,
	serverAddresses set.Strings,
) (string, string, error) {
	// Generate a new state server certificate with the machine addresses in the SAN value.
	cert, key, err := cfg.GenerateStateServerCertAndKey(serverAddresses.Values())
	if err != nil {
		return "", "", errors.Annotate(err, "cannot generate state server certificate")
	}

	oldCert := info.Cert
	// Write out the new state info containing the certificate.
	info.Cert = string(cert)
	info.PrivateKey = string(key)
	if err := environ.UpdateServerCertificate(info, oldCert); err != nil {
		return "", "", err
	}
	return cert, key, nil
}

// certificateSANs extracts the SAN IP addresses from the certificate.
func certificateSANs(serverCert string) ([]string, error) {
	srvCert, err := cert.ParseCert(serverCert)
	if err != nil {
		return nil, errors.Annotatef(err, "invalid server certificate")
	}
	sanIPs := make([]string, len(srvCert.IPAddresses))
	for i, ip := range srvCert.IPAddresses {
		sanIPs[i] = ip.String()
	}
	return sanIPs, nil
}

// TearDown is defined on the NotifyWatchHandler interface.
func (c *CertificateUpdater) TearDown() error {
	return nil
}
