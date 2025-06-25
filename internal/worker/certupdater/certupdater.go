// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package certupdater

import (
	"context"
	"net"
	"reflect"

	"github.com/juju/worker/v4"

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
// In practice, CertificateUpdater is used by a controller's machine agent to watch
// that server's machines addresses in state, and write a new certificate to the
// agent's config file.
type CertificateUpdater struct {
	authority             pki.Authority
	controllerNodeService ControllerNodeService
	addresses             []string
	logger                logger.Logger
}

// ControllerNodeService returns all known API addresses.
type ControllerNodeService interface {
	// GetAllCloudLocalAPIAddresses returns a string slice of api
	// addresses available for clients. The list list only contains cloud
	// local addresses. The returned strings are IP address only without
	// port numbers.
	GetAllCloudLocalAPIAddresses(ctx context.Context) ([]string, error)
	// WatchControllerAPIAddresses returns a watcher that observes changes to the
	// controller api addresses.
	WatchControllerAPIAddresses(ctx context.Context) (watcher.NotifyWatcher, error)
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

// NewCertificateUpdater returns a worker.Worker that watches for changes to
// machine addresses and then generates a new controller certificate with those
// addresses in the certificate's SAN value.
func NewCertificateUpdater(config Config) (worker.Worker, error) {
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
	// Populate certificate SAN with any addresses we know about now.
	initialSANAddresses, err := c.controllerNodeService.GetAllCloudLocalAPIAddresses(ctx)
	if err != nil {
		return nil, errors.Errorf("retrieving initial server addresses: %w", err)
	}
	if err := c.updateCertificate(ctx, initialSANAddresses); err != nil {
		return nil, errors.Errorf("setting initial certificate SAN list: %w", err)
	}
	return c.controllerNodeService.WatchControllerAPIAddresses(ctx)
}

// Handle is defined on the NotifyWatchHandler interface.
func (c *CertificateUpdater) Handle(ctx context.Context) error {
	addresses, err := c.controllerNodeService.GetAllCloudLocalAPIAddresses(ctx)
	if err != nil {
		return errors.Errorf("retrieving cloud local api addresses: %w", err)
	}

	if reflect.DeepEqual(addresses, c.addresses) {
		// Sometimes the watcher will tell us things have changed, when they
		// haven't as far as we can tell.
		c.logger.Debugf(ctx, "addresses have not changed since last updated cert")
		return nil
	}
	return c.updateCertificate(ctx, addresses)
}

func (c *CertificateUpdater) updateCertificate(ctx context.Context, addresses []string) error {
	c.logger.Debugf(ctx, "new machine addresses: %#v", addresses)
	c.addresses = addresses

	if len(addresses) == 0 {
		return nil
	}

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
		return errors.Errorf("generating default controller ip certificate: %w", err)
	}
	return nil
}

// TearDown is defined on the NotifyWatchHandler interface.
func (c *CertificateUpdater) TearDown() error {
	return nil
}
