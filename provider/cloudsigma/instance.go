// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"fmt"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"

	"github.com/Altoros/gosigma"
)

type sigmaInstance struct {
	server gosigma.Server
}

var ErrNoDNSName = fmt.Errorf("IPv4 address not found")

// Id returns a provider-generated identifier for the Instance.
func (i sigmaInstance) Id() instance.Id {
	if i.server == nil {
		return instance.Id("")
	}
	id := instance.Id(i.server.UUID())
	logger.Tracef("sigmaInstance.Id: %s", id)
	return id
}

// Status returns the provider-specific status for the instance.
func (i sigmaInstance) Status() string {
	if i.server == nil {
		return ""
	}
	status := i.server.Status()
	logger.Tracef("sigmaInstance.Status: %s", status)
	return status
}

// Refresh refreshes local knowledge of the instance from the provider.
func (i sigmaInstance) Refresh() error {
	if i.server == nil {
		return fmt.Errorf("invalid instance")
	}
	err := i.server.Refresh()
	logger.Tracef("sigmaInstance.Refresh: %s", err)
	return err
}

// Addresses returns a list of hostnames or ip addresses
// associated with the instance. This will supercede DNSName
// which can be implemented by selecting a preferred address.
func (i sigmaInstance) Addresses() ([]network.Address, error) {

	ip := i.findIPv4()

	if ip == "" {
		logger.Tracef("IPv4 address not found")
		return nil, ErrNoDNSName
	}

	addr := network.Address{
		Value: ip,
		Type:  network.IPv4Address,
		Scope: network.ScopePublic,
	}

	logger.Tracef("sigmaInstance.Addresses: %v", addr)

	return []network.Address{addr}, nil
}

// DNSName returns the DNS name for the instance.
// If the name is not yet allocated, it will return
// an ErrNoDNSName error.
func (i sigmaInstance) DNSName() (string, error) {
	ip := i.findIPv4()

	if ip == "" {
		logger.Tracef("sigmaInstance.DNSName: IPv4 address not found, refreshing...")
		if err := i.Refresh(); err != nil {
			return "", err
		}

		ip = i.findIPv4()
		if ip == "" {
			return "", ErrNoDNSName
		}
	}

	logger.Infof("sigmaInstance.DNSName: %s", ip)

	return ip, nil
}

/* TODO REMOVE
// WaitDNSName returns the DNS name for the instance,
// waiting until it is allocated if necessary.
// TODO: We may not need this in the interface any more.  All
// implementations now delegate to environs.WaitDNSName.
func (i sigmaInstance) WaitDNSName() (string, error) {
	return common.WaitDNSName(i)
}
*/

// OpenPorts opens the given ports on the instance, which
// should have been started with the given machine id.
func (i sigmaInstance) OpenPorts(machineID string, ports []network.Port) error {
	logger.Tracef("sigmaInstance.OpenPorts: not implemented")
	return nil
}

// ClosePorts closes the given ports on the instance, which
// should have been started with the given machine id.
func (i sigmaInstance) ClosePorts(machineID string, ports []network.Port) error {
	logger.Tracef("sigmaInstance.ClosePorts: not implemented")
	return nil
}

// Ports returns the set of ports open on the instance, which
// should have been started with the given machine id.
// The ports are returned as sorted by SortPorts.
func (i sigmaInstance) Ports(machineID string) ([]network.Port, error) {
	logger.Tracef("sigmaInstance.Ports: not implemented")
	return []network.Port{}, nil
}

func (i sigmaInstance) findIPv4() string {
	if i.server == nil {
		return ""
	}
	addrs := i.server.IPv4()
	if len(addrs) == 0 {
		return ""
	}
	return addrs[0]
}

func (i sigmaInstance) hardware() *instance.HardwareCharacteristics {
	if i.server == nil {
		return nil
	}
	memory := i.server.Mem() / gosigma.Megabyte
	cores := uint64(i.server.SMP())
	cpu := i.server.CPU()
	hw := instance.HardwareCharacteristics{
		Mem:      &memory,
		CpuCores: &cores,
		CpuPower: &cpu,
	}
	return &hw
}
