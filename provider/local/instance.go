// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"fmt"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/instance"
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

func (inst *localInstance) Addresses() ([]instance.Address, error) {
	logger.Errorf("localInstance.Addresses not implemented")
	return nil, nil
}

// DNSName implements instance.Instance.DNSName.
func (inst *localInstance) DNSName() (string, error) {
	if string(inst.id) == "localhost" {
		// get the bridge address from the environment
		addr, err := inst.env.findBridgeAddress()
		if err != nil {
			logger.Errorf("failed to get bridge address: %v", err)
			return "", instance.ErrNoDNSName
		}
		return addr, nil
	}
	// Get the IPv4 address from eth0
	return getAddressForInterface("eth0")
}

// WaitDNSName implements instance.Instance.WaitDNSName.
func (inst *localInstance) WaitDNSName() (string, error) {
	return environs.WaitDNSName(inst)
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
