// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

type environInstance struct {
	openstackInstance instance.Instance
}

// Id implements instance.Instance.
func (i environInstance) Id() instance.Id {
	return i.openstackInstance.Id()
}

// Status implements instance.Instance.
func (i environInstance) Status() string {
	return i.openstackInstance.Status()
}

// Refresh implements instance.Instance.
func (i environInstance) Refresh() error {
	return i.openstackInstance.Refresh()
}

// Addresses implements instance.Instance.
func (i environInstance) Addresses() ([]network.Address, error) {
	return i.openstackInstance.Addresses()
}

// OpenPorts implements instance.Instance.
func (i environInstance) OpenPorts(machineId string, ports []network.PortRange) error {
	return i.openstackInstance.OpenPorts(machineId, ports)
}

// ClosePorts implements instance.Instance.
func (i environInstance) ClosePorts(machineId string, ports []network.PortRange) error {
	return i.openstackInstance.ClosePorts(machineId, ports)
}

// Ports implements instance.Instance.
func (i environInstance) Ports(machineId string) ([]network.PortRange, error) {
	return i.openstackInstance.Ports(machineId)
}
