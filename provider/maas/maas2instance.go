// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"github.com/juju/errors"
	"github.com/juju/gomaasapi"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

type maas2Instance struct {
	machine    gomaasapi.Machine
	controller gomaasapi.Controller
}

var _ instance.Instance = (*maas2Instance)(nil)

func (mi *maas2Instance) String() string {
	return mi.machine.Hostname()
}

func (mi *maas2Instance) Id() instance.Id {
	// TODO (mfoord): this should be machine.URI() but that isn't implemented
	// yet.
	return instance.Id(mi.machine.SystemId())
}

func (mi *maas2Instance) Addresses() ([]network.Address, error) {
	return nil, errors.New("write me or bite me")
}

// Status returns a juju status based on the maas instance returned
// status message.
func (mi *maas2Instance) Status() instance.InstanceStatus {
	var statusMsg, statusName string
	err := mi.refresh()
	if err != nil {
		// The instanceStatusConverter will turn these into an appropriate
		// error status.
		statusMsg = ""
		statusName = ""

	} else {
		statusName = mi.machine.StatusName()
		statusMsg = mi.machine.StatusMessage()
	}
	return convertInstanceStatus(statusMsg, statusName, mi.Id())
}

func (mi *maas2Instance) refresh() error {
	// XXXX refresh the machine, that requires being able to fetch a machine by
	// id from the controller which isn't yet implemented.
	return nil
}

// MAAS does not do firewalling so these port methods do nothing.
func (mi *maas2Instance) OpenPorts(machineId string, ports []network.PortRange) error {
	logger.Debugf("unimplemented OpenPorts() called")
	return nil
}

func (mi *maas2Instance) ClosePorts(machineId string, ports []network.PortRange) error {
	logger.Debugf("unimplemented ClosePorts() called")
	return nil
}

func (mi *maas2Instance) Ports(machineId string) ([]network.PortRange, error) {
	logger.Debugf("unimplemented Ports() called")
	return nil, nil
}
