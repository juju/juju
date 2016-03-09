// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/tools/lxdclient"
)

type lxdInstance struct {
	id     string
	client *lxdclient.Client
}

var _ instance.Instance = (*lxdInstance)(nil)

// Id implements instance.Instance.Id.
func (lxd *lxdInstance) Id() instance.Id {
	return instance.Id(lxd.id)
}

func (*lxdInstance) Refresh() error {
	return nil
}

func (lxd *lxdInstance) Addresses() ([]network.Address, error) {
	return nil, errors.NotImplementedf("lxdInstance.Addresses")
}

// Status implements instance.Instance.Status.
func (lxd *lxdInstance) Status() string {
	// On error, the state will be "unknown".
	status, err := lxd.client.Status(lxd.id)
	if err != nil {
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

// Add a string representation of the id.
func (lxd *lxdInstance) String() string {
	return fmt.Sprintf("lxd:%s", lxd.id)
}
