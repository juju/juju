// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddressupdater

import (
	"context"
	"fmt"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"

	"github.com/juju/juju/core/logger"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/network"
)

// APIAddresser is an interface that is provided to NewAPIAddressUpdater
// which can be used to watch for API address changes.
type APIAddresser interface {
	APIHostPorts(context.Context) ([]corenetwork.ProviderHostPorts, error)
	WatchAPIHostPorts(context.Context) (watcher.NotifyWatcher, error)
}

// APIAddressSetter is an interface that is provided to NewAPIAddressUpdater
// whose SetAPIHostPorts method will be invoked whenever address changes occur.
type APIAddressSetter interface {
	SetAPIHostPorts(servers []corenetwork.HostPorts) error
}

// Config defines the operation of a Worker.
type Config struct {
	Addresser APIAddresser
	Setter    APIAddressSetter
	Logger    logger.Logger
}

// Validate returns an error if config cannot drive a Worker.
func (config Config) Validate() error {
	if config.Addresser == nil {
		return errors.NotValidf("nil Addresser")
	}
	if config.Setter == nil {
		return errors.NotValidf("nil Setter")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// APIAddressUpdater is responsible for propagating API addresses.
//
// In practice, APIAddressUpdater is used by a machine agent to watch
// API addresses in state and write the changes to the agent's config file.
type APIAddressUpdater struct {
	config Config

	mu      sync.Mutex
	current []corenetwork.ProviderHostPorts
}

// NewAPIAddressUpdater returns a worker.Worker that watches for changes to
// API addresses and then sets them on the APIAddressSetter.
func NewAPIAddressUpdater(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	handler := &APIAddressUpdater{
		config: config,
	}
	w, err := watcher.NewNotifyWorker(watcher.NotifyConfig{
		Handler: handler,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// SetUp is part of the watcher.NotifyHandler interface.
func (c *APIAddressUpdater) SetUp(ctx context.Context) (watcher.NotifyWatcher, error) {
	return c.config.Addresser.WatchAPIHostPorts(ctx)
}

// Handle is part of the watcher.NotifyHandler interface.
func (c *APIAddressUpdater) Handle(ctx context.Context) error {
	hps, err := c.getAddresses(ctx)
	if err != nil {
		return err
	}

	// Logging to identify lp: 1888453
	if len(hps) == 0 {
		c.config.Logger.Warningf(ctx, "empty API host ports received. Updating using existing entries.")
	}

	c.config.Logger.Debugf(ctx, "updating API hostPorts to %+v", hps)
	c.mu.Lock()
	// Protection case to possible help with lp: 1888453
	if len(hps) != 0 {
		c.current = hps
	} else {
		hps = c.current
	}
	c.mu.Unlock()

	// API host/port entries are stored in state as SpaceHostPorts.
	// When retrieved, the space IDs are reconciled so that they are returned
	// as ProviderHostPorts.
	// Here, we indirect them because they are ultimately just stored as dial
	// address strings. This could be re-evaluated in the future if the space
	// information becomes worthwhile to agents.
	hpsToSet := make([]corenetwork.HostPorts, len(hps))
	for i, hps := range hps {
		hpsToSet[i] = hps.HostPorts()
	}

	if err := c.config.Setter.SetAPIHostPorts(hpsToSet); err != nil {
		return fmt.Errorf("error setting addresses: %v", err)
	}
	return nil
}

func (c *APIAddressUpdater) getAddresses(ctx context.Context) ([]corenetwork.ProviderHostPorts, error) {
	addresses, err := c.config.Addresser.APIHostPorts(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting addresses: %v", err)
	}

	// Filter out any LXC or LXD bridge addresses.
	// See LP bugs #1416928 and #1567683.
	hpsToSet := make([]corenetwork.ProviderHostPorts, 0)
	for _, hostPorts := range addresses {
		// Strip ports, filter, then add ports again.
		filtered := network.FilterBridgeAddresses(ctx, hostPorts.Addresses())
		hps := make(corenetwork.ProviderHostPorts, 0, len(filtered))
		for _, hostPort := range hostPorts {
			for _, addr := range filtered {
				if addr.Value == hostPort.Value {
					hps = append(hps, hostPort)
				}
			}
		}
		if len(hps) > 0 {
			hpsToSet = append(hpsToSet, hps)
		}
	}

	// Logging to identify lp: 1888453
	if len(hpsToSet) == 0 {
		c.config.Logger.Warningf(ctx, "get address returning zero results after filtering, non filtered list: %v", addresses)
	}

	return hpsToSet, nil
}

// TearDown is part of the watcher.NotifyHandler interface.
func (c *APIAddressUpdater) TearDown() error {
	return nil
}

// Report shows up in the dependency engine report.
func (c *APIAddressUpdater) Report() map[string]interface{} {
	report := make(map[string]interface{})
	c.mu.Lock()
	defer c.mu.Unlock()
	var servers [][]string
	for _, server := range c.current {
		var addresses []string
		for _, addr := range server {
			addresses = append(addresses, corenetwork.DialAddress(addr))
		}
		servers = append(servers, addresses)
	}
	report["servers"] = servers
	return report
}
