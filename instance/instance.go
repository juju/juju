// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance

import (
	"errors"

	"launchpad.net/juju-core/state/api/params"
)

var ErrNoDNSName = errors.New("DNS name not allocated")

// An instance Id is a provider-specific identifier associated with an
// instance (physical or virtual machine allocated in the provider).
type Id string

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
	OpenPorts(machineId string, ports []params.Port) error

	// ClosePorts closes the given ports on the instance, which
	// should have been started with the given machine id.
	ClosePorts(machineId string, ports []params.Port) error

	// Ports returns the set of ports open on the instance, which
	// should have been started with the given machine id.
	// The ports are returned as sorted by state.SortPorts.
	Ports(machineId string) ([]params.Port, error)
}
