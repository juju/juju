// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package certupdater

import (
	"reflect"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/cert"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/watcher/legacy"
)

var logger = loggo.GetLogger("juju.worker.certupdater")

// CertificateUpdater is responsible for generating controller certificates.
//
// In practice, CertificateUpdater is used by a controller's machine agent to watch
// that server's machines addresses in state, and write a new certificate to the
// agent's config file.
type CertificateUpdater struct {
	addressWatcher  AddressWatcher
	getter          StateServingInfoGetter
	setter          StateServingInfoSetter
	configGetter    ControllerConfigGetter
	hostPortsGetter APIHostPortsGetter
	addresses       []network.Address
}

// AddressWatcher is an interface that is provided to NewCertificateUpdater
// which can be used to watch for machine address changes.
type AddressWatcher interface {
	WatchAddresses() state.NotifyWatcher
	Addresses() (addresses []network.Address)
}

// ControllerConfigGetter is an interface that is provided to NewCertificateUpdater
// which can be used to get the controller config.
type ControllerConfigGetter interface {
	ControllerConfig() (controller.Config, error)
}

// StateServingInfoGetter is an interface that is provided to NewCertificateUpdater
// whose StateServingInfo method will be invoked to get state serving info.
type StateServingInfoGetter interface {
	StateServingInfo() (params.StateServingInfo, bool)
}

// StateServingInfoSetter defines a function that is called to set a
// StateServingInfo value with a newly generated certificate.
type StateServingInfoSetter func(info params.StateServingInfo) error

// APIHostPortsGetter is an interface that is provided to NewCertificateUpdater.
// It returns all known API addresses.
type APIHostPortsGetter interface {
	APIHostPortsForClients() ([][]network.HostPort, error)
}

// Config holds the configuration for the certificate updater worker.
type Config struct {
	AddressWatcher         AddressWatcher
	StateServingInfoGetter StateServingInfoGetter
	StateServingInfoSetter StateServingInfoSetter
	ControllerConfigGetter ControllerConfigGetter
	APIHostPortsGetter     APIHostPortsGetter
}

// NewCertificateUpdater returns a worker.Worker that watches for changes to
// machine addresses and then generates a new controller certificate with those
// addresses in the certificate's SAN value.
func NewCertificateUpdater(config Config) worker.Worker {
	return legacy.NewNotifyWorker(&CertificateUpdater{
		addressWatcher:  config.AddressWatcher,
		configGetter:    config.ControllerConfigGetter,
		hostPortsGetter: config.APIHostPortsGetter,
		getter:          config.StateServingInfoGetter,
		setter:          config.StateServingInfoSetter,
	})
}

// SetUp is defined on the NotifyWatchHandler interface.
func (c *CertificateUpdater) SetUp() (state.NotifyWatcher, error) {
	// Populate certificate SAN with any addresses we know about now.
	apiHostPorts, err := c.hostPortsGetter.APIHostPortsForClients()
	if err != nil {
		return nil, errors.Annotate(err, "retrieving initial server addresses")
	}
	var initialSANAddresses []network.Address
	for _, server := range apiHostPorts {
		for _, nhp := range server {
			if nhp.Scope != network.ScopeCloudLocal {
				continue
			}
			initialSANAddresses = append(initialSANAddresses, nhp.Address)
		}
	}
	if err := c.updateCertificate(initialSANAddresses); err != nil {
		return nil, errors.Annotate(err, "setting initial certificate SAN list")
	}
	return c.addressWatcher.WatchAddresses(), nil
}

// Handle is defined on the NotifyWatchHandler interface.
func (c *CertificateUpdater) Handle(done <-chan struct{}) error {
	addresses := c.addressWatcher.Addresses()
	if reflect.DeepEqual(addresses, c.addresses) {
		// Sometimes the watcher will tell us things have changed, when they
		// haven't as far as we can tell.
		logger.Debugf("addresses haven't really changed since last updated cert")
		return nil
	}
	return c.updateCertificate(addresses)
}

func (c *CertificateUpdater) updateCertificate(addresses []network.Address) error {
	logger.Debugf("new machine addresses: %#v", addresses)
	c.addresses = addresses

	// Older Juju deployments will not have the CA cert private key
	// available.
	stateInfo, ok := c.getter.StateServingInfo()
	if !ok {
		return errors.New("no state serving info, cannot regenerate server certificate")
	}
	caPrivateKey := stateInfo.CAPrivateKey
	if caPrivateKey == "" {
		logger.Errorf("no CA cert private key, cannot regenerate server certificate")
		return nil
	}

	cfg, err := c.configGetter.ControllerConfig()
	if err != nil {
		return errors.Annotate(err, "cannot read controller config")
	}

	// For backwards compatibility, we must include "anything", "juju-apiserver"
	// and "juju-mongodb" as hostnames as that is what clients specify
	// as the hostname for verification (this certificate is used both
	// for serving MongoDB and API server connections).  We also
	// explicitly include localhost.
	serverAddrs := []string{"localhost", "juju-apiserver", "juju-mongodb", "anything"}
	for _, addr := range addresses {
		if addr.Value == "localhost" {
			continue
		}
		serverAddrs = append(serverAddrs, addr.Value)
	}
	newServerAddrs, update, err := updateRequired(stateInfo.Cert, serverAddrs)
	if err != nil {
		return errors.Annotate(err, "cannot determine if cert update needed")
	}
	if !update {
		logger.Debugf("no certificate update required")
		return nil
	}

	// Generate a new controller certificate with the machine addresses in the SAN value.
	caCert, hasCACert := cfg.CACert()
	if !hasCACert {
		return errors.New("configuration has no ca-cert")
	}
	newCert, newKey, err := controller.GenerateControllerCertAndKey(caCert, caPrivateKey, newServerAddrs)
	if err != nil {
		return errors.Annotate(err, "cannot generate controller certificate")
	}
	stateInfo.Cert = newCert
	stateInfo.PrivateKey = newKey
	err = c.setter(stateInfo)
	if err != nil {
		return errors.Annotate(err, "cannot write agent config")
	}
	logger.Infof("controller certificate addresses updated to %q", newServerAddrs)
	return nil
}

// updateRequired returns true and a list of merged addresses if any of the
// new addresses are not yet contained in the server cert SAN list.
func updateRequired(serverCert string, newAddrs []string) ([]string, bool, error) {
	x509Cert, err := cert.ParseCert(serverCert)
	if err != nil {
		return nil, false, errors.Annotate(err, "cannot parse existing TLS certificate")
	}
	existingAddr := set.NewStrings()
	for _, ip := range x509Cert.IPAddresses {
		existingAddr.Add(ip.String())
	}
	logger.Debugf("existing cert addresses %v", existingAddr)
	logger.Debugf("new addresses %v", newAddrs)
	// Does newAddr contain any that are not already in existingAddr?
	newAddrSet := set.NewStrings(newAddrs...)
	update := newAddrSet.Difference(existingAddr).Size() > 0
	newAddrSet = newAddrSet.Union(existingAddr)
	return newAddrSet.SortedValues(), update, nil
}

// TearDown is defined on the NotifyWatchHandler interface.
func (c *CertificateUpdater) TearDown() error {
	return nil
}
