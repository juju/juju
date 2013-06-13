// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"fmt"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
)

func newLxcBroker() Broker {
	return &lxcBroker{}
}

type lxcBroker struct {
}

func (broker *lxcBroker) StartInstance(machineId, machineNonce string, series string, cons constraints.Value, info *state.Info, apiInfo *api.Info) (environs.Instance, error) {

	return nil, fmt.Errorf("Not implemented yet")
}

// StopInstances shuts down the given instances.
func (broker *lxcBroker) StopInstances([]environs.Instance) error {
	return fmt.Errorf("Not implemented yet")
}

func (broker *lxcBroker) AllInstances() ([]environs.Instance, error) {
	return nil, fmt.Errorf("Not implemented yet")
}

// Not yet entirely convinced that this is the right place for the instance
// parts, but bas good as any for now.

type lxcInstance struct {
}

// Id returns a provider-generated identifier for the Instance.
func (instance *lxcInstance) Id() state.InstanceId {
}

// DNSName returns the DNS name for the instance.
// If the name is not yet allocated, it will return
// an ErrNoDNSName error.
func (instance *lxcInstance) DNSName() (string, error) {
	return "", environs.ErrNoDNSName
}

// WaitDNSName returns the DNS name for the instance,
// waiting until it is allocated if necessary.
func (instance *lxcInstance) WaitDNSName() (string, error) {
	return "", environs.ErrNoDNSName
}

// OpenPorts opens the given ports on the instance, which
// should have been started with the given machine id.
func (instance *lxcInstance) OpenPorts(machineId string, ports []params.Port) error {
	return fmt.Errorf("not implemented")
}

// ClosePorts closes the given ports on the instance, which
// should have been started with the given machine id.
func (instance *lxcInstance) ClosePorts(machineId string, ports []params.Port) error {
	return fmt.Errorf("not implemented")
}

// Ports returns the set of ports open on the instance, which
// should have been started with the given machine id.
// The ports are returned as sorted by state.SortPorts.
func (instance *lxcInstance) Ports(machineId string) ([]params.Port, error) {
	return nil, fmt.Errorf("not implemented")
}
