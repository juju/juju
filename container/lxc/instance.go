// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc

import (
	"fmt"

	"github.com/juju/errors"
	"launchpad.net/golxc"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

type lxcInstance struct {
	golxc.Container
	id string
}

var _ instance.Instance = (*lxcInstance)(nil)

// Id implements instance.Instance.Id.
func (lxc *lxcInstance) Id() instance.Id {
	return instance.Id(lxc.id)
}

// Status implements instance.Instance.Status.
func (lxc *lxcInstance) Status() string {
	// On error, the state will be "unknown".
	state, _, _ := lxc.Info()
	return string(state)
}

func (*lxcInstance) Refresh() error {
	return nil
}

func (lxc *lxcInstance) Addresses() ([]network.Address, error) {
	return nil, errors.NotImplementedf("lxcInstance.Addresses")
}

// OpenPorts implements instance.Instance.OpenPorts.
func (lxc *lxcInstance) OpenPorts(machineId string, ports []network.PortRange) error {
	return fmt.Errorf("not implemented")
}

// ClosePorts implements instance.Instance.ClosePorts.
func (lxc *lxcInstance) ClosePorts(machineId string, ports []network.PortRange) error {
	return fmt.Errorf("not implemented")
}

// Ports implements instance.Instance.Ports.
func (lxc *lxcInstance) Ports(machineId string) ([]network.PortRange, error) {
	return nil, fmt.Errorf("not implemented")
}

// Add a string representation of the id.
func (lxc *lxcInstance) String() string {
	return fmt.Sprintf("lxc:%s", lxc.id)
}
