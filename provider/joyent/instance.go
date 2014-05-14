// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"github.com/joyent/gosdc/cloudapi"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider/common"
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

func (inst *joyentInstance) Addresses() ([]instance.Address, error) {
	addresses := make([]instance.Address, len(inst.machine.IPs))
	for _, ip := range inst.machine.IPs {
		address := instance.NewAddress(ip, instance.NetworkUnknown)
		if ip == inst.machine.PrimaryIP {
			address.NetworkScope = instance.NetworkPublic
		} else {
			address.NetworkScope = instance.NetworkCloudLocal
		}
		addresses = append(addresses, address)
	}

	return addresses, nil
}

func (inst *joyentInstance) DNSName() (string, error) {
	addresses, err := inst.Addresses()
	if err != nil {
		return "", err
	}
	addr := instance.SelectPublicAddress(addresses)
	if addr == "" {
		return "", instance.ErrNoDNSName
	}
	return addr, nil
}

func (inst *joyentInstance) WaitDNSName() (string, error) {
	return common.WaitDNSName(inst)
}
