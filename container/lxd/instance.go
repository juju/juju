// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/container/lxd/lxdclient"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

// TODO(ericsnow) Move this check to a test suite.
var _ instance.Instance = (*lxdInstance)(nil)

// lxdInstance implements instance.Instance.
type lxdInstance struct {
	raw    *lxdclient.Instance
	client *lxdclient.Client
}

// String returns a string representation of the instance, based on its ID.
func (lxd *lxdInstance) String() string {
	return fmt.Sprintf("lxd:%s", lxd.raw.Name)
}

// Id implements instance.Instance.Id.
func (lxd *lxdInstance) Id() instance.Id {
	return instance.Id(lxd.raw.Name)
}

// Addresses implements instance.Instance.Id.
func (lxd *lxdInstance) Addresses() ([]network.Address, error) {
	return nil, errors.NotImplementedf("lxdInstance.Addresses")
}

// Status implements instance.Instance.Status.
func (lxd *lxdInstance) Status() string {
	// TODO(ericsnow) Don't bother with dynamic update?

	// On error, the state will be "unknown".
	status, err := lxd.raw.CurrentStatus(lxd.client)
	if err != nil {
		logger.Errorf("could not get status for LXD container %q: %v", lxd.raw.Name, err)
		// TODO(ericsnow) Fall back to the last known status?
		return "unknown"
	}

	return status
}

// OpenPorts implements instance.Instance.OpenPorts.
func (lxd *lxdInstance) OpenPorts(machineId string, ports []network.PortRange) error {
	return fmt.Errorf("not implemented")
}

// ClosePorts implements instance.Instance.ClosePorts.
func (lxd *lxdInstance) ClosePorts(machineId string, ports []network.PortRange) error {
	return fmt.Errorf("not implemented")
}

// Ports implements instance.Instance.Ports.
func (lxd *lxdInstance) Ports(machineId string) ([]network.PortRange, error) {
	return nil, fmt.Errorf("not implemented")
}
