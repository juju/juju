// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddressupdater

import (
	"fmt"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/worker.v1"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/network"
)

var logger = loggo.GetLogger("juju.worker.apiaddressupdater")

// APIAddressUpdater is responsible for propagating API addresses.
//
// In practice, APIAddressUpdater is used by a machine agent to watch
// API addresses in state and write the changes to the agent's config file.
type APIAddressUpdater struct {
	addresser APIAddresser
	setter    APIAddressSetter

	mu      sync.Mutex
	current [][]corenetwork.HostPort
}

// APIAddresser is an interface that is provided to NewAPIAddressUpdater
// which can be used to watch for API address changes.
type APIAddresser interface {
	APIHostPorts() ([][]corenetwork.HostPort, error)
	WatchAPIHostPorts() (watcher.NotifyWatcher, error)
}

// APIAddressSetter is an interface that is provided to NewAPIAddressUpdater
// whose SetAPIHostPorts method will be invoked whenever address changes occur.
type APIAddressSetter interface {
	SetAPIHostPorts(servers [][]corenetwork.HostPort) error
}

// NewAPIAddressUpdater returns a worker.Worker that watches for changes to
// API addresses and then sets them on the APIAddressSetter.
// TODO(fwereade): this should have a config struct, and some validation.
func NewAPIAddressUpdater(addresser APIAddresser, setter APIAddressSetter) (worker.Worker, error) {
	handler := &APIAddressUpdater{
		addresser: addresser,
		setter:    setter,
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
func (c *APIAddressUpdater) SetUp() (watcher.NotifyWatcher, error) {
	return c.addresser.WatchAPIHostPorts()
}

// Handle is part of the watcher.NotifyHandler interface.
func (c *APIAddressUpdater) Handle(_ <-chan struct{}) error {
	hpsToSet, err := c.getAddresses()
	if err != nil {
		return err
	}
	logger.Debugf("updating API hostPorts to %+v", hpsToSet)
	c.mu.Lock()
	c.current = hpsToSet
	c.mu.Unlock()
	if err := c.setter.SetAPIHostPorts(hpsToSet); err != nil {
		return fmt.Errorf("error setting addresses: %v", err)
	}
	return nil
}

func (c *APIAddressUpdater) getAddresses() ([][]corenetwork.HostPort, error) {
	addresses, err := c.addresser.APIHostPorts()
	if err != nil {
		return nil, fmt.Errorf("error getting addresses: %v", err)
	}

	// Filter out any LXC or LXD bridge addresses. See LP bug #1416928. and
	// bug #1567683
	hpsToSet := make([][]corenetwork.HostPort, 0, len(addresses))
	for _, hostPorts := range addresses {
		// Strip ports, filter, then add ports again.
		filtered := network.FilterBridgeAddresses(corenetwork.HostsWithoutPort(hostPorts))
		hps := make([]corenetwork.HostPort, 0, len(filtered))
		for _, hostPort := range hostPorts {
			for _, addr := range filtered {
				if addr.Value == hostPort.Address.Value {
					hps = append(hps, hostPort)
				}
			}
		}
		if len(hps) > 0 {
			hpsToSet = append(hpsToSet, hps)
		}
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
			addresses = append(addresses, addr.String())
		}
		servers = append(servers, addresses)
	}
	report["servers"] = servers
	return report
}
