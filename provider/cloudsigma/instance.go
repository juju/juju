// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"github.com/altoros/gosigma"
	"github.com/juju/errors"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/status"
)

var _ instance.Instance = (*sigmaInstance)(nil)

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
func (i sigmaInstance) Status() instance.InstanceStatus {
	entityStatus := i.server.Status()
	logger.Tracef("sigmaInstance.Status: %s", entityStatus)
	jujuStatus := status.Pending
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

	return instance.InstanceStatus{
		Status:  jujuStatus,
		Message: entityStatus,
	}

}

// Addresses returns a list of hostnames or ip addresses
// associated with the instance. This will supercede DNSName
// which can be implemented by selecting a preferred address.
func (i sigmaInstance) Addresses() ([]network.Address, error) {
	ip := i.findIPv4()

	if ip != "" {
		addr := network.Address{
			Value: ip,
			Type:  network.IPv4Address,
			Scope: network.ScopePublic,
		}

		logger.Tracef("sigmaInstance.Addresses: %v", addr)

		return []network.Address{addr}, nil
	}
	return []network.Address{}, nil
}

// OpenPorts opens the given ports on the instance, which
// should have been started with the given machine id.
func (i sigmaInstance) OpenPorts(machineID string, ports []network.PortRange) error {
	return errors.NotImplementedf("OpenPorts")
}

// ClosePorts closes the given ports on the instance, which
// should have been started with the given machine id.
func (i sigmaInstance) ClosePorts(machineID string, ports []network.PortRange) error {
	return errors.NotImplementedf("ClosePorts")
}

// Ports returns the set of ports open on the instance, which
// should have been started with the given machine id.
// The ports are returned as sorted by SortPorts.
func (i sigmaInstance) Ports(machineID string) ([]network.PortRange, error) {
	return nil, errors.NotImplementedf("Ports")
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
	cores := uint64(i.server.SMP())
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
