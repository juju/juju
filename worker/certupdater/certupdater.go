// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package certupdater

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/set"

	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cert"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.certupdater")

// CertificateUpdater is responsible for generating state server certificates.
//
// In practice, CertificateUpdater is used by a state server's machine agent to watch
// that server's machines addresses in state, and write a new certificate to the
// agent's config file.
type CertificateUpdater struct {
	addressWatcher  AddressWatcher
	getter          StateServingInfoGetter
	setter          StateServingInfoSetter
	configGetter    EnvironConfigGetter
	hostPortsGetter APIHostPortsGetter
	certChanged     chan params.StateServingInfo
	addresses       []network.Address
}

// AddressWatcher is an interface that is provided to NewCertificateUpdater
// which can be used to watch for machine address changes.
type AddressWatcher interface {
	WatchAddresses() state.NotifyWatcher
	Addresses() (addresses []network.Address)
}

// EnvironConfigGetter is an interface that is provided to NewCertificateUpdater
// which can be used to get environment config.
type EnvironConfigGetter interface {
	EnvironConfig() (*config.Config, error)
}

// StateServingInfoGetter is an interface that is provided to NewCertificateUpdater
// whose StateServingInfo method will be invoked to get state serving info.
type StateServingInfoGetter interface {
	StateServingInfo() (params.StateServingInfo, bool)
}

// StateServingInfoSetter defines a function that is called to set a
// StateServingInfo value with a newly generated certificate.
type StateServingInfoSetter func(info params.StateServingInfo, done <-chan struct{}) error

// APIHostPortsGetter is an interface that is provided to NewCertificateUpdater
// whose APIHostPorts method will be invoked to get state server addresses.
type APIHostPortsGetter interface {
	APIHostPorts() ([][]network.HostPort, error)
}

// NewCertificateUpdater returns a worker.Worker that watches for changes to
// machine addresses and then generates a new state server certificate with those
// addresses in the certificate's SAN value.
func NewCertificateUpdater(addressWatcher AddressWatcher, getter StateServingInfoGetter,
	configGetter EnvironConfigGetter, hostPortsGetter APIHostPortsGetter, setter StateServingInfoSetter,
	certChanged chan params.StateServingInfo,
) worker.Worker {
	return worker.NewNotifyWorker(&CertificateUpdater{
		addressWatcher:  addressWatcher,
		configGetter:    configGetter,
		hostPortsGetter: hostPortsGetter,
		getter:          getter,
		setter:          setter,
		certChanged:     certChanged,
	})
}

// SetUp is defined on the NotifyWatchHandler interface.
func (c *CertificateUpdater) SetUp() (watcher.NotifyWatcher, error) {
	// Populate certificate SAN with any addresses we know about now.
	apiHostPorts, err := c.hostPortsGetter.APIHostPorts()
	if err != nil {
		return nil, errors.Annotate(err, "retrieving initial server addesses")
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
	if err := c.updateCertificate(initialSANAddresses, make(chan struct{})); err != nil {
		return nil, errors.Annotate(err, "setting initial cerificate SAN list")
	}
	// Return
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
	return c.updateCertificate(addresses, done)
}

func (c *CertificateUpdater) updateCertificate(addresses []network.Address, done <-chan struct{}) error {
	logger.Debugf("new machine addresses: %#v", addresses)
	c.addresses = addresses

	// Older Juju deployments will not have the CA cert private key
	// available.
	stateInfo, ok := c.getter.StateServingInfo()
	if !ok {
		logger.Warningf("no state serving info, cannot regenerate server certificate")
		return nil
	}
	caPrivateKey := stateInfo.CAPrivateKey
	if caPrivateKey == "" {
		logger.Warningf("no CA cert private key, cannot regenerate server certificate")
		return nil
	}
	// Grab the env config and update a copy with ca cert private key.
	envConfig, err := c.configGetter.EnvironConfig()
	if err != nil {
		return errors.Annotate(err, "cannot read environment config")
	}
	envConfig, err = envConfig.Apply(map[string]interface{}{"ca-private-key": caPrivateKey})
	if err != nil {
		return errors.Annotate(err, "cannot add CA private key to environment config")
	}

	// For backwards compatibility, we must include "anything", "juju-apiserver"
	// and "juju-mongodb" as hostnames as that is what clients specify
	// as the hostname for verification (this certicate is used both
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

	// Generate a new state server certificate with the machine addresses in the SAN value.
	newCert, newKey, err := envConfig.GenerateStateServerCertAndKey(newServerAddrs)
	if err != nil {
		return errors.Annotate(err, "cannot generate state server certificate")
	}
	stateInfo.Cert = string(newCert)
	stateInfo.PrivateKey = string(newKey)
	err = c.setter(stateInfo, done)
	if err != nil {
		return errors.Annotate(err, "cannot write agent config")
	}
	logger.Infof("State Server cerificate addresses updated to %q", newServerAddrs)
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
	select {
	case <-c.certChanged:
		// already closed
	default:
		close(c.certChanged)
	}
	return nil
}
