// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi/v2"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/envcontext"
)

type maasInstance struct {
	machine           gomaasapi.Machine
	constraintMatches gomaasapi.ConstraintMatches
	environ           *maasEnviron
}

func (mi *maasInstance) zone() (string, error) {
	return mi.machine.Zone().Name(), nil
}

func (mi *maasInstance) hostname() (string, error) {
	return mi.machine.Hostname(), nil
}

func (mi *maasInstance) hardwareCharacteristics() (*instance.HardwareCharacteristics, error) {
	nodeArch := strings.Split(mi.machine.Architecture(), "/")[0]
	nodeCpuCount := uint64(mi.machine.CPUCount())
	nodeMemoryMB := uint64(mi.machine.Memory())
	// zone can't error on the maasInstance implementation.
	zone, _ := mi.zone()
	tags := mi.machine.Tags()
	hc := &instance.HardwareCharacteristics{
		Arch:     &nodeArch,
		CpuCores: &nodeCpuCount,
		Mem:      &nodeMemoryMB,
		Tags:     &tags,
	}
	if zone != "" {
		hc.AvailabilityZone = &zone
	}
	return hc, nil
}

func (mi *maasInstance) displayName() (string, error) {
	hostname := mi.machine.Hostname()
	if hostname != "" {
		return hostname, nil
	}
	return mi.machine.FQDN(), nil
}

func (mi *maasInstance) String() string {
	return fmt.Sprintf("%s:%s", mi.machine.Hostname(), mi.machine.SystemID())
}

func (mi *maasInstance) Id() instance.Id {
	return instance.Id(mi.machine.SystemID())
}

func (mi *maasInstance) Addresses(ctx envcontext.ProviderCallContext) (corenetwork.ProviderAddresses, error) {
	subnetsMap, err := mi.environ.subnetToSpaceIds(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Get all the interface details and extract the addresses.
	interfaces, err := maasNetworkInterfaces(ctx, mi, subnetsMap)

	if err != nil {
		return nil, errors.Trace(err)
	}

	var addresses []corenetwork.ProviderAddress
	for _, iface := range interfaces {
		if primAddr := iface.PrimaryAddress(); primAddr.Value != "" {
			addresses = append(addresses, primAddr)
		} else {
			logger.Debugf(ctx, "no address found on interface %q", iface.InterfaceName)
		}
	}

	logger.Debugf(ctx, "%q has addresses %q", mi.machine.Hostname(), addresses)
	return addresses, nil
}

// Status returns a juju status based on the maas instance returned
// status message.
func (mi *maasInstance) Status(ctx envcontext.ProviderCallContext) instance.Status {
	// A fresh status is not obtained here because the interface it is intended
	// to satisfy gets a new maasInstance before each call, using a fresh status
	// would cause us to mask errors since this interface does not contemplate
	// returning them.
	statusName := mi.machine.StatusName()
	statusMsg := mi.machine.StatusMessage()
	return convertInstanceStatus(ctx, statusName, statusMsg, mi.Id())
}

func convertInstanceStatus(ctx context.Context, statusMsg, substatus string, id instance.Id) instance.Status {
	maasInstanceStatus := status.Empty
	switch normalizeStatus(statusMsg) {
	case "":
		logger.Debugf(ctx, "unable to obtain status of instance %s", id)
		statusMsg = "error in getting status"
	case "deployed":
		maasInstanceStatus = status.Running
	case "deploying":
		maasInstanceStatus = status.Allocating
		if substatus != "" {
			statusMsg = fmt.Sprintf("%s: %s", statusMsg, substatus)
		}
	case "failed deployment":
		maasInstanceStatus = status.ProvisioningError
		if substatus != "" {
			statusMsg = fmt.Sprintf("%s: %s", statusMsg, substatus)
		}
	default:
		maasInstanceStatus = status.Empty
		statusMsg = fmt.Sprintf("%s: %s", statusMsg, substatus)
	}
	return instance.Status{
		Status:  maasInstanceStatus,
		Message: statusMsg,
	}
}

func normalizeStatus(statusMsg string) string {
	return strings.ToLower(strings.TrimSpace(statusMsg))
}

// MAAS does not do firewalling so these port methods do nothing.

func (mi *maasInstance) OpenPorts(ctx envcontext.ProviderCallContext, _ string, _ firewall.IngressRules) error {
	logger.Debugf(ctx, "unimplemented OpenPorts() called")
	return nil
}

func (mi *maasInstance) ClosePorts(ctx envcontext.ProviderCallContext, _ string, _ firewall.IngressRules) error {
	logger.Debugf(ctx, "unimplemented ClosePorts() called")
	return nil
}

func (mi *maasInstance) IngressRules(ctx envcontext.ProviderCallContext, _ string) (firewall.IngressRules, error) {
	logger.Debugf(ctx, "unimplemented IngressRules() called")
	return nil, nil
}
