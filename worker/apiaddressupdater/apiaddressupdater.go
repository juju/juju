// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddressupdater

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/network"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.apiaddressupdater")

// APIAddressUpdater is responsible for propagating API addresses.
//
// In practice, APIAddressUpdater is used by a machine agent to watch
// API addresses in state and write the changes to the agent's config file.
type APIAddressUpdater struct {
	addresser APIAddresser
	setter    APIAddressSetter
}

// APIAddresser is an interface that is provided to NewAPIAddressUpdater
// which can be used to watch for API address changes.
type APIAddresser interface {
	APIHostPorts() ([][]network.HostPort, error)
	WatchAPIHostPorts() (watcher.NotifyWatcher, error)
}

// APIAddressSetter is an interface that is provided to NewAPIAddressUpdater
// whose SetAPIHostPorts method will be invoked whenever address changes occur.
type APIAddressSetter interface {
	SetAPIHostPorts(servers [][]network.HostPort) error
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
	addresses, err := c.addresser.APIHostPorts()
	if err != nil {
		return fmt.Errorf("error getting addresses: %v", err)
	}

	// Filter out any LXC bridge addresses. See LP bug #1416928.
	hpsToSet := make([][]network.HostPort, 0, len(addresses))
	for _, hostPorts := range addresses {
		// First try to keep only addresses in the default space where all API servers are on.
		defaultSpaceHP, ok := network.SelectHostPortBySpace(hostPorts, network.DefaultSpace)
		if ok {
			hpsToSet = append(hpsToSet, []network.HostPort{defaultSpaceHP})
			continue
		} else {
			// As a fallback, use the old behavior.
			logger.Warningf("cannot determine API addresses by space %q (using all as fallback)", network.DefaultSpace)
		}

		// Strip ports, filter, then add ports again.
		filtered := network.FilterLXCAddresses(network.HostsWithoutPort(hostPorts))
		hps := make([]network.HostPort, 0, len(filtered))
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
	logger.Debugf("updating API hostPorts to %+v", hpsToSet)
	if err := c.setter.SetAPIHostPorts(hpsToSet); err != nil {
		return fmt.Errorf("error setting addresses: %v", err)
	}
	return nil
}

// TearDown is part of the watcher.NotifyHandler interface.
func (c *APIAddressUpdater) TearDown() error {
	return nil
}
