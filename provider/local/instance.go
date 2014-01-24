// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"fmt"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider/common"
)

type localInstance struct {
	id  instance.Id
	env *localEnviron
}

var _ instance.Instance = (*localInstance)(nil)

// Id implements instance.Instance.Id.
func (inst *localInstance) Id() instance.Id {
	return inst.id
}

// Status implements instance.Instance.Status.
func (inst *localInstance) Status() string {
	return ""
}

func (*localInstance) Refresh() error {
	return nil
}

func (inst *localInstance) Addresses() ([]instance.Address, error) {
	if inst.id == bootstrapInstanceId {
		addrs := []instance.Address{{
			NetworkScope: instance.NetworkPublic,
			Type:         instance.HostName,
			Value:        "localhost",
		}, {
			NetworkScope: instance.NetworkCloudLocal,
			Type:         instance.Ipv4Address,
			Value:        inst.env.config.bootstrapIPAddress(),
		}}
		return addrs, nil
	}
	return nil, errors.NewNotImplementedError("localInstance.Addresses")
}

// DNSName implements instance.Instance.DNSName.
func (inst *localInstance) DNSName() (string, error) {
	if inst.id == bootstrapInstanceId {
		return inst.env.config.bootstrapIPAddress(), nil
	}
	// Get the IPv4 address from eth0
	return getAddressForInterface("eth0")
}

// WaitDNSName implements instance.Instance.WaitDNSName.
func (inst *localInstance) WaitDNSName() (string, error) {
	return common.WaitDNSName(inst)
}

// OpenPorts implements instance.Instance.OpenPorts.
func (inst *localInstance) OpenPorts(machineId string, ports []instance.Port) error {
	logger.Infof("OpenPorts called for %s:%v", machineId, ports)
	return nil
}

// ClosePorts implements instance.Instance.ClosePorts.
func (inst *localInstance) ClosePorts(machineId string, ports []instance.Port) error {
	logger.Infof("ClosePorts called for %s:%v", machineId, ports)
	return nil
}

// Ports implements instance.Instance.Ports.
func (inst *localInstance) Ports(machineId string) ([]instance.Port, error) {
	return nil, nil
}

// Add a string representation of the id.
func (inst *localInstance) String() string {
	return fmt.Sprintf("inst:%v", inst.id)
}
