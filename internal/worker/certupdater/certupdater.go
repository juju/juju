// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package certupdater

import (
	"context"
	"net"
	"slices"

	"github.com/juju/worker/v5"

	"github.com/juju/juju/controller"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/pki"
)

// ControllerConfigGetter is an interface that returns the controller config.
type ControllerConfigGetter interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// CertificateUpdater is responsible for generating controller certificates.
//
// In practice, CertificateUpdater is used by a controller agent to watch the
// published client and peer addresses, and write an updated certificate to the
// agent's config file.
type CertificateUpdater struct {
	authority             pki.Authority
	controllerNodeService ControllerNodeService
	addresses             []string
	initialized           bool
	logger                logger.Logger
}

// ControllerNodeService provides controller certificate addresses.
type ControllerNodeService interface {
	// GetAllAPIAddressesForCertificates returns SAN addresses without ports.
	GetAllAPIAddressesForCertificates(ctx context.Context) ([]string, error)
	// WatchControllerAddressesForCertificates watches client and peer address
	// projections.
	WatchControllerAddressesForCertificates(ctx context.Context) (watcher.NotifyWatcher, error)
}

// Config holds the configuration for the certificate updater worker.
type Config struct {
	Authority             pki.Authority
	ControllerNodeService ControllerNodeService
	Logger                logger.Logger
}

func (c *Config) Validate() error {
	if c.Authority == nil {
		return errors.New("nil Authority").Add(coreerrors.NotValid)
	}
	if c.ControllerNodeService == nil {
		return errors.New("nil ControllerNodeService").Add(coreerrors.NotValid)
	}
	if c.Logger == nil {
		return errors.New("nil Logger").Add(coreerrors.NotValid)
	}
	return nil
}

// NewCertificateUpdater returns a worker.Worker that watches published client
// and peer addresses and generates a controller certificate with those
// addresses in the certificate's SAN value.
func NewCertificateUpdater(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	return watcher.NewNotifyWorker(watcher.NotifyConfig{
		Handler: &CertificateUpdater{
			authority:             config.Authority,
			controllerNodeService: config.ControllerNodeService,
			logger:                config.Logger,
		},
	})
}

// SetUp is defined on the NotifyWatchHandler interface.
func (c *CertificateUpdater) SetUp(ctx context.Context) (watcher.NotifyWatcher, error) {
	return c.controllerNodeService.WatchControllerAddressesForCertificates(ctx)
}

// Handle is defined on the NotifyWatchHandler interface.
func (c *CertificateUpdater) Handle(ctx context.Context) error {
	addresses, err := c.controllerNodeService.GetAllAPIAddressesForCertificates(ctx)
	if err != nil {
		return errors.Errorf("retrieving controller certificate addresses: %w", err)
	}
	if len(addresses) == 0 {
		// If there are no addresses, we want to generate a certificate with
		// some default SAN values, so we don't return an error here.
		c.logger.Debugf(ctx, "no controller certificate addresses found, using default SAN addresses: %#v", controller.DefaultDNSNames)
	}
	addresses = append(slices.Clone(addresses), controller.DefaultDNSNames...)

	if c.initialized && slices.Equal(addresses, c.addresses) {
		// Sometimes the watcher will tell us things have changed, when they
		// haven't as far as we can tell.
		c.logger.Debugf(ctx, "addresses have not changed since last updated cert")
		return nil
	}
	return c.updateCertificate(ctx, addresses)
}

func (c *CertificateUpdater) updateCertificate(ctx context.Context, addresses []string) error {
	c.logger.Debugf(ctx, "new controller certificate SAN addresses: %#v", addresses)

	request := c.authority.LeafRequestForGroup(pki.ControllerIPLeafGroup)
	for _, addr := range addresses {
		if addr == "localhost" {
			continue
		}
		switch network.DeriveAddressType(addr) {
		case network.HostName:
			request.AddDNSNames(addr)
		case network.IPv4Address, network.IPv6Address:
			ip := net.ParseIP(addr)
			if ip == nil {
				return errors.Errorf(
					"value %q is not a valid ip address", addr)
			}
			request.AddIPAddresses(ip)
		default:
			c.logger.Warningf(ctx,
				"unsupported address %q for controller certificate",
				addr)
		}
	}

	if _, err := request.Commit(); err != nil {
		c.logger.Debugf(ctx, "commit error: %w", err)
		return errors.Errorf("generating controller certificate: %w", err)
	}
	c.addresses = slices.Clone(addresses)
	c.initialized = true
	return nil
}

// TearDown is defined on the NotifyWatchHandler interface.
func (c *CertificateUpdater) TearDown() error {
	return nil
}
