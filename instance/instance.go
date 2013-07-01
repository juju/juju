// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance

import (
	"errors"
	"fmt"
	"strings"
)

type ContainerType string

const (
	NONE = ContainerType("none")
	LXC  = ContainerType("lxc")
	KVM  = ContainerType("kvm")
)

// SupportedContainerTypes is used to validate add-machine arguments.
var SupportedContainerTypes []ContainerType = []ContainerType{
	LXC,
}

// ParseSupportedContainerType converts the specified string into a supported
// ContainerType instance or returns an error if the container type is invalid.
// For this version of the function, 'none' is a valid value.
func ParseSupportedContainerTypeOrNone(ctype string) (ContainerType, error) {
	if ContainerType(ctype) == NONE {
		return NONE, nil
	}
	return ParseSupportedContainerType(ctype)
}

// ParseSupportedContainerType converts the specified string into a supported
// ContainerType instance or returns an error if the container type is invalid.
func ParseSupportedContainerType(ctype string) (ContainerType, error) {
	for _, supportedType := range SupportedContainerTypes {
		if ContainerType(ctype) == supportedType {
			return supportedType, nil
		}
	}
	return "", fmt.Errorf("invalid container type %q", ctype)
}

var ErrNoDNSName = errors.New("DNS name not allocated")

// An instance Id is a provider-specific identifier associated with an
// instance (physical or virtual machine allocated in the provider).
type Id string

// Port identifies a network port number for a particular protocol.
type Port struct {
	Protocol string
	Number   int
}

func (p Port) String() string {
	return fmt.Sprintf("%s:%d", p.Protocol, p.Number)
}

// Instance represents the the realization of a machine in state.
type Instance interface {
	// Id returns a provider-generated identifier for the Instance.
	Id() Id

	// DNSName returns the DNS name for the instance.
	// If the name is not yet allocated, it will return
	// an ErrNoDNSName error.
	DNSName() (string, error)

	// WaitDNSName returns the DNS name for the instance,
	// waiting until it is allocated if necessary.
	WaitDNSName() (string, error)

	// OpenPorts opens the given ports on the instance, which
	// should have been started with the given machine id.
	OpenPorts(machineId string, ports []Port) error

	// ClosePorts closes the given ports on the instance, which
	// should have been started with the given machine id.
	ClosePorts(machineId string, ports []Port) error

	// Ports returns the set of ports open on the instance, which
	// should have been started with the given machine id.
	// The ports are returned as sorted by state.SortPorts.
	Ports(machineId string) ([]Port, error)
}

// HardwareCharacteristics represents the characteristics of the instance (if known).
// Attributes that are nil are unknown or not supported.
type HardwareCharacteristics struct {
	Arch     *string
	Mem      *uint64
	CpuCores *uint64
	CpuPower *uint64
}

func uintStr(i uint64) string {
	if i == 0 {
		return ""
	}
	return fmt.Sprintf("%d", i)
}

func (v HardwareCharacteristics) String() string {
	var strs []string
	if v.Arch != nil {
		strs = append(strs, "arch="+*v.Arch)
	}
	if v.CpuCores != nil {
		strs = append(strs, "cpu-cores="+uintStr(*v.CpuCores))
	}
	if v.CpuPower != nil {
		strs = append(strs, "cpu-power="+uintStr(*v.CpuPower))
	}
	if v.Mem != nil {
		s := uintStr(*v.Mem)
		if s != "" {
			s += "M"
		}
		strs = append(strs, "mem="+s)
	}
	return strings.Join(strs, " ")
}
