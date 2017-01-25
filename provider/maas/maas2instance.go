// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"fmt"
	"strings"

	"github.com/juju/gomaasapi"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

type maas2Instance struct {
	machine           gomaasapi.Machine
	constraintMatches gomaasapi.ConstraintMatches
}

var _ maasInstance = (*maas2Instance)(nil)

func (mi *maas2Instance) zone() (string, error) {
	return mi.machine.Zone().Name(), nil
}

func (mi *maas2Instance) hostname() (string, error) {
	return mi.machine.Hostname(), nil
}

func (mi *maas2Instance) hardwareCharacteristics() (*instance.HardwareCharacteristics, error) {
	nodeArch := strings.Split(mi.machine.Architecture(), "/")[0]
	nodeCpuCount := uint64(mi.machine.CPUCount())
	nodeMemoryMB := uint64(mi.machine.Memory())
	// zone can't error on the maas2Instance implementaation.
	zone, _ := mi.zone()
	tags := mi.machine.Tags()
	hc := &instance.HardwareCharacteristics{
		Arch:             &nodeArch,
		CpuCores:         &nodeCpuCount,
		Mem:              &nodeMemoryMB,
		AvailabilityZone: &zone,
		Tags:             &tags,
	}
	return hc, nil
}

func (mi *maas2Instance) String() string {
	return fmt.Sprintf("%s:%s", mi.machine.Hostname(), mi.machine.SystemID())
}

func (mi *maas2Instance) Id() instance.Id {
	return instance.Id(mi.machine.SystemID())
}

func (mi *maas2Instance) Addresses() ([]network.Address, error) {
	machineAddresses := mi.machine.IPAddresses()
	addresses := make([]network.Address, len(machineAddresses))
	for i, address := range machineAddresses {
		addresses[i] = network.NewAddress(address)
	}
	return addresses, nil
}

// Status returns a juju status based on the maas instance returned
// status message.
func (mi *maas2Instance) Status() instance.InstanceStatus {
	// A fresh status is not obtained here because the interface it is intended
	// to satisfy gets a new maas2Instance before each call, using a fresh status
	// would cause us to mask errors since this interface does not contemplate
	// returing them.
	statusName := mi.machine.StatusName()
	statusMsg := mi.machine.StatusMessage()
	return convertInstanceStatus(statusName, statusMsg, mi.Id())
}

// MAAS does not do firewalling so these port methods do nothing.
func (mi *maas2Instance) OpenPorts(machineId string, rules []network.IngressRule) error {
	logger.Debugf("unimplemented OpenPorts() called")
	return nil
}

func (mi *maas2Instance) ClosePorts(machineId string, rules []network.IngressRule) error {
	logger.Debugf("unimplemented ClosePorts() called")
	return nil
}

func (mi *maas2Instance) IngressRules(machineId string) ([]network.IngressRule, error) {
	logger.Debugf("unimplemented Rules() called")
	return nil, nil
}
