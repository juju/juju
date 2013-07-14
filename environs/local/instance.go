// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"fmt"

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
	return "", instance.ErrNoDNSName
}

// WaitDNSName implements instance.Instance.WaitDNSName.
func (inst *localInstance) WaitDNSName() (string, error) {
	return "", instance.ErrNoDNSName
}

// OpenPorts implements instance.Instance.OpenPorts.
func (inst *localInstance) OpenPorts(machineId string, ports []instance.Port) error {
	return fmt.Errorf("instance open ports not implemented")
}

// ClosePorts implements instance.Instance.ClosePorts.
func (inst *localInstance) ClosePorts(machineId string, ports []instance.Port) error {
	return fmt.Errorf("instance close not implemented")
}

// Ports implements instance.Instance.Ports.
func (inst *localInstance) Ports(machineId string) ([]instance.Port, error) {
	return nil, nil
}

// Add a string representation of the id.
func (inst *localInstance) String() string {
	return fmt.Sprintf("inst:%v", inst.id)
}
