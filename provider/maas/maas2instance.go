// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi/v2"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/environs/context"
)

type maas2Instance struct {
	machine           gomaasapi.Machine
	constraintMatches gomaasapi.ConstraintMatches
	environ           *maasEnviron
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

func (mi *maas2Instance) displayName() (string, error) {
	hostname := mi.machine.Hostname()
	if hostname != "" {
		return hostname, nil
	}
	return mi.machine.FQDN(), nil
}

func (mi *maas2Instance) String() string {
	return fmt.Sprintf("%s:%s", mi.machine.Hostname(), mi.machine.SystemID())
}

func (mi *maas2Instance) Id() instance.Id {
	return instance.Id(mi.machine.SystemID())
}

func (mi *maas2Instance) Addresses(ctx context.ProviderCallContext) (corenetwork.ProviderAddresses, error) {
	subnetsMap, err := mi.environ.subnetToSpaceIds(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Get all the interface details and extract the addresses.
	interfaces, err := maas2NetworkInterfaces(ctx, mi, subnetsMap)

	if err != nil {
		return nil, errors.Trace(err)
	}

	var addresses []corenetwork.ProviderAddress
	for _, iface := range interfaces {
		if primAddr := iface.PrimaryAddress(); primAddr.Value != "" {
			addresses = append(addresses, primAddr)
		} else {
			logger.Debugf("no address found on interface %q", iface.InterfaceName)
		}
	}

	logger.Debugf("%q has addresses %q", mi.machine.Hostname(), addresses)
	return addresses, nil
}

// Status returns a juju status based on the maas instance returned
// status message.
func (mi *maas2Instance) Status(ctx context.ProviderCallContext) instance.Status {
	// A fresh status is not obtained here because the interface it is intended
	// to satisfy gets a new maas2Instance before each call, using a fresh status
	// would cause us to mask errors since this interface does not contemplate
	// returing them.
	statusName := mi.machine.StatusName()
	statusMsg := mi.machine.StatusMessage()
	return convertInstanceStatus(statusName, statusMsg, mi.Id())
}

// MAAS does not do firewalling so these port methods do nothing.
func (mi *maas2Instance) OpenPorts(ctx context.ProviderCallContext, machineId string, rules firewall.IngressRules) error {
	logger.Debugf("unimplemented OpenPorts() called")
	return nil
}

func (mi *maas2Instance) ClosePorts(ctx context.ProviderCallContext, machineId string, rules firewall.IngressRules) error {
	logger.Debugf("unimplemented ClosePorts() called")
	return nil
}

func (mi *maas2Instance) IngressRules(ctx context.ProviderCallContext, machineId string) (firewall.IngressRules, error) {
	logger.Debugf("unimplemented IngressRules() called")
	return nil, nil
}
