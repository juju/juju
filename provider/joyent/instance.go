// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"fmt"
	"strings"
	"time"

	"launchpad.net/gojoyent/cloudapi"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider/common"
)

type joyentInstance struct {
	machine *cloudapi.Machine
	env     *JoyentEnviron
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
	logger.Debugf("Getting Addresses for instance %q, state %s", inst.Id(), inst.machine.State)
	addresses := make([]instance.Address, len(inst.machine.IPs))
	logger.Debugf("Got addresses %q", inst.machine.IPs)
	for _, ip := range inst.machine.IPs {
		address := instance.NewAddress(ip)
		if ip == inst.machine.PrimaryIP {
			address.NetworkScope = instance.NetworkPublic
		} else {
			address.NetworkScope = instance.NetworkCloudLocal
		}
		addresses = append(addresses, address)
	}

	//logger.Debugf("Return addresses %q", addresses)

	return addresses, nil
}

func (inst *joyentInstance) DNSName() (string, error) {
	logger.Debugf("Getting DNS Name for instance %q", inst.Id())
	addresses, err := inst.Addresses()
	//logger.Debugf("Got addresses %q, err %q", addresses, err)
	if err != nil {
		return "", err
	}
	addr := instance.SelectPublicAddress(addresses)
	//logger.Debugf("Got public addresses %q, err %q", addr, err)
	if addr == "" {
		return "", instance.ErrNoDNSName
	}
	return addr, nil
}

func (inst *joyentInstance) WaitDNSName() (string, error) {
	return common.WaitDNSName(inst)
}

// Stop will stop and delete the machine
// Stopped machines are still billed for in the Joyent Public Cloud
func (inst *joyentInstance) Stop() error {
	id := string(inst.Id())

	// wait for machine to be running
	// if machine is still provisioning stop will fail
	for !inst.pollMachineState(id, "running") {
		time.Sleep(1 * time.Second)
	}

	err := inst.env.compute.cloudapi.StopMachine(id)
	if err != nil {
		return fmt.Errorf("cannot stop instance %s: %v", id, err)
	}

	// wait for machine to be stopped
	for !inst.pollMachineState(id, "stopped") {
		time.Sleep(1 * time.Second)
	}

	err = inst.env.compute.cloudapi.DeleteMachine(id)
	if err != nil {
		return fmt.Errorf("cannot delete instance %s: %v", id, err)
	}

	return nil
}

func (inst *joyentInstance) pollMachineState(machineId, state string) bool {
	machineConfig, err := inst.env.compute.cloudapi.GetMachine(machineId)
	if err != nil {
		return false
	}
	return strings.EqualFold(machineConfig.State, state)
}
