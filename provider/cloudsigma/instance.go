// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"github.com/altoros/gosigma"
	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
)

var _ instances.Instance = (*sigmaInstance)(nil)

type sigmaInstance struct {
	server gosigma.Server
}

var ErrNoDNSName = errors.New("IPv4 address not found")

// Id returns a provider-generated identifier for the Instance.
func (i sigmaInstance) Id() instance.Id {
	id := instance.Id(i.server.UUID())
	logger.Tracef("sigmaInstance.Id: %s", id)
	return id
}

// Status returns the provider-specific status for the instance.
func (i sigmaInstance) Status(ctx context.ProviderCallContext) instance.Status {
	entityStatus := i.server.Status()
	logger.Tracef("sigmaInstance.Status: %s", entityStatus)
	var jujuStatus status.Status
	switch entityStatus {
	case gosigma.ServerStarting:
		jujuStatus = status.Allocating
	case gosigma.ServerRunning:
		jujuStatus = status.Running
	case gosigma.ServerStopping, gosigma.ServerStopped:
		jujuStatus = status.Empty
	case gosigma.ServerUnavailable:
		// I am not sure about this one.
		jujuStatus = status.Pending
	default:
		jujuStatus = status.Pending
	}

	return instance.Status{
		Status:  jujuStatus,
		Message: entityStatus,
	}

}

// Addresses returns a list of hostnames or ip addresses
// associated with the instance. This will supersede DNSName
// which can be implemented by selecting a preferred address.
func (i sigmaInstance) Addresses(ctx context.ProviderCallContext) (corenetwork.ProviderAddresses, error) {
	ip := i.findIPv4()

	if ip != "" {
		addr := corenetwork.ProviderAddress{
			MachineAddress: corenetwork.MachineAddress{
				Value: ip,
				Type:  corenetwork.IPv4Address,
				Scope: corenetwork.ScopePublic,
			},
		}

		logger.Tracef("sigmaInstance.Addresses: %v", addr)

		return []corenetwork.ProviderAddress{addr}, nil
	}
	return []corenetwork.ProviderAddress{}, nil
}

// OpenPorts opens the given ports on the instance, which
// should have been started with the given machine id.
func (i sigmaInstance) OpenPorts(ctx context.ProviderCallContext, machineID string, ports firewall.IngressRules) error {
	return errors.NotImplementedf("OpenPorts")
}

// ClosePorts closes the given ports on the instance, which
// should have been started with the given machine id.
func (i sigmaInstance) ClosePorts(ctx context.ProviderCallContext, machineID string, ports firewall.IngressRules) error {
	return errors.NotImplementedf("ClosePorts")
}

// IngressRules returns the set of ports open on the instance, which
// should have been started with the given machine id.
// The rules are returned as sorted by SortInstanceRules.
func (i sigmaInstance) IngressRules(ctx context.ProviderCallContext, machineID string) (firewall.IngressRules, error) {
	return nil, errors.NotImplementedf("InstanceRules")
}

func (i sigmaInstance) findIPv4() string {
	addrs := i.server.IPv4()
	if len(addrs) == 0 {
		return ""
	}
	return addrs[0]
}

func (i *sigmaInstance) hardware(arch string, driveSize uint64) (*instance.HardwareCharacteristics, error) {
	memory := i.server.Mem() / gosigma.Megabyte
	cores := i.server.SMP()
	cpu := i.server.CPU()
	hw := instance.HardwareCharacteristics{
		Mem:      &memory,
		CpuCores: &cores,
		CpuPower: &cpu,
		Arch:     &arch,
	}

	diskSpace := driveSize / gosigma.Megabyte
	if diskSpace > 0 {
		hw.RootDisk = &diskSpace
	}

	return &hw, nil
}
