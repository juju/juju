// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc

import (
	"fmt"

	"launchpad.net/juju-core/instance"
)

type lxcInstance struct {
	id string
}

// Id returns a provider-generated identifier for the Instance.
func (lxc *lxcInstance) Id() instance.Id {
	return instance.Id(lxc.id)
}

// DNSName returns the DNS name for the instance.
// If the name is not yet allocated, it will return
// an ErrNoDNSName error.
func (lxc *lxcInstance) DNSName() (string, error) {
	return "", instance.ErrNoDNSName
}

// WaitDNSName returns the DNS name for the instance,
// waiting until it is allocated if necessary.
func (lxc *lxcInstance) WaitDNSName() (string, error) {
	return "", instance.ErrNoDNSName
}

// OpenPorts opens the given ports on the instance, which
// should have been started with the given machine id.
func (lxc *lxcInstance) OpenPorts(machineId string, ports []instance.Port) error {
	return fmt.Errorf("not implemented")
}

// ClosePorts closes the given ports on the instance, which
// should have been started with the given machine id.
func (lxc *lxcInstance) ClosePorts(machineId string, ports []instance.Port) error {
	return fmt.Errorf("not implemented")
}

// Ports returns the set of ports open on the instance, which
// should have been started with the given machine id.
// The ports are returned as sorted by state.SortPorts.
func (lxc *lxcInstance) Ports(machineId string) ([]instance.Port, error) {
	return nil, fmt.Errorf("not implemented")
}

// Metadata returns the characteristics of the instance.
func (lxc *lxcInstance) Metadata() *instance.Metadata {
	return nil
}
