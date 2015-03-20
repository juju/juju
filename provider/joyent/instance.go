// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"github.com/joyent/gosdc/cloudapi"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

type joyentInstance struct {
	machine *cloudapi.Machine
	env     *joyentEnviron
}

var _ instance.Instance = (*joyentInstance)(nil)

func (inst *joyentInstance) Id() instance.Id {
	return instance.Id(inst.machine.Id)
}

func (inst *joyentInstance) Status() string {
	return inst.machine.State
}

func (inst *joyentInstance) Refresh() error {
	return nil
}

func (inst *joyentInstance) Addresses() ([]network.Address, error) {
	addresses := make([]network.Address, 0, len(inst.machine.IPs))
	for _, ip := range inst.machine.IPs {
		address := network.NewAddress(ip)
		if ip == inst.machine.PrimaryIP {
			address.Scope = network.ScopePublic
		} else {
			address.Scope = network.ScopeCloudLocal
		}
		addresses = append(addresses, address)
	}

	return addresses, nil
}
