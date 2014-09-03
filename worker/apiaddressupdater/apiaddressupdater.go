// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddressupdater

import (
	"fmt"

	"github.com/juju/loggo"

	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/network"
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
func NewAPIAddressUpdater(addresser APIAddresser, setter APIAddressSetter) worker.Worker {
	return worker.NewNotifyWorker(&APIAddressUpdater{
		addresser: addresser,
		setter:    setter,
	})
}

func (c *APIAddressUpdater) SetUp() (watcher.NotifyWatcher, error) {
	return c.addresser.WatchAPIHostPorts()
}

func (c *APIAddressUpdater) Handle() error {
	addresses, err := c.addresser.APIHostPorts()
	if err != nil {
		return fmt.Errorf("error getting addresses: %v", err)
	}
	if err := c.setter.SetAPIHostPorts(addresses); err != nil {
		return fmt.Errorf("error setting addresses: %v", err)
	}
	logger.Infof("API addresses updated to %q", addresses)
	return nil
}

func (c *APIAddressUpdater) TearDown() error {
	return nil
}
