// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"fmt"

	"launchpad.net/juju-core/instance"
)

type kvmInstance struct {
	container Container
	id        string
}

var _ instance.Instance = (*kvmInstance)(nil)

// Id implements instance.Instance.Id.
func (kvm *kvmInstance) Id() instance.Id {
	return instance.Id(kvm.id)
}

// Status implements instance.Instance.Status.
func (kvm *kvmInstance) Status() string {
	if kvm.container.IsRunning() {
		return "running"
	}
	return "stopped"
}

func (*kvmInstance) Refresh() error {
	return nil
}

func (kvm *kvmInstance) Addresses() ([]instance.Address, error) {
	logger.Errorf("kvmInstance.Addresses not implemented")
	return nil, nil
}

// DNSName implements instance.Instance.DNSName.
func (kvm *kvmInstance) DNSName() (string, error) {
	return "", instance.ErrNoDNSName
}

// WaitDNSName implements instance.Instance.WaitDNSName.
func (kvm *kvmInstance) WaitDNSName() (string, error) {
	return "", instance.ErrNoDNSName
}

// OpenPorts implements instance.Instance.OpenPorts.
func (kvm *kvmInstance) OpenPorts(machineId string, ports []instance.Port) error {
	return fmt.Errorf("not implemented")
}

// ClosePorts implements instance.Instance.ClosePorts.
func (kvm *kvmInstance) ClosePorts(machineId string, ports []instance.Port) error {
	return fmt.Errorf("not implemented")
}

// Ports implements instance.Instance.Ports.
func (kvm *kvmInstance) Ports(machineId string) ([]instance.Port, error) {
	return nil, fmt.Errorf("not implemented")
}

// Add a string representation of the id.
func (kvm *kvmInstance) String() string {
	return fmt.Sprintf("kvm:%s", kvm.id)
}
