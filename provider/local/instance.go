// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/instance"
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
			Value:        inst.env.bridgeAddress,
		}}
		return addrs, nil
	}
	return nil, errors.NotImplementedf("localInstance.Addresses")
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
