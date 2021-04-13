// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package certupdater

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/worker/v2"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/pki"
	"github.com/juju/juju/state"
	"github.com/juju/juju/watcher/legacy"
)

var (
	logger = loggo.GetLogger("juju.worker.certupdater")
)

// CertificateUpdater is responsible for generating controller certificates.
//
// In practice, CertificateUpdater is used by a controller's machine agent to watch
// that server's machines addresses in state, and write a new certificate to the
// agent's config file.
type CertificateUpdater struct {
	addressWatcher  AddressWatcher
	authority       pki.Authority
	hostPortsGetter APIHostPortsGetter
	addresses       network.SpaceAddresses
}

// AddressWatcher is an interface that is provided to NewCertificateUpdater
// which can be used to watch for machine address changes.
type AddressWatcher interface {
	WatchAddresses() state.NotifyWatcher
	Addresses() (addresses network.SpaceAddresses)
}

// StateServingInfoGetter is an interface that is provided to NewCertificateUpdater
// whose StateServingInfo method will be invoked to get state serving info.
type StateServingInfoGetter interface {
	StateServingInfo() (controller.StateServingInfo, bool)
}

// APIHostPortsGetter is an interface that is provided to NewCertificateUpdater.
// It returns all known API addresses.
type APIHostPortsGetter interface {
	APIHostPortsForClients() ([]network.SpaceHostPorts, error)
}

// Config holds the configuration for the certificate updater worker.
type Config struct {
	AddressWatcher     AddressWatcher
	Authority          pki.Authority
	APIHostPortsGetter APIHostPortsGetter
}

// NewCertificateUpdater returns a worker.Worker that watches for changes to
// machine addresses and then generates a new controller certificate with those
// addresses in the certificate's SAN value.
func NewCertificateUpdater(config Config) worker.Worker {
	return legacy.NewNotifyWorker(&CertificateUpdater{
		addressWatcher:  config.AddressWatcher,
		authority:       config.Authority,
		hostPortsGetter: config.APIHostPortsGetter,
	})
}

// SetUp is defined on the NotifyWatchHandler interface.
func (c *CertificateUpdater) SetUp() (state.NotifyWatcher, error) {
	// Populate certificate SAN with any addresses we know about now.
	apiHostPorts, err := c.hostPortsGetter.APIHostPortsForClients()
	if err != nil {
		return nil, errors.Annotate(err, "retrieving initial server addresses")
	}
	var initialSANAddresses network.SpaceAddresses
	for _, server := range apiHostPorts {
		for _, nhp := range server {
			if nhp.Scope != network.ScopeCloudLocal {
				continue
			}
			initialSANAddresses = append(initialSANAddresses, nhp.SpaceAddress)
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

func (c *CertificateUpdater) updateCertificate(addresses network.SpaceAddresses) error {
	logger.Debugf("new machine addresses: %#v", addresses)
	c.addresses = addresses

	request := c.authority.LeafRequestForGroup(pki.ControllerIPLeafGroup)

	for _, addr := range addresses {
		if addr.Value == "localhost" {
			continue
		}

		switch addr.Type {
		case network.HostName:
			request.AddDNSNames(addr.Value)
		case network.IPv4Address:
			ip := addr.IP()
			if ip == nil {
				return errors.Errorf(
					"value %s is not a valid ip address", addr.Value)
			}
			request.AddIPAddresses(ip)
		case network.IPv6Address:
			ip := addr.IP()
			if ip == nil {
				return errors.Errorf(
					"value %s is not a valid ip address", addr.Value)
			}
			request.AddIPAddresses(ip)
		default:
			logger.Warningf(
				"unsupported space address type %s for controller certificate",
				addr.Type)
		}

	}

	if _, err := request.Commit(); err != nil {
		return errors.Annotate(err, "generating default controller ip certificate")
	}
	return nil
}

// TearDown is defined on the NotifyWatchHandler interface.
func (c *CertificateUpdater) TearDown() error {
	return nil
}
